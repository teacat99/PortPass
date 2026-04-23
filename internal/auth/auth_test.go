package auth

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/teacat99/PortPass/internal/config"
	"github.com/teacat99/PortPass/internal/model"
)

func init() { gin.SetMode(gin.TestMode) }

func mustCIDR(s string) *net.IPNet {
	_, n, _ := net.ParseCIDR(s)
	return n
}

// fakeUserRepo is an in-memory UserRepo used by the tests; it keeps the
// test surface free of SQLite. Index by id and by username.
type fakeUserRepo struct {
	byID   map[uint]*model.User
	byName map[string]*model.User
}

func newFakeRepo(us ...*model.User) *fakeUserRepo {
	r := &fakeUserRepo{byID: map[uint]*model.User{}, byName: map[string]*model.User{}}
	for _, u := range us {
		r.byID[u.ID] = u
		r.byName[u.Username] = u
	}
	return r
}

func (r *fakeUserRepo) GetUserByUsername(name string) (*model.User, error) {
	return r.byName[name], nil
}

func (r *fakeUserRepo) GetUserByID(id uint) (*model.User, error) {
	return r.byID[id], nil
}

func hash(pw string) string {
	h, _ := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	return string(h)
}

func newEngine(a *Authenticator) *gin.Engine {
	r := gin.New()
	pub := r.Group("/api")
	pub.POST("/auth/login", a.LoginHandler)
	pub.GET("/auth/status", a.StatusHandler)
	g := r.Group("/api", a.Middleware())
	g.GET("/ping", func(c *gin.Context) {
		id, name, role := Principal(c)
		c.JSON(200, gin.H{"id": id, "name": name, "role": role})
	})
	return r
}

func TestPasswordMode_LoginFlow(t *testing.T) {
	cfg := &config.Config{AuthMode: config.AuthModePassword, AdminUsername: "admin"}
	admin := &model.User{ID: 1, Username: "admin", PasswordHash: hash("secret"), Role: model.RoleAdmin}
	a := New(cfg, newFakeRepo(admin))
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
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("want 401 got %d", w.Code)
	}

	// correct password (with username) -> token
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("login: want 200 got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" || resp.Role != model.RoleAdmin {
		t.Fatalf("unexpected login payload: %+v", resp)
	}

	// token accepted -> 200 with principal
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/ping", nil)
	req.Header.Set("Authorization", "Bearer "+resp.Token)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("authed: want 200 got %d", w.Code)
	}
	var p struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID != 1 || p.Name != "admin" || p.Role != model.RoleAdmin {
		t.Fatalf("principal propagation: %+v", p)
	}

	// legacy client (no username) still works: falls back to cfg.AdminUsername
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("legacy login: want 200 got %d body=%s", w.Code, w.Body.String())
	}
}

func TestPasswordMode_DisabledUserRejected(t *testing.T) {
	cfg := &config.Config{AuthMode: config.AuthModePassword, AdminUsername: "admin"}
	admin := &model.User{ID: 1, Username: "admin", PasswordHash: hash("secret"), Role: model.RoleAdmin}
	disabled := &model.User{ID: 2, Username: "alice", PasswordHash: hash("s3cret"), Role: model.RoleUser, Disabled: true}
	a := New(cfg, newFakeRepo(admin, disabled))
	r := newEngine(a)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"alice","password":"s3cret"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("disabled user: want 401 got %d", w.Code)
	}
}

func TestIPWhitelistMode_SystemAdminPrincipal(t *testing.T) {
	cfg := &config.Config{
		AuthMode:         config.AuthModeIPWhitelist,
		AdminIPWhitelist: []*net.IPNet{mustCIDR("10.0.0.0/24")},
	}
	admin := &model.User{ID: 7, Username: "sys", Role: model.RoleAdmin}
	a := New(cfg, newFakeRepo(admin))
	a.SetSystemAdmin(admin.ID, admin.Username)
	r := newEngine(a)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/ping", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("whitelisted IP: want 200 got %d", w.Code)
	}
	var p struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID != 7 || p.Role != model.RoleAdmin {
		t.Fatalf("principal: %+v", p)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/ping", nil)
	req.RemoteAddr = "203.0.113.9:1"
	r.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("non-whitelisted IP: want 403 got %d", w.Code)
	}
}

func TestNoneMode_SystemAdminPrincipal(t *testing.T) {
	cfg := &config.Config{AuthMode: config.AuthModeNone}
	admin := &model.User{ID: 42, Username: "system", Role: model.RoleAdmin}
	a := New(cfg, newFakeRepo(admin))
	a.SetSystemAdmin(admin.ID, admin.Username)
	r := newEngine(a)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("none mode: want 200 got %d", w.Code)
	}
	var p struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.ID != 42 {
		t.Fatalf("system admin id propagated: %+v", p)
	}
}
