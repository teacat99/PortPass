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
	"github.com/teacat99/PortPass/internal/captcha"
	"github.com/teacat99/PortPass/internal/config"
	"github.com/teacat99/PortPass/internal/lifecycle"
	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/netutil"
	"github.com/teacat99/PortPass/internal/notify"
	"github.com/teacat99/PortPass/internal/portset"
	"github.com/teacat99/PortPass/internal/runtime"
	"github.com/teacat99/PortPass/internal/store"
)

// ensurePortPolicy enforces the full policy chain for a rule request:
//  1. The requested port group must not overlap any ProtectedPort for
//     ANY caller (admin included) - operator-declared "hands-off" ports.
//  2. Admins are otherwise unrestricted.
//  3. Non-admins with at least one UserAllowedRange row use that list
//     as their exclusive override (covering preset.user_allowed).
//  4. Non-admins with no personal policy fall back to the global
//     preset.user_allowed whitelist.
//  5. Whichever slot matched, duration_sec must not exceed its
//     MaxDurationSec when that cap is non-zero.
//
// On success returns (maxDurationSec, true) where a zero means "no
// explicit cap"; on failure writes the HTTP response and returns false.
func (s *Server) ensurePortPolicy(c *gin.Context, ps portset.Set, proto string, durationSec int) (int, bool) {
	if ps.Empty() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no ports on request"})
		return 0, false
	}
	// (1) Protected check applies to everyone, admin included.
	prot, err := s.store.FindProtectedOverlap(ps, proto)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return 0, false
	}
	if prot != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("port(s) %s are protected (%s)", prot.Ports, prot.Name)})
		return 0, false
	}

	uid, _, role := auth.Principal(c)
	if role == model.RoleAdmin {
		return 0, true
	}

	hasPersonal, err := s.store.HasPersonalRanges(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return 0, false
	}
	var cap int
	if hasPersonal {
		match, err := s.store.FindUserAllowedForRequest(uid, ps, proto)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return 0, false
		}
		if match == nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "ports not in your allowed ranges"})
			return 0, false
		}
		cap = match.MaxDurationSec
	} else {
		matches, err := s.store.FindPresetsForPortSet(ps, proto)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return 0, false
		}
		if len(matches) == 0 {
			c.JSON(http.StatusForbidden, gin.H{"error": "port not allowed for non-admin user"})
			return 0, false
		}
		// Pick the MOST RESTRICTIVE non-zero MaxDurationSec as the cap;
		// if none set a cap, leave it at zero.
		for _, p := range matches {
			if p.MaxDurationSec <= 0 {
				continue
			}
			if cap == 0 || p.MaxDurationSec < cap {
				cap = p.MaxDurationSec
			}
		}
	}
	if cap > 0 && durationSec > cap {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("duration exceeds allowed for this port (max %ds)", cap)})
		return 0, false
	}
	return cap, true
}

// Server wires the HTTP router with its dependencies. Constructing Server
// from main keeps the API package free of global state and simplifies tests.
type Server struct {
	cfg       *config.Config
	rt        *runtime.Settings
	store     *store.Store
	lifecycle *lifecycle.Manager
	auth      *auth.Authenticator
	captcha   *captcha.Service
	notify    *notify.Ntfy
	limiter   *ipRateLimiter
}

// New builds a Server with all collaborators supplied. Callers must not use
// nil pointers (except captcha / notify, which are optional features); the
// API package is a thin coordinator and does not itself instantiate these
// dependencies.
func New(
	cfg *config.Config,
	rt *runtime.Settings,
	s *store.Store,
	lm *lifecycle.Manager,
	a *auth.Authenticator,
	cs *captcha.Service,
	nt *notify.Ntfy,
) *Server {
	limiter := newIPRateLimiter(rt.RateLimitPerMinutePerIP(), time.Minute)
	rt.AddHook(runtime.KeyRateLimitPerMinutePerIP, func() {
		limiter.SetMax(rt.RateLimitPerMinutePerIP())
	})
	return &Server{
		cfg:       cfg,
		rt:        rt,
		store:     s,
		lifecycle: lm,
		auth:      a,
		captcha:   cs,
		notify:    nt,
		limiter:   limiter,
	}
}

// Router mounts the /api/* tree on a gin.Engine. Authentication is enforced
// by the auth middleware; /auth/* endpoints are mounted before the gate so
// unauthenticated clients can still log in and discover auth mode.
func (s *Server) Router(engine *gin.Engine) {
	pub := engine.Group("/api")
	pub.GET("/auth/status", s.auth.StatusHandler)
	pub.POST("/auth/login", s.auth.LoginHandler)
	pub.GET("/auth/captcha", s.handleIssueCaptcha)

	g := engine.Group("/api", s.auth.Middleware())

	g.GET("/health", s.handleHealth)
	g.GET("/client-ip", s.handleClientIP)

	// Identity & self-service.
	g.GET("/auth/me", s.handleMe)
	g.POST("/auth/password", s.handleChangeOwnPassword)
	// Login history: self-view for every user, system-wide for admins.
	g.GET("/auth/my-recent-logins", s.handleMyLoginHistory)
	g.GET("/auth/login-history", s.handleLoginHistory)

	// Rules are visible to every authenticated user; per-role scoping is
	// applied inside the handler (admin sees all, user sees own).
	g.GET("/rules", s.handleListRules)
	g.POST("/rules", s.handleCreateRule)
	g.GET("/rules/:id", s.handleGetRule)
	g.POST("/rules/:id/terminate", s.handleTerminateRule)
	g.POST("/rules/:id/extend", s.handleExtendRule)
	g.POST("/rules/:id/duplicate", s.handleDuplicateRule)
	g.POST("/rules/:id/notify", s.handleSetRuleNotify)

	g.GET("/history", s.handleHistory)

	// Preset list is readable by every user (non-admin sees only the
	// user-allowed subset, further filtered by personal policy). Mutations
	// are admin-only.
	g.GET("/preset-ports", s.handleListPresets)
	g.POST("/preset-ports", s.handleUpsertPreset)
	g.DELETE("/preset-ports/:id", s.handleDeletePreset)

	// Preset categories — list is open to every authenticated user so
	// the home page can render group icons; mutations are admin-only.
	g.GET("/preset-categories", s.handleListPresetCategories)
	g.POST("/preset-categories", s.handleUpsertPresetCategory)
	g.DELETE("/preset-categories/:id", s.handleDeletePresetCategory)

	// Protected ports — admin-only list + CRUD; used by the policy
	// chain to block anyone (admin included) from opening sensitive
	// business ports by accident.
	g.GET("/protected-ports", s.handleListProtected)
	g.POST("/protected-ports", s.handleUpsertProtected)
	g.DELETE("/protected-ports/:id", s.handleDeleteProtected)

	// User management endpoints (admin-only is enforced inside the
	// handler via ensureAdmin so the auth layer can keep a single gate).
	g.GET("/users", s.handleListUsers)
	g.POST("/users", s.handleCreateUser)
	g.PUT("/users/:id", s.handleUpdateUser)
	g.POST("/users/:id/password", s.handleResetUserPassword)
	g.DELETE("/users/:id", s.handleDeleteUser)

	// Per-user allowed port ranges — admin-only list/create/delete.
	// Adding any row switches the user from preset-whitelist fallback
	// to personal-range override.
	g.GET("/users/:id/port-ranges", s.handleListUserRanges)
	g.POST("/users/:id/port-ranges", s.handleUpsertUserRange)
	g.DELETE("/users/:id/port-ranges/:rid", s.handleDeleteUserRange)
	g.DELETE("/users/:id/port-ranges", s.handleClearUserRanges)

	g.GET("/settings", s.handleGetSettings)
	g.PUT("/settings", s.handlePutSettings)

	// Runtime tunables: typed view of the hot-mutable subset of config.
	g.GET("/runtime-settings", s.handleGetRuntimeSettings)
	g.PUT("/runtime-settings", s.handlePutRuntimeSettings)

	// Ntfy push: synchronous test hook so the operator can validate URL.
	g.POST("/notify/test", s.handleTestNotify)

	// Expiry-notification polling. Browser tabs hit /pending every ~30s
	// to fetch their own rules whose lead-time threshold has elapsed
	// but were not yet flagged as notified, then ack the IDs they
	// successfully showed via Notification API. Per-rule scoping is
	// done inside the handlers (always limited to the caller's user_id).
	g.GET("/notify/pending", s.handleNotifyPending)
	g.POST("/notify/ack", s.handleNotifyAck)

	// Public-ish view of the three notify settings every user needs in
	// order to render the bell toggle and the polling loop. We split
	// this out from /runtime-settings (admin-only) so non-admin users
	// can still see the defaults and the lead time without leaking the
	// full runtime configuration surface.
	g.GET("/notify/settings", s.handleNotifySettings)
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
	SourceIP    string `json:"source_ip"`
	UseClientIP bool   `json:"use_client_ip"`
	// Port stays for backwards-compatibility with older API clients that
	// didn't know about port groups. New clients should supply Ports.
	Port        int    `json:"port"`
	Ports       string `json:"ports"`
	Protocol    string `json:"protocol"`
	DurationSec int    `json:"duration_sec"`
	ExpireAt    string `json:"expire_at"`
	Note        string `json:"note"`
	// NotifyEnabled toggles the expiry-notification feature for this
	// rule. When true the lead time is snapshotted from the current
	// runtime.NotifyLeadMinutes() setting, so future setting changes
	// don't affect rules already created. The pointer differentiates
	// "unspecified by client" (use server default) from "explicitly
	// false" (opt out even when the default is on).
	NotifyEnabled *bool `json:"notify_enabled,omitempty"`
	// CleanupOnExpire records whether the lifecycle manager should
	// drop existing conntrack entries when the firewall rule is
	// removed (auto expiry, reconcile-driven cleanup). Pointer so
	// "unset by client" (use runtime default) is distinguishable
	// from "explicitly false".
	CleanupOnExpire *bool `json:"cleanup_on_expire,omitempty"`
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
	ps, err := resolvePortSet(req.Ports, req.Port)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	expireAt, err := resolveExpiry(req, s.rt.MaxDurationHours())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, username, _ := auth.Principal(c)

	// Port policy: Protected + personal range (if any) or preset.user_allowed.
	durationForPolicy := req.DurationSec
	if durationForPolicy == 0 {
		durationForPolicy = int(time.Until(expireAt).Seconds())
	}
	if _, ok := s.ensurePortPolicy(c, ps, proto, durationForPolicy); !ok {
		return
	}

	existing, err := s.store.ListActiveByUserIP(uid, clientIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if maxRules := s.rt.MaxRulesPerIP(); maxRules > 0 && len(existing) >= maxRules {
		c.JSON(http.StatusForbidden, gin.H{"error": "concurrent rule quota exceeded"})
		return
	}

	notifyEnabled := s.rt.NotifyDefaultEnabled()
	if req.NotifyEnabled != nil {
		notifyEnabled = *req.NotifyEnabled
	}
	notifyLead := 0
	if notifyEnabled {
		notifyLead = s.rt.NotifyLeadMinutes() * 60
	}
	cleanupOnExpire := s.rt.CleanupOnExpireDefault()
	if req.CleanupOnExpire != nil {
		cleanupOnExpire = *req.CleanupOnExpire
	}
	rule := &model.Rule{
		UserID:            uid,
		SourceIP:          source,
		Port:              ps.First(),
		Ports:             ps.String(),
		Protocol:          proto,
		Note:              req.Note,
		Status:            model.StatusPending,
		ExpireAt:          expireAt,
		CreatedBy:         username,
		CreatedIP:         clientIP,
		CreatedAt:         time.Now(),
		NotifyEnabled:     notifyEnabled,
		NotifyLeadSeconds: notifyLead,
		CleanupOnExpire:   cleanupOnExpire,
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
		Detail: fmt.Sprintf("%s %s/%s until %s", source, rule.Ports, proto, expireAt.Format(time.RFC3339)),
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

// terminateReq captures the optional cleanup toggle the UI surfaces in
// the "Terminate" confirmation dialog. The body is optional: a missing
// or empty body keeps cleanup off (safe default).
type terminateReq struct {
	Cleanup bool `json:"cleanup"`
}

func (s *Server) handleTerminateRule(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Body is optional - the legacy clients send no body at all and
	// must keep working. ShouldBindJSON returns EOF on empty body which
	// we treat as cleanup=false; any actual JSON parse error stays
	// fatal so a malformed payload doesn't silently flip behaviour.
	var req terminateReq
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	r, err := s.store.GetRule(id)
	if err != nil || r == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if !s.ensureRuleVisible(c, r) {
		return
	}
	if err := s.lifecycle.Revoke(r, req.Cleanup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	detail := ""
	if req.Cleanup {
		detail = fmt.Sprintf("cleanup=true flushed=%d", r.LastCleanupCount)
	}
	_ = s.store.WriteAudit(&model.AuditLog{
		Action: "terminate", RuleID: r.ID, ActorIP: s.clientIP(c), Detail: detail,
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
	maxExpire := time.Now().Add(time.Duration(s.rt.MaxDurationHours()) * time.Hour)
	if newExpire.After(maxExpire) {
		newExpire = maxExpire
	}
	// Port policy: extending must still fit the preset cap for regular users.
	remaining := int(time.Until(newExpire).Seconds())
	rps := rulePorts(r)
	if _, ok := s.ensurePortPolicy(c, rps, r.Protocol, remaining); !ok {
		return
	}
	// Extending starts a fresh notification cycle: clear both per-channel
	// sent_at marks so the browser poll and ntfy watcher both fire again
	// before the *new* expiry.
	r.NotifySentBrowserAt = nil
	r.NotifySentNtfyAt = nil
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
	max := time.Now().Add(time.Duration(s.rt.MaxDurationHours()) * time.Hour)
	if expireAt.After(max) {
		expireAt = max
	}
	uid, username, _ := auth.Principal(c)
	durationForPolicy := int(time.Until(expireAt).Seconds())
	rps := rulePorts(src)
	if _, ok := s.ensurePortPolicy(c, rps, src.Protocol, durationForPolicy); !ok {
		return
	}
	dupNotifyLead := 0
	if src.NotifyEnabled {
		dupNotifyLead = s.rt.NotifyLeadMinutes() * 60
	}
	dup := &model.Rule{
		UserID: uid, SourceIP: src.SourceIP, Port: src.Port, Ports: src.Ports,
		Protocol: src.Protocol, Note: src.Note,
		Status: model.StatusPending, ExpireAt: expireAt, CreatedBy: username,
		CreatedIP: clientIP, CreatedAt: time.Now(),
		NotifyEnabled:     src.NotifyEnabled,
		NotifyLeadSeconds: dupNotifyLead,
		CleanupOnExpire:   src.CleanupOnExpire,
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

// notifyReq toggles expiry-notification on an existing rule. Only the
// enabled flag is configurable from the client; the lead time always
// snapshots the current runtime setting at the moment of enabling so a
// later global change does not retroactively move this rule's window.
type notifyReq struct {
	Enabled bool `json:"enabled"`
}

// handleSetRuleNotify flips notify_enabled on an already-created rule.
// Re-enabling counts as a fresh notification cycle: lead_seconds is
// re-snapshotted from settings and both per-channel sent_at marks are
// cleared, mirroring the Extend flow. Disabling leaves lead_seconds /
// sent_at intact so the operator can re-enable later without losing the
// audit trail of past pushes.
func (s *Server) handleSetRuleNotify(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req notifyReq
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
	prev := r.NotifyEnabled
	r.NotifyEnabled = req.Enabled
	if req.Enabled {
		r.NotifyLeadSeconds = s.rt.NotifyLeadMinutes() * 60
		r.NotifySentBrowserAt = nil
		r.NotifySentNtfyAt = nil
	}
	if err := s.store.UpdateRule(r); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if prev != r.NotifyEnabled {
		_, username, _ := auth.Principal(c)
		_ = s.store.WriteAudit(&model.AuditLog{
			Action: "notify", RuleID: r.ID, Actor: username, ActorIP: s.clientIP(c),
			Detail: fmt.Sprintf("enabled=%v lead_seconds=%d", r.NotifyEnabled, r.NotifyLeadSeconds),
		})
	}
	c.JSON(http.StatusOK, r)
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
	uid, _, role := auth.Principal(c)
	if role == model.RoleAdmin {
		ps, err := s.store.ListPresetPorts()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, ps)
		return
	}
	// Non-admin: start from user-allowed presets. When the user has a
	// personal range policy, further filter to presets fully covered
	// by at least one of their ranges — the "override" semantics.
	ps, err := s.store.ListUserAllowedPresets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hasPersonal, err := s.store.HasPersonalRanges(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !hasPersonal {
		c.JSON(http.StatusOK, ps)
		return
	}
	ranges, err := s.store.ListUserAllowedRanges(uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filtered := make([]model.PresetPort, 0, len(ps))
	for _, p := range ps {
		pset, err := portset.Parse(p.Ports)
		if err != nil || pset.Empty() {
			continue
		}
		for _, r := range ranges {
			rset, err := portset.Parse(r.Ports)
			if err != nil || rset.Empty() {
				continue
			}
			// Match when protocols are compatible AND the user range
			// is a superset of the preset's port group.
			if (r.Protocol == p.Protocol || r.Protocol == model.ProtoBoth || p.Protocol == model.ProtoBoth) &&
				rset.ContainsSet(pset) {
				filtered = append(filtered, p)
				break
			}
		}
	}
	c.JSON(http.StatusOK, filtered)
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
	ps, err := resolvePortSet(p.Ports, p.Port)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	// Verify the referenced category exists. nil category_id means
	// "auto-detect", which is valid; a non-nil reference must resolve.
	if p.CategoryID != nil {
		cat, err := s.store.GetPresetCategory(*p.CategoryID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if cat == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category_id does not exist"})
			return
		}
	}
	// Reverse-intersection check: a preset cannot cover any port that
	// is registered as protected. Surfaces the conflict early so the
	// admin edits one list at a time.
	if clash, err := s.store.FindProtectedOverlap(ps, p.Protocol); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if clash != nil {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("preset overlaps protected port %q (%s)", clash.Name, clash.Ports)})
		return
	}
	p.Ports = ps.String()
	p.Port = ps.First()
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

// handleListPresetCategories returns every category in render order.
// Read access is open to every authenticated user so the home page
// (used by both admin and user roles) can render group icons without
// requiring elevated privileges.
func (s *Server) handleListPresetCategories(c *gin.Context) {
	cats, err := s.store.ListPresetCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cats)
}

// handleUpsertPresetCategory creates or updates a category. The Builtin
// flag is server-controlled: a fresh insert always lands as Builtin=false,
// and an update preserves whatever the existing row had so a rename or
// icon swap on a built-in row keeps its protected status.
func (s *Server) handleUpsertPresetCategory(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	var p model.PresetCategory
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(p.Label) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "label too long (max 64)"})
		return
	}
	if len(p.Icon) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "icon too long (max 255)"})
		return
	}
	// Edits look up the current row to preserve Builtin / Key. New rows
	// get Builtin=false unconditionally and a blank Key (built-ins are
	// only ever created by the seeder).
	if p.ID != 0 {
		existing, err := s.store.GetPresetCategory(p.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "category not found"})
			return
		}
		p.Builtin = existing.Builtin
		p.Key = existing.Key
		p.CreatedAt = existing.CreatedAt
	} else {
		p.Builtin = false
		p.Key = ""
	}
	if err := s.store.UpsertPresetCategory(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

// handleDeletePresetCategory removes a non-builtin category. Built-in
// categories are protected (Builtin=true returns 409) so the seven-row
// baseline can never be wiped out by accident; presets pointing at the
// deleted row are detached inside the store transaction.
func (s *Server) handleDeletePresetCategory(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cat, err := s.store.GetPresetCategory(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cat == nil {
		c.Status(http.StatusNoContent)
		return
	}
	if cat.Builtin {
		c.JSON(http.StatusConflict, gin.H{"error": "builtin category cannot be deleted"})
		return
	}
	if err := s.store.DeletePresetCategory(id); err != nil {
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
		"max_duration_hours":     s.rt.MaxDurationHours(),
		"history_retention_days": s.rt.HistoryRetentionDays(),
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

// handleGetRuntimeSettings returns the typed snapshot of every hot-mutable
// runtime field plus a small read-only block of "system info" the UI
// renders next to the editable form. Includes the captcha threshold and
// the boolean state of the ntfy / subnet features so the frontend can
// hide the "Test" button or grey out unset paths.
func (s *Server) handleGetRuntimeSettings(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	system := gin.H{
		"listen":          s.cfg.Listen,
		"data_dir":        s.cfg.DataDir,
		"firewall_driver": s.cfg.FirewallDriver,
		"auth_mode":       string(s.cfg.AuthMode),
		"jwt_secret_set":  s.cfg.JWTSecret != "",
		"trusted_proxies": stringifyNets(s.cfg.TrustedProxies),
	}
	c.JSON(http.StatusOK, gin.H{
		"settings": s.rt.Snapshot(),
		"system":   system,
	})
}

// handlePutRuntimeSettings accepts a {key: stringValue} payload, runs
// each value through the typed validator in runtime.Settings, then
// atomically writes the survivors to the KV table. Failed validation
// returns 400 with a per-field error map so the UI can highlight the
// offending input.
func (s *Server) handlePutRuntimeSettings(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	var raw map[string]string
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updates := make(map[runtime.Key]string, len(raw))
	for k, v := range raw {
		updates[runtime.Key(k)] = v
	}
	if err := s.rt.SetMany(updates, s.store.SetSetting); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"settings": s.rt.Snapshot()})
}

// handleIssueCaptcha returns a fresh math challenge keyed by id. Mounted
// in the public group so unauthenticated users on the login page can
// fetch one when the previous attempt told them to.
func (s *Server) handleIssueCaptcha(c *gin.Context) {
	if s.captcha == nil {
		c.JSON(http.StatusOK, gin.H{"id": "", "question": ""})
		return
	}
	id, q := s.captcha.Issue()
	c.JSON(http.StatusOK, gin.H{"id": id, "question": q})
}

// handleNotifyPending returns rules belonging to the caller that are
// inside their pre-expiry notification window and not yet acknowledged.
// The browser tab polls this every ~30s and pops a Notification for
// every entry. Returns an empty list when the global channel selector
// excludes the browser (in that case ntfy is doing all the work and
// the UI doesn't need to chime).
func (s *Server) handleNotifyPending(c *gin.Context) {
	if !s.rt.NotifyChannelIncludes(runtime.NotifyChannelBrowser) {
		c.JSON(http.StatusOK, gin.H{"rules": []any{}})
		return
	}
	uid, _, _ := auth.Principal(c)
	if uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	rules, err := s.store.ListPendingNotify(uid, store.NotifyChannelBrowser, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

type notifyAckReq struct {
	RuleIDs []uint `json:"rule_ids"`
}

// handleNotifyAck stamps notify_sent_at on the supplied rule IDs after
// the browser has shown its local Notification. Scoped to the caller's
// own rules so a malicious user cannot suppress someone else's pushes.
func (s *Server) handleNotifyAck(c *gin.Context) {
	uid, _, _ := auth.Principal(c)
	if uid == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var req notifyAckReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.RuleIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"updated": 0})
		return
	}
	n, err := s.store.MarkNotifySent(req.RuleIDs, store.NotifyChannelBrowser, uid, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": n})
}

// handleNotifySettings returns the small slice of runtime-settings
// every authenticated user is allowed to read so HomeView and
// RulesView can render their per-rule controls. Originally scoped to
// expiry-notification (lead_minutes, channels, default_enabled), it
// has since picked up cleanup_on_expire_default because the create
// form needs that default too. The full /runtime-settings endpoint
// stays admin-only; this carve-out is kept minimal and the keys are
// stable across releases.
func (s *Server) handleNotifySettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"lead_minutes":              s.rt.NotifyLeadMinutes(),
		"channels":                  s.rt.NotifyChannels(),
		"default_enabled":           s.rt.NotifyDefaultEnabled(),
		"cleanup_on_expire_default": s.rt.CleanupOnExpireDefault(),
	})
}

// handleTestNotify fires one synchronous push so the operator gets
// instant feedback about a misconfigured URL/Topic/Token without
// having to trigger a real lockout to test.
func (s *Server) handleTestNotify(c *gin.Context) {
	if !s.ensureAdmin(c) {
		return
	}
	if s.notify == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notify subsystem not initialised"})
		return
	}
	if err := s.notify.Test(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
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

// resolvePortSet normalises the (ports, port) request inputs into a
// canonical portset.Set. New clients supply `ports`; legacy clients
// that still send just an integer port are transparently lifted into a
// single-port set so nothing breaks on upgrade.
func resolvePortSet(ports string, legacyPort int) (portset.Set, error) {
	if strings.TrimSpace(ports) != "" {
		ps, err := portset.Parse(ports)
		if err != nil {
			return portset.Set{}, err
		}
		if ps.Empty() {
			return portset.Set{}, errors.New("empty port list")
		}
		return ps, nil
	}
	if legacyPort < portset.MinPort || legacyPort > portset.MaxPort {
		return portset.Set{}, errors.New("port out of range")
	}
	return portset.FromPort(legacyPort), nil
}

// rulePorts extracts the canonical port set from a Rule, falling back
// to the legacy single-port column when Ports is empty.
func rulePorts(r *model.Rule) portset.Set {
	if strings.TrimSpace(r.Ports) != "" {
		if ps, err := portset.Parse(r.Ports); err == nil && !ps.Empty() {
			return ps
		}
	}
	if r.Port > 0 {
		return portset.FromPort(r.Port)
	}
	return portset.Set{}
}
