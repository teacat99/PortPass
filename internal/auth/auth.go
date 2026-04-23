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

	"github.com/teacat99/PortPass/internal/config"
	"github.com/teacat99/PortPass/internal/netutil"
)

// Authenticator bundles login and middleware logic for a specific AuthMode.
// It is constructed once during server bootstrap and reused across requests.
type Authenticator struct {
	cfg    *config.Config
	secret []byte
}

// New initialises an Authenticator. When AUTH_MODE=password and no JWT
// secret is provided, a random secret is generated so each process run
// invalidates tokens from previous runs (by design for single-user tool).
func New(cfg *config.Config) *Authenticator {
	secret := []byte(cfg.JWTSecret)
	if len(secret) == 0 {
		secret = randomSecret(32)
	}
	return &Authenticator{cfg: cfg, secret: secret}
}

// Middleware is the shared gate used by /api/* (login/status endpoints are
// mounted before this middleware so they remain reachable). Behaviour per
// mode:
//
//   - password     : requires a valid JWT in Authorization header.
//   - ipwhitelist  : requires the resolved client IP to be in the whitelist.
//   - none         : always allow.
func (a *Authenticator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch a.cfg.AuthMode {
		case config.AuthModePassword:
			if !a.checkJWT(c) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorised"})
				return
			}
		case config.AuthModeIPWhitelist:
			if !a.checkIP(c) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "ip not permitted"})
				return
			}
		case config.AuthModeNone:
			// No-op; backend intentionally unprotected (internal networks only).
		}
		c.Next()
	}
}

// LoginHandler verifies the admin password and returns a signed JWT. Only
// wired when AUTH_MODE=password.
func (a *Authenticator) LoginHandler(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if a.cfg.AuthMode != config.AuthModePassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password auth disabled"})
		return
	}
	// Constant-time comparison guards against timing attacks, even though
	// for a single-admin single-password tool it is mostly theatre.
	if !constantTimeEq(req.Password, a.cfg.AdminPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid password"})
		return
	}
	tok, err := a.sign()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": tok})
}

// StatusHandler exposes enough metadata for the frontend router to decide
// whether to redirect to /login.
func (a *Authenticator) StatusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"mode":     string(a.cfg.AuthMode),
		"required": a.cfg.AuthMode != config.AuthModeNone,
	})
}

// sign issues a JWT with a 24h expiry. Admin role is implicit - PortPass
// currently has only one role.
func (a *Authenticator) sign() (string, error) {
	claims := jwt.MapClaims{
		"sub": "admin",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(24 * time.Hour).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.secret)
}

func (a *Authenticator) checkJWT(c *gin.Context) bool {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	raw := strings.TrimPrefix(header, "Bearer ")
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.secret, nil
	})
	if err != nil || !tok.Valid {
		return false
	}
	return true
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

// constantTimeEq avoids short-circuiting that would leak password length.
func constantTimeEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
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
