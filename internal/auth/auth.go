package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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
// without booting a real SQLite database.
type UserRepo interface {
	GetUserByUsername(name string) (*model.User, error)
	GetUserByID(id uint) (*model.User, error)
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

// LoginHandler authenticates against the users table (bcrypt) and returns
// a signed JWT. Password auth is the only mode that exposes this endpoint.
func (a *Authenticator) LoginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// `code` is a stable machine-readable discriminator the frontend
		// maps to localised user-facing strings; `error` is the English
		// fallback kept for non-browser clients and log scrapers.
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_request", "error": err.Error()})
		return
	}
	if a.cfg.AuthMode != config.AuthModePassword {
		c.JSON(http.StatusBadRequest, gin.H{"code": "auth_disabled", "error": "password auth disabled"})
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
	u, err := a.users.GetUserByUsername(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "internal", "error": err.Error()})
		return
	}
	if u == nil || u.Disabled || u.PasswordHash == "" {
		// Do a dummy bcrypt compare so the response timing does not leak
		// whether the username exists.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvaOZQZQ0ZQZQZQZQZQZQZQZQZQZQZQZQO"), []byte(req.Password))
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_credentials", "error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_credentials", "error": "invalid credentials"})
		return
	}
	tok, err := a.sign(u)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":    tok,
		"username": u.Username,
		"role":     u.Role,
	})
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
