package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/teacat99/PortPass/internal/api"
	"github.com/teacat99/PortPass/internal/auth"
	"github.com/teacat99/PortPass/internal/captcha"
	"github.com/teacat99/PortPass/internal/config"
	"github.com/teacat99/PortPass/internal/firewall"
	"github.com/teacat99/PortPass/internal/lifecycle"
	"github.com/teacat99/PortPass/internal/notify"
	"github.com/teacat99/PortPass/internal/runtime"
	"github.com/teacat99/PortPass/internal/store"
	"github.com/teacat99/PortPass/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	dbPath := filepath.Join(cfg.DataDir, "portpass.db")

	s, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	if err := s.SeedPresetCategories(); err != nil {
		log.Fatalf("seed preset categories: %v", err)
	}
	if err := s.SeedPresetPorts(); err != nil {
		log.Fatalf("seed presets: %v", err)
	}
	adminID, err := s.SeedAdminIfEmpty(cfg.AdminUsername, cfg.AdminPassword)
	if err != nil {
		log.Fatalf("seed admin: %v", err)
	}
	adminUsername := cfg.AdminUsername
	if adminUsername == "" {
		adminUsername = store.DefaultAdminUsername
	}

	drv, err := firewall.NewDriver(cfg.FirewallDriver)
	if err != nil {
		log.Fatalf("firewall driver: %v", err)
	}
	if err := drv.HealthCheck(); err != nil {
		log.Fatalf("firewall healthcheck: %v", err)
	}
	log.Printf("firewall driver: %s", drv.Name())

	lm := lifecycle.New(s, drv, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := lm.Start(ctx); err != nil {
		log.Fatalf("lifecycle start: %v", err)
	}

	// runtime.Settings holds every hot-mutable knob; load once from
	// env defaults, then overlay the operator's persisted KV values.
	rt := runtime.New(cfg)
	if err := rt.LoadFromKV(s.LookupSetting); err != nil {
		log.Printf("[runtime] load persisted settings: %v (continuing with env defaults)", err)
	}

	notifier := notify.New(rt)
	captchaSvc := captcha.New(rt, s)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	authn := auth.New(cfg, rt, s)
	authn.SetSystemAdmin(adminID, adminUsername)
	authn.SetCaptcha(captchaSvc)
	authn.SetNotifier(notifier)
	server := api.New(cfg, rt, s, lm, authn, captchaSvc, notifier)
	server.Router(r)

	mountStatic(r)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("PortPass listening on %s (auth=%s, driver=%s)", cfg.Listen, cfg.AuthMode, drv.Name())
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down (firewall rules retained for next boot)")

	shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	_ = httpSrv.Shutdown(shutdownCtx)
	lm.Stop()
}

// mountStatic wires the embedded frontend assets on top of the Gin router.
// When no dist is present (dev mode before M3) a helpful placeholder is
// returned instead of a 404 so operators can tell the server is running.
func mountStatic(r *gin.Engine) {
	sub, err := fs.Sub(web.FS, "dist")
	if err != nil {
		log.Printf("[web] embed not available: %v", err)
		r.NoRoute(func(c *gin.Context) { c.String(http.StatusOK, "PortPass backend is running. Frontend will ship in M2.") })
		return
	}
	r.NoRoute(func(c *gin.Context) {
		path := strings.TrimPrefix(c.Request.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := sub.Open(path)
		if err == nil {
			stat, _ := f.Stat()
			if stat != nil && !stat.IsDir() {
				http.ServeFileFS(c.Writer, c.Request, sub, path)
				return
			}
		}
		if data, err := fs.ReadFile(sub, "index.html"); err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
			return
		}
		c.String(http.StatusOK, "PortPass backend is running. Frontend will ship in M2.")
	})
}
