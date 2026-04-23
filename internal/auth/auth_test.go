package auth

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/teacat99/PortPass/internal/config"
)

func init() { gin.SetMode(gin.TestMode) }

func mustCIDR(s string) *net.IPNet {
	_, n, _ := net.ParseCIDR(s)
	return n
}

func newEngine(a *Authenticator) *gin.Engine {
	r := gin.New()
	pub := r.Group("/api")
	pub.POST("/auth/login", a.LoginHandler)
	pub.GET("/auth/status", a.StatusHandler)
	g := r.Group("/api", a.Middleware())
	g.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
	return r
}

func TestPasswordMode_LoginFlow(t *testing.T) {
	cfg := &config.Config{AuthMode: config.AuthModePassword, AdminPassword: "secret"}
	a := New(cfg)
	r := newEngine(a)

	// unauthenticated -> 401
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}

	// wrong password -> 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}

	// correct password -> token
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login: want 200 got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct{ Token string }
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatalf("empty token")
	}

	// token accepted -> 200
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/ping", nil)
	req.Header.Set("Authorization", "Bearer "+resp.Token)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("authed: want 200 got %d", w.Code)
	}
}

func TestIPWhitelistMode(t *testing.T) {
	cfg := &config.Config{
		AuthMode:         config.AuthModeIPWhitelist,
		AdminIPWhitelist: []*net.IPNet{mustCIDR("10.0.0.0/24")},
	}
	a := New(cfg)
	r := newEngine(a)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/ping", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("whitelisted IP: want 200 got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/ping", nil)
	req.RemoteAddr = "203.0.113.9:1"
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("non-whitelisted IP: want 403 got %d", w.Code)
	}
}

func TestNoneMode(t *testing.T) {
	cfg := &config.Config{AuthMode: config.AuthModeNone}
	a := New(cfg)
	r := newEngine(a)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("none mode: want 200 got %d", w.Code)
	}
}
