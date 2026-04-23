package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/teacat99/PortPass/internal/auth"
	"github.com/teacat99/PortPass/internal/config"
	"github.com/teacat99/PortPass/internal/lifecycle"
	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/netutil"
	"github.com/teacat99/PortPass/internal/store"
)

// ensurePortPolicy enforces the non-admin user port policy: the requested
// (port, protocol) must match a PresetPort with UserAllowed=true, and the
// requested duration must fit within the preset's MaxDurationSec (0 means
// inherit the global cap only). Admins bypass this check.
// Returns (matchedPreset, nil) on success; the caller is expected to abort
// with the returned error code when non-nil.
func (s *Server) ensurePortPolicy(c *gin.Context, port int, proto string, durationSec int) (*model.PresetPort, bool) {
	_, _, role := auth.Principal(c)
	if role == model.RoleAdmin {
		return nil, true
	}
	preset, err := s.store.FindPresetForUser(port, proto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	if preset == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "port not allowed for non-admin user"})
		return nil, false
	}
	if preset.MaxDurationSec > 0 && durationSec > preset.MaxDurationSec {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("duration exceeds allowed for this port (max %ds)", preset.MaxDurationSec)})
		return nil, false
	}
	return preset, true
}

// Server wires the HTTP router with its dependencies. Constructing Server
// from main keeps the API package free of global state and simplifies tests.
type Server struct {
	cfg       *config.Config
	store     *store.Store
	lifecycle *lifecycle.Manager
	auth      *auth.Authenticator
	limiter   *ipRateLimiter
}

// New builds a Server with all collaborators supplied. Callers must not use
// nil pointers; the API package is a thin coordinator and does not itself
// instantiate these dependencies.
func New(cfg *config.Config, s *store.Store, lm *lifecycle.Manager, a *auth.Authenticator) *Server {
	return &Server{
		cfg:       cfg,
		store:     s,
		lifecycle: lm,
		auth:      a,
		limiter:   newIPRateLimiter(cfg.RateLimitPerMinutePerIP, time.Minute),
	}
}

// Router mounts the /api/* tree on a gin.Engine. Authentication is enforced
// by the auth middleware; /auth/* endpoints are mounted before the gate so
// unauthenticated clients can still log in and discover auth mode.
func (s *Server) Router(engine *gin.Engine) {
	pub := engine.Group("/api")
	pub.GET("/auth/status", s.auth.StatusHandler)
	pub.POST("/auth/login", s.auth.LoginHandler)

	g := engine.Group("/api", s.auth.Middleware())

	g.GET("/health", s.handleHealth)
	g.GET("/client-ip", s.handleClientIP)

	// Identity & self-service.
	g.GET("/auth/me", s.handleMe)
	g.POST("/auth/password", s.handleChangeOwnPassword)

	// Rules are visible to every authenticated user; per-role scoping is
	// applied inside the handler (admin sees all, user sees own).
	g.GET("/rules", s.handleListRules)
	g.POST("/rules", s.handleCreateRule)
	g.GET("/rules/:id", s.handleGetRule)
	g.POST("/rules/:id/terminate", s.handleTerminateRule)
	g.POST("/rules/:id/extend", s.handleExtendRule)
	g.POST("/rules/:id/duplicate", s.handleDuplicateRule)

	g.GET("/history", s.handleHistory)

	// Preset list is readable by every user (non-admin sees only the
	// user-allowed subset). Mutations are admin-only.
	g.GET("/preset-ports", s.handleListPresets)
	g.POST("/preset-ports", s.handleUpsertPreset)
	g.DELETE("/preset-ports/:id", s.handleDeletePreset)

	// User management endpoints (admin-only is enforced inside the
	// handler via ensureAdmin so the auth layer can keep a single gate).
	g.GET("/users", s.handleListUsers)
	g.POST("/users", s.handleCreateUser)
	g.PUT("/users/:id", s.handleUpdateUser)
	g.POST("/users/:id/password", s.handleResetUserPassword)
	g.DELETE("/users/:id", s.handleDeleteUser)

	g.GET("/settings", s.handleGetSettings)
	g.PUT("/settings", s.handlePutSettings)
}

// clientIP is the single choke-point for extracting the trusted client IP.
// Every handler that cares about the caller identity routes through here.
func (s *Server) clientIP(c *gin.Context) string {
	return netutil.ClientIP(c.Request, s.cfg.TrustedProxies)
}

// ------------------------- handlers -------------------------

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "time": time.Now().UTC()})
}

func (s *Server) handleClientIP(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ip": s.clientIP(c)})
}

type createRuleReq struct {
	SourceIP       string `json:"source_ip"`
	UseClientIP    bool   `json:"use_client_ip"`
	Port           int    `json:"port" binding:"required"`
	Protocol       string `json:"protocol"`
	DurationSec    int    `json:"duration_sec"`
	ExpireAt       string `json:"expire_at"`
	Note           string `json:"note"`
}

// handleCreateRule creates, persists and schedules a new firewall rule. It
// validates the request thoroughly up-front (port range, protocol enum,
// max-duration cap, per-IP rate and concurrency caps) because a bad rule
// would either fail silently at the driver layer or be wasted work.
func (s *Server) handleCreateRule(c *gin.Context) {
	var req createRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	clientIP := s.clientIP(c)

	if !s.limiter.Allow(clientIP) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
		return
	}

	source, err := resolveSourceIP(req, clientIP)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	proto, err := normaliseProtocol(req.Protocol)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port < 1 || req.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "port out of range"})
		return
	}

	expireAt, err := resolveExpiry(req, s.cfg.MaxDurationHours)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, username, _ := auth.Principal(c)

	// Port policy: non-admin users must hit a user-allowed preset and
	// respect its MaxDurationSec. Admins skip this entirely.
	durationForPolicy := req.DurationSec
	if durationForPolicy == 0 {
		durationForPolicy = int(time.Until(expireAt).Seconds())
	}
	if _, ok := s.ensurePortPolicy(c, req.Port, proto, durationForPolicy); !ok {
		return
	}

	existing, err := s.store.ListActiveByUserIP(uid, clientIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.cfg.MaxRulesPerIP > 0 && len(existing) >= s.cfg.MaxRulesPerIP {
		c.JSON(http.StatusForbidden, gin.H{"error": "concurrent rule quota exceeded"})
		return
	}

	rule := &model.Rule{
		UserID:    uid,
		SourceIP:  source,
		Port:      req.Port,
		Protocol:  proto,
		Note:      req.Note,
		Status:    model.StatusPending,
		ExpireAt:  expireAt,
		CreatedBy: username,
		CreatedIP: clientIP,
		CreatedAt: time.Now(),
	}
	if err := s.store.CreateRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.lifecycle.Schedule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "create", RuleID: rule.ID, Actor: username, ActorIP: clientIP,
		Detail: fmt.Sprintf("%s %d/%s until %s", source, req.Port, proto, expireAt.Format(time.RFC3339)),
	})
	c.JSON(http.StatusOK, rule)
}

func (s *Server) handleListRules(c *gin.Context) {
	filter := store.RuleFilter{
		Statuses: []string{model.StatusPending, model.StatusActive},
		Limit:    parseIntDefault(c.Query("limit"), 200),
	}
	if q := c.Query("status"); q != "" {
		filter.Statuses = strings.Split(q, ",")
	}
	s.applyRoleScope(c, &filter)
	rules, total, err := s.store.ListAllRules(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules, "total": total})
}

// applyRoleScope narrows the filter to the caller's user id when the
// caller is not an admin. Admins may optionally pass ?user_id= to drill
// into a specific user; a zero or absent value returns everything.
func (s *Server) applyRoleScope(c *gin.Context, filter *store.RuleFilter) {
	uid, _, role := auth.Principal(c)
	if role == model.RoleAdmin {
		if q := c.Query("user_id"); q != "" {
			if n := parseIntDefault(q, 0); n > 0 {
				filter.UserID = uint(n)
			}
		}
		return
	}
	filter.UserID = uid
}

// ensureRuleVisible makes sure the current principal may read/mutate the
// target rule. Admins see all; users only their own.
func (s *Server) ensureRuleVisible(c *gin.Context, r *model.Rule) bool {
	uid, _, role := auth.Principal(c)
	if role == model.RoleAdmin {
		return true
	}
	if r.UserID != uid {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
		return false
	}
	return true
}

func (s *Server) handleGetRule(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := s.store.GetRule(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if r == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !s.ensureRuleVisible(c, r) {
		return
	}
	c.JSON(http.StatusOK, r)
}

func (s *Server) handleTerminateRule(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := s.store.GetRule(id)
	if err != nil || r == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !s.ensureRuleVisible(c, r) {
		return
	}
	if err := s.lifecycle.Revoke(r); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "terminate", RuleID: r.ID, ActorIP: s.clientIP(c),
	})
	c.JSON(http.StatusOK, r)
}

type extendReq struct {
	DurationSec int `json:"duration_sec" binding:"required"`
}

func (s *Server) handleExtendRule(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req extendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := s.store.GetRule(id)
	if err != nil || r == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !s.ensureRuleVisible(c, r) {
		return
	}
	if r.Status != model.StatusActive && r.Status != model.StatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule is not active"})
		return
	}
	newExpire := r.ExpireAt.Add(time.Duration(req.DurationSec) * time.Second)
	maxExpire := time.Now().Add(time.Duration(s.cfg.MaxDurationHours) * time.Hour)
	if newExpire.After(maxExpire) {
		newExpire = maxExpire
	}
	// Port policy: extending must still fit the preset cap for regular users.
	remaining := int(time.Until(newExpire).Seconds())
	if _, ok := s.ensurePortPolicy(c, r.Port, r.Protocol, remaining); !ok {
		return
	}
	if err := s.lifecycle.Extend(r, newExpire); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "extend", RuleID: r.ID, ActorIP: s.clientIP(c),
		Detail: fmt.Sprintf("+%ds -> %s", req.DurationSec, newExpire.Format(time.RFC3339)),
	})
	c.JSON(http.StatusOK, r)
}

func (s *Server) handleDuplicateRule(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	src, err := s.store.GetRule(id)
	if err != nil || src == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !s.ensureRuleVisible(c, src) {
		return
	}
	clientIP := s.clientIP(c)
	if !s.limiter.Allow(clientIP) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
		return
	}
	expireAt := time.Now().Add(time.Until(src.ExpireAt))
	if dur := expireAt.Sub(time.Now()); dur <= 0 {
		expireAt = time.Now().Add(time.Hour)
	}
	max := time.Now().Add(time.Duration(s.cfg.MaxDurationHours) * time.Hour)
	if expireAt.After(max) {
		expireAt = max
	}
	uid, username, _ := auth.Principal(c)
	durationForPolicy := int(time.Until(expireAt).Seconds())
	if _, ok := s.ensurePortPolicy(c, src.Port, src.Protocol, durationForPolicy); !ok {
		return
	}
	dup := &model.Rule{
		UserID: uid, SourceIP: src.SourceIP, Port: src.Port, Protocol: src.Protocol, Note: src.Note,
		Status: model.StatusPending, ExpireAt: expireAt, CreatedBy: username,
		CreatedIP: clientIP, CreatedAt: time.Now(),
	}
	if err := s.store.CreateRule(dup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := s.lifecycle.Schedule(dup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "duplicate", RuleID: dup.ID, Actor: username, ActorIP: clientIP,
		Detail: fmt.Sprintf("from rule %d", src.ID),
	})
	c.JSON(http.StatusOK, dup)
}

func (s *Server) handleHistory(c *gin.Context) {
	filter := store.RuleFilter{
		Statuses: []string{model.StatusExpired, model.StatusRevoked, model.StatusFailed},
		Limit:    parseIntDefault(c.Query("limit"), 100),
		Offset:   parseIntDefault(c.Query("offset"), 0),
		IP:       c.Query("ip"),
		Port:     parseIntDefault(c.Query("port"), 0),
	}
	if status := c.Query("status"); status != "" {
		filter.Statuses = strings.Split(status, ",")
	}
	if from := c.Query("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			filter.From = t
		}
	}
	if to := c.Query("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			filter.To = t
		}
	}
	s.applyRoleScope(c, &filter)
	rules, total, err := s.store.ListAllRules(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules, "total": total})
}

func (s *Server) handleListPresets(c *gin.Context) {
	_, _, role := auth.Principal(c)
	var (
		ps  []model.PresetPort
		err error
	)
	if role == model.RoleAdmin {
		ps, err = s.store.ListPresetPorts()
	} else {
		ps, err = s.store.ListUserAllowedPresets()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ps)
}

func (s *Server) handleUpsertPreset(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	var p model.PresetPort
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if p.Port < 1 || p.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "port out of range"})
		return
	}
	if _, err := normaliseProtocol(p.Protocol); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if p.MaxDurationSec < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_duration_sec must be >= 0"})
		return
	}
	if err := s.store.UpsertPresetPort(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (s *Server) handleDeletePreset(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.DeletePresetPort(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleGetSettings(c *gin.Context) {
	rows, err := s.store.ListSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := gin.H{
		"auth_mode":              string(s.cfg.AuthMode),
		"max_duration_hours":     s.cfg.MaxDurationHours,
		"history_retention_days": s.cfg.HistoryRetentionDays,
		"firewall_driver":        s.cfg.FirewallDriver,
		"trusted_proxies":        stringifyNets(s.cfg.TrustedProxies),
		"kv":                     rows,
	}
	c.JSON(http.StatusOK, out)
}

func (s *Server) handlePutSettings(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	var kv map[string]string
	if err := c.ShouldBindJSON(&kv); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for k, v := range kv {
		if err := s.store.SetSetting(k, v); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.Status(http.StatusOK)
}

// ------------------------- helpers -------------------------

func resolveSourceIP(req createRuleReq, clientIP string) (string, error) {
	if req.UseClientIP {
		if clientIP == "" {
			return "", errors.New("client IP unavailable")
		}
		return appendMask(clientIP), nil
	}
	src := strings.TrimSpace(req.SourceIP)
	if src == "" || src == "any" || src == "all" {
		return "0.0.0.0/0", nil
	}
	if _, _, err := net.ParseCIDR(src); err == nil {
		return src, nil
	}
	if ip := net.ParseIP(src); ip != nil {
		return appendMask(src), nil
	}
	return "", fmt.Errorf("invalid source IP %q", src)
}

func appendMask(ip string) string {
	if strings.Contains(ip, "/") {
		return ip
	}
	if strings.Contains(ip, ":") {
		return ip + "/128"
	}
	return ip + "/32"
}

func normaliseProtocol(p string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", model.ProtoTCP:
		return model.ProtoTCP, nil
	case model.ProtoUDP:
		return model.ProtoUDP, nil
	case model.ProtoBoth, "tcp+udp", "tcp_udp":
		return model.ProtoBoth, nil
	}
	return "", fmt.Errorf("invalid protocol %q", p)
}

func resolveExpiry(req createRuleReq, maxHours int) (time.Time, error) {
	now := time.Now()
	max := now.Add(time.Duration(maxHours) * time.Hour)
	var expire time.Time
	switch {
	case req.ExpireAt != "":
		t, err := time.Parse(time.RFC3339, req.ExpireAt)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid expire_at: %w", err)
		}
		expire = t
	case req.DurationSec > 0:
		expire = now.Add(time.Duration(req.DurationSec) * time.Second)
	default:
		return time.Time{}, errors.New("duration_sec or expire_at is required")
	}
	if !expire.After(now) {
		return time.Time{}, errors.New("expiry must be in the future")
	}
	if expire.After(max) {
		return time.Time{}, fmt.Errorf("expiry exceeds max %dh", maxHours)
	}
	return expire, nil
}

func parseID(s string) (uint, error) {
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", s)
	}
	return uint(n), nil
}

func parseIntDefault(s string, d int) int {
	if s == "" {
		return d
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return d
	}
	return n
}

func stringifyNets(nets []*net.IPNet) []string {
	out := make([]string, 0, len(nets))
	for _, n := range nets {
		out = append(out, n.String())
	}
	return out
}
