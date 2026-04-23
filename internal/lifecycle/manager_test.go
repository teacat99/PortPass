package lifecycle

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/teacat99/PortPass/internal/firewall"
	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s
}

func TestScheduleApplyAndExpire(t *testing.T) {
	s := newTestStore(t)
	drv := firewall.NewMock()
	m := New(s, drv, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop()

	r := &model.Rule{
		SourceIP: "1.2.3.4/32",
		Port:     22,
		Protocol: model.ProtoTCP,
		ExpireAt: time.Now().Add(80 * time.Millisecond),
		Status:   model.StatusPending,
	}
	if err := s.CreateRule(r); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	if err := m.Schedule(r); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	live, _ := drv.List()
	if len(live) != 1 {
		t.Fatalf("expected 1 live rule, got %d", len(live))
	}

	time.Sleep(250 * time.Millisecond)
	live, _ = drv.List()
	if len(live) != 0 {
		t.Fatalf("expected rule expired, still %d live", len(live))
	}
	got, _ := s.GetRule(r.ID)
	if got.Status != model.StatusExpired {
		t.Fatalf("expected status=expired, got %q", got.Status)
	}
}

func TestRevokeRemovesImmediately(t *testing.T) {
	s := newTestStore(t)
	drv := firewall.NewMock()
	m := New(s, drv, time.Hour)
	defer m.Stop()

	r := &model.Rule{
		SourceIP: "1.2.3.4/32", Port: 80, Protocol: model.ProtoTCP,
		ExpireAt: time.Now().Add(time.Hour), Status: model.StatusPending,
	}
	if err := s.CreateRule(r); err != nil {
		t.Fatal(err)
	}
	if err := m.Schedule(r); err != nil {
		t.Fatal(err)
	}
	if err := m.Revoke(r); err != nil {
		t.Fatal(err)
	}
	live, _ := drv.List()
	if len(live) != 0 {
		t.Fatalf("expected 0 live rules after revoke, got %d", len(live))
	}
	if r.Status != model.StatusRevoked {
		t.Fatalf("expected revoked status, got %q", r.Status)
	}
}

func TestReconcileReappliesMissingRule(t *testing.T) {
	s := newTestStore(t)
	drv := firewall.NewMock()
	m := New(s, drv, time.Hour)
	defer m.Stop()

	r := &model.Rule{
		SourceIP: "1.2.3.4/32", Port: 443, Protocol: model.ProtoTCP,
		ExpireAt: time.Now().Add(time.Hour), Status: model.StatusPending,
	}
	if err := s.CreateRule(r); err != nil {
		t.Fatal(err)
	}
	if err := m.Schedule(r); err != nil {
		t.Fatal(err)
	}
	// Simulate external deletion (e.g. operator manually ran iptables -F)
	_ = drv.Remove(r)

	if err := m.Reconcile(); err != nil {
		t.Fatal(err)
	}
	live, _ := drv.List()
	if len(live) != 1 {
		t.Fatalf("reconcile should re-apply missing rule, live=%d", len(live))
	}
}

func TestReconcileCleansOrphan(t *testing.T) {
	s := newTestStore(t)
	drv := firewall.NewMock()
	m := New(s, drv, time.Hour)
	defer m.Stop()

	// Inject a firewall rule with no DB counterpart.
	orphan := &model.Rule{ID: 99, SourceIP: "8.8.8.8/32", Port: 53, Protocol: model.ProtoTCP}
	_, _ = drv.Apply(orphan)

	if err := m.Reconcile(); err != nil {
		t.Fatal(err)
	}
	live, _ := drv.List()
	if len(live) != 0 {
		t.Fatalf("orphan should be cleaned, got %d", len(live))
	}
}

func TestReconcileExpiresOverdueRule(t *testing.T) {
	s := newTestStore(t)
	drv := firewall.NewMock()
	m := New(s, drv, time.Hour)
	defer m.Stop()

	r := &model.Rule{
		SourceIP: "1.2.3.4/32", Port: 22, Protocol: model.ProtoTCP,
		ExpireAt: time.Now().Add(-1 * time.Minute), Status: model.StatusActive,
		DriverName: "mock", CommentTag: "portpass:1",
	}
	if err := s.CreateRule(r); err != nil {
		t.Fatal(err)
	}
	_, _ = drv.Apply(r)

	if err := m.Reconcile(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRule(r.ID)
	if got.Status != model.StatusExpired {
		t.Fatalf("overdue rule not marked expired, status=%q", got.Status)
	}
	live, _ := drv.List()
	if len(live) != 0 {
		t.Fatalf("expected 0 live rules, got %d", len(live))
	}
}
