package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/teacat99/PortPass/internal/config"
	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/netutil"
)

// UserRepo captures the parts of the user store that the authenticator
// needs. Keeping it narrow lets the unit tests provide a lightweight fake
// without booting a real SQLite database. The login-attempt methods back
// the brute-force defence; the user methods back the credential lookup.
type UserRepo interface {
	GetUserByUsername(name string) (*model.User, error)
	GetUserByID(id uint) (*model.User, error)

	RecordLoginAttempt(a *model.LoginAttempt) error
	CountLoginFailuresByIP(ip string, since time.Time) (int64, error)
	CountLoginFailuresByUsername(username string, since time.Time) (int64, error)
	LastSuccessfulLogin(username string) (*model.LoginAttempt, error)
}

// Authenticator bundles login and middleware logic for a specific AuthMode.
// It is constructed once during server bootstrap and reused across requests.
type Authenticator struct {
	cfg    *config.Config
	secret []byte
	users  UserRepo

	// systemAdminID / systemAdminUsername identify the implicit actor for
	// ipwhitelist / none modes. They are populated after the store seeds
	// the admin account during bootstrap.
	systemAdminID       uint
	systemAdminUsername string
}

// Context keys used to propagate the authenticated principal to handlers.
const (
	ctxKeyUserID   = "pp_user_id"
	ctxKeyUsername = "pp_username"
	ctxKeyRole     = "pp_role"
)

// Login failure reason codes. They live in the DB verbatim (short, stable
// machine-readable strings) so the UI and audit log consumers can filter
// on them without parsing free-form English text.
const (
	ReasonOK             = "ok"
	ReasonBadRequest     = "bad_request"
	ReasonInvalidCreds   = "invalid_credentials"
	ReasonUserDisabled   = "user_disabled"
	ReasonAuthDisabled   = "auth_disabled"
	ReasonLockedIP       = "locked_ip"
	ReasonLockedUser     = "locked_user"
	ReasonInternal       = "internal"
)

// New initialises an Authenticator. When no JWT secret is configured we
// derive a random one so tokens from previous processes are invalidated
// (acceptable for a single-admin self-hosted tool).
func New(cfg *config.Config, users UserRepo) *Authenticator {
	secret := []byte(cfg.JWTSecret)
	if len(secret) == 0 {
		secret = randomSecret(32)
	}
	return &Authenticator{cfg: cfg, secret: secret, users: users}
}

// SetSystemAdmin registers the built-in admin identity that non-password
// modes should impersonate. Call once at bootstrap after SeedAdminIfEmpty.
func (a *Authenticator) SetSystemAdmin(id uint, username string) {
	a.systemAdminID = id
	a.systemAdminUsername = username
}

// Middleware is the shared gate used by /api/* (login/status endpoints are
// mounted before this middleware so they remain reachable). Behaviour per
// mode:
//
//   - password     : requires a valid JWT whose user is still active.
//   - ipwhitelist  : requires the resolved client IP to be in the whitelist;
//                    the request is executed as the built-in system admin.
//   - none         : always allow; executed as the system admin.
func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch a.cfg.AuthMode {
		case config.AuthModePassword:
			ok, u := a.checkJWT(c)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorised"})
				return
			}
			a.setPrincipal(c, u.ID, u.Username, u.Role)
		case config.AuthModeIPWhitelist:
			if !a.checkIP(c) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "ip not permitted"})
				return
			}
			a.setPrincipal(c, a.systemAdminID, a.systemAdminUsername, model.RoleAdmin)
		case config.AuthModeNone:
			a.setPrincipal(c, a.systemAdminID, a.systemAdminUsername, model.RoleAdmin)
		}
		c.Next()
	}
}

func (a *Authenticator) setPrincipal(c *gin.Context, id uint, username, role string) {
	c.Set(ctxKeyUserID, id)
	c.Set(ctxKeyUsername, username)
	c.Set(ctxKeyRole, role)
}

// Principal reads the authenticated user attached by the middleware. It
// returns zero values when the context has not passed through the gate.
func Principal(c *gin.Context) (id uint, username, role string) {
	if v, ok := c.Get(ctxKeyUserID); ok {
		id, _ = v.(uint)
	}
	if v, ok := c.Get(ctxKeyUsername); ok {
		username, _ = v.(string)
	}
	if v, ok := c.Get(ctxKeyRole); ok {
		role, _ = v.(string)
	}
	return
}

// recordAttempt writes one login_attempts row and best-effort swallows
// the error: a DB write failure must not prevent the auth response.
func (a *Authenticator) recordAttempt(username, ip, userAgent, reason string, success bool) {
	_ = a.users.RecordLoginAttempt(&model.LoginAttempt{
		Username:  username,
		ClientIP:  ip,
		Success:   success,
		Reason:    reason,
		UserAgent: truncate(userAgent, 255),
	})
}

// penaltyDelay returns the per-attempt slowdown we inject on failures. It
// is driven by the running failure count for this username so normal
// users feel no delay on a single typo, while scripted attackers stall:
//   count=1 -> 100ms, 2 -> 200ms, 3 -> 400ms, ... capped at 5s.
// We apply it AFTER recording the failure so the counter is already up
// to date when the next attempt is evaluated.
func penaltyDelay(count int) time.Duration {
	if count <= 0 {
		return 0
	}
	ms := 100 * math.Pow(2, float64(count-1))
	if ms > 5000 {
		ms = 5000
	}
	return time.Duration(ms) * time.Millisecond
}

// LoginHandler authenticates against the users table (bcrypt) and returns
// a signed JWT. Password auth is the only mode that exposes this endpoint.
//
// Brute-force defence: every attempt is recorded in login_attempts. Before
// checking the password we reject requests whose client IP or submitted
// username has already crossed the configured failure threshold within
// the rolling window, answering 429 with a retry_after hint. Failed
// attempts additionally get an exponential delay so scripted attackers
// see their RPS collapse. All counters are persisted, so a server
// restart does not wipe the attacker's state.
func (a *Authenticator) LoginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password" binding:"required"`
	}
	clientIP := netutil.ClientIP(c.Request, a.cfg.TrustedProxies)
	userAgent := c.GetHeader("User-Agent")

	if err := c.ShouldBindJSON(&req); err != nil {
		a.recordAttempt("", clientIP, userAgent, ReasonBadRequest, false)
		// `code` is a stable machine-readable discriminator the frontend
		// maps to localised user-facing strings; `error` is the English
		// fallback kept for non-browser clients and log scrapers.
		c.JSON(http.StatusBadRequest, gin.H{"code": ReasonBadRequest, "error": err.Error()})
		return
	}
	if a.cfg.AuthMode != config.AuthModePassword {
		c.JSON(http.StatusBadRequest, gin.H{"code": ReasonAuthDisabled, "error": "password auth disabled"})
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		// Legacy single-user clients may still send only {password}; in
		// that case we default to the configured seed admin username so
		// the existing login flow keeps working.
		username = a.cfg.AdminUsername
		if username == "" {
			username = "admin"
		}
	}

	// --- 1. IP lockout check -------------------------------------------------
	if a.cfg.LoginFailMaxPerIP > 0 && a.cfg.LoginFailWindowIPMin > 0 {
		since := time.Now().Add(-time.Duration(a.cfg.LoginFailWindowIPMin) * time.Minute)
		n, err := a.users.CountLoginFailuresByIP(clientIP, since)
		if err == nil && int(n) >= a.cfg.LoginFailMaxPerIP {
			retry := a.cfg.LoginLockoutIPMin
			if retry <= 0 {
				retry = a.cfg.LoginFailWindowIPMin
			}
			a.recordAttempt(username, clientIP, userAgent, ReasonLockedIP, false)
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":              ReasonLockedIP,
				"error":             "too many failed attempts from this address",
				"retry_after_secs":  retry * 60,
			})
			return
		}
	}

	// --- 2. Username lockout check ------------------------------------------
	if a.cfg.LoginFailMaxPerUser > 0 && a.cfg.LoginFailWindowUserMin > 0 {
		since := time.Now().Add(-time.Duration(a.cfg.LoginFailWindowUserMin) * time.Minute)
		n, err := a.users.CountLoginFailuresByUsername(username, since)
		if err == nil && int(n) >= a.cfg.LoginFailMaxPerUser {
			retry := a.cfg.LoginLockoutUserMin
			if retry <= 0 {
				retry = a.cfg.LoginFailWindowUserMin
			}
			a.recordAttempt(username, clientIP, userAgent, ReasonLockedUser, false)
			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":              ReasonLockedUser,
				"error":             "too many failed attempts for this account",
				"retry_after_secs":  retry * 60,
			})
			return
		}
	}

	// --- 3. Credential verification -----------------------------------------
	u, err := a.users.GetUserByUsername(username)
	if err != nil {
		a.recordAttempt(username, clientIP, userAgent, ReasonInternal, false)
		c.JSON(http.StatusInternalServerError, gin.H{"code": ReasonInternal, "error": err.Error()})
		return
	}
	verified := false
	disabledHit := false
	if u == nil || u.Disabled || u.PasswordHash == "" {
		// Do a dummy bcrypt compare so the response timing does not leak
		// whether the username exists.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvaOZQZQ0ZQZQZQZQZQZQZQZQZQZQZQZQO"), []byte(req.Password))
		disabledHit = u != nil && u.Disabled
	} else if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err == nil {
		verified = true
	}

	if !verified {
		reason := ReasonInvalidCreds
		if disabledHit {
			reason = ReasonUserDisabled
		}
		a.recordAttempt(username, clientIP, userAgent, reason, false)
		// Running failure count for this username drives the delay.
		if a.cfg.LoginFailWindowUserMin > 0 {
			since := time.Now().Add(-time.Duration(a.cfg.LoginFailWindowUserMin) * time.Minute)
			if n, err := a.users.CountLoginFailuresByUsername(username, since); err == nil {
				time.Sleep(penaltyDelay(int(n)))
			}
		}
		c.JSON(http.StatusUnauthorized, gin.H{"code": ReasonInvalidCreds, "error": "invalid credentials"})
		return
	}

	// --- 4. Success ----------------------------------------------------------
	tok, err := a.sign(u)
	if err != nil {
		a.recordAttempt(username, clientIP, userAgent, ReasonInternal, false)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Fetch the previous successful login BEFORE recording this one, so the
	// "last login" banner shows the session the user cares about (the one
	// before this one) rather than the attempt they just completed.
	prev, _ := a.users.LastSuccessfulLogin(username)
	a.recordAttempt(username, clientIP, userAgent, ReasonOK, true)

	resp := gin.H{
		"token":    tok,
		"username": u.Username,
		"role":     u.Role,
	}
	if prev != nil {
		resp["last_login"] = gin.H{
			"at":         prev.CreatedAt,
			"client_ip":  prev.ClientIP,
			"user_agent": prev.UserAgent,
		}
	}
	c.JSON(http.StatusOK, resp)
}

// StatusHandler exposes enough metadata for the frontend router to decide
// whether to redirect to /login.
func (a *Authenticator) StatusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"mode":     string(a.cfg.AuthMode),
		"required": a.cfg.AuthMode != config.AuthModeNone,
	})
}

// sign issues a JWT with a 24h expiry, including the authenticated user's
// id / username / role so downstream handlers can authorise actions.
func (a *Authenticator) sign(u *model.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":      u.Username,
		"uid":      u.ID,
		"role":     u.Role,
		"username": u.Username,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
}

// checkJWT validates the bearer token and returns the active user behind
// it. An account that has since been disabled or deleted rejects the
// request even if the token has not yet expired.
func (a *Authenticator) checkJWT(c *gin.Context) (bool, *model.User) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false, nil
	}
	raw := strings.TrimPrefix(header, "Bearer ")
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.secret, nil
	})
	if err != nil || !tok.Valid {
		return false, nil
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return false, nil
	}
	uidFloat, _ := claims["uid"].(float64)
	uid := uint(uidFloat)
	if uid == 0 {
		return false, nil
	}
	u, err := a.users.GetUserByID(uid)
	if err != nil || u == nil || u.Disabled {
		return false, nil
	}
	return true, u
}

func (a *Authenticator) checkIP(c *gin.Context) bool {
	ip := netutil.ClientIP(c.Request, a.cfg.TrustedProxies)
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range a.cfg.AdminIPWhitelist {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func randomSecret(n int) []byte {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	out := make([]byte, hex.EncodedLen(n))
	hex.Encode(out, buf)
	return out
}

// truncate clamps a string at max UTF-8 bytes; used to keep User-Agent
// values within the DB column width without panicking on multibyte input.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
