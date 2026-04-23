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

	"github.com/teacat99/PortPass/internal/config"
	"github.com/teacat99/PortPass/internal/lifecycle"
	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/netutil"
	"github.com/teacat99/PortPass/internal/store"
)

// Server wires the HTTP router with its dependencies. Constructing Server
// from main keeps the API package free of global state and simplifies tests.
type Server struct {
	cfg       *config.Config
	store     *store.Store
	lifecycle *lifecycle.Manager
	limiter   *ipRateLimiter
}

// New builds a Server with all collaborators supplied. Callers must not use
// nil pointers; the API package is a thin coordinator and does not itself
// instantiate these dependencies.
func New(cfg *config.Config, s *store.Store, lm *lifecycle.Manager) *Server {
	return &Server{
		cfg:       cfg,
		store:     s,
		lifecycle: lm,
		limiter:   newIPRateLimiter(cfg.RateLimitPerMinutePerIP, time.Minute),
	}
}

// Router mounts the /api/* tree on a gin.Engine. Auth middlewares are
// attached in M4 and apply across the whole /api group. Static file serving
// (for embedded frontend) is wired separately by the caller in main.
func (s *Server) Router(engine *gin.Engine) {
	g := engine.Group("/api")

	g.GET("/health", s.handleHealth)
	g.GET("/client-ip", s.handleClientIP)

	g.GET("/rules", s.handleListRules)
	g.POST("/rules", s.handleCreateRule)
	g.GET("/rules/:id", s.handleGetRule)
	g.POST("/rules/:id/terminate", s.handleTerminateRule)
	g.POST("/rules/:id/extend", s.handleExtendRule)
	g.POST("/rules/:id/duplicate", s.handleDuplicateRule)

	g.GET("/history", s.handleHistory)

	g.GET("/preset-ports", s.handleListPresets)
	g.POST("/preset-ports", s.handleUpsertPreset)
	g.DELETE("/preset-ports/:id", s.handleDeletePreset)

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

	existing, err := s.store.ListActiveByIP(clientIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if s.cfg.MaxRulesPerIP > 0 && len(existing) >= s.cfg.MaxRulesPerIP {
		c.JSON(http.StatusForbidden, gin.H{"error": "concurrent rule quota exceeded"})
		return
	}

	rule := &model.Rule{
		SourceIP:  source,
		Port:      req.Port,
		Protocol:  proto,
		Note:      req.Note,
		Status:    model.StatusPending,
		ExpireAt:  expireAt,
		CreatedBy: "local",
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
		Action: "create", RuleID: rule.ID, Actor: rule.CreatedBy, ActorIP: clientIP,
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
	rules, total, err := s.store.ListAllRules(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules, "total": total})
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
	if r.Status != model.StatusActive && r.Status != model.StatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rule is not active"})
		return
	}
	newExpire := r.ExpireAt.Add(time.Duration(req.DurationSec) * time.Second)
	maxExpire := time.Now().Add(time.Duration(s.cfg.MaxDurationHours) * time.Hour)
	if newExpire.After(maxExpire) {
		newExpire = maxExpire
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
	dup := &model.Rule{
		SourceIP: src.SourceIP, Port: src.Port, Protocol: src.Protocol, Note: src.Note,
		Status: model.StatusPending, ExpireAt: expireAt, CreatedBy: "local",
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
		Action: "duplicate", RuleID: dup.ID, ActorIP: clientIP,
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
	rules, total, err := s.store.ListAllRules(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules, "total": total})
}

func (s *Server) handleListPresets(c *gin.Context) {
	ps, err := s.store.ListPresetPorts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ps)
}

func (s *Server) handleUpsertPreset(c *gin.Context) {
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
	if err := s.store.UpsertPresetPort(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (s *Server) handleDeletePreset(c *gin.Context) {
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
