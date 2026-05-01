package lifecycle

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/teacat99/PortPass/internal/firewall"
	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/store"
)

// Manager owns every time.AfterFunc timer for active rules and runs periodic
// reconciliation to keep the persisted state and live firewall state in sync.
//
// Reliability strategy (see plan.md §"规则生命周期管理"):
//  1. Primary channel: AfterFunc fires at expire_at and removes the rule.
//  2. Fallback: every ReconcileInterval the manager scans DB vs. driver and
//     fixes any drift (expired-but-present rules, clock skew after sleep,
//     rules manually deleted by an operator, orphaned rules whose DB row
//     was lost).
//  3. Boot: Start() reconciles once synchronously so the in-memory state
//     matches reality before the HTTP server begins accepting requests.
//  4. Shutdown: Stop() cancels timers but does NOT remove firewall rules,
//     so a container restart is not perceived as a revocation. The next
//     boot reconciles and re-schedules.
//
// Cleanup-on-expire (per-rule opt-in): when a rule carries
// CleanupOnExpire=true, the manager additionally drops every conntrack
// entry permitted by that rule after Remove() returns. Failures are
// logged but never block the rule's status transition - the firewall
// row going away is the contract; severing live flows is best-effort.
type Manager struct {
	store    *store.Store
	driver   firewall.Driver
	interval time.Duration

	mu     sync.Mutex
	timers map[uint]*time.Timer

	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates a Manager. ReconcileInterval defaults to 30s when zero.
func New(s *store.Store, d firewall.Driver, reconcileInterval time.Duration) *Manager {
	if reconcileInterval <= 0 {
		reconcileInterval = 30 * time.Second
	}
	return &Manager{
		store:    s,
		driver:   d,
		interval: reconcileInterval,
		timers:   make(map[uint]*time.Timer),
		stopCh:   make(chan struct{}),
	}
}

// Start performs initial reconciliation and launches the background ticker.
// Returns the first reconciliation error; the ticker loop swallows errors
// after logging them so a transient failure never kills the manager.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.Reconcile(); err != nil {
		return err
	}
	go m.loop(ctx)
	return nil
}

// Stop cancels every scheduled timer but leaves firewall rules in place.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.timers {
		t.Stop()
		delete(m.timers, id)
	}
}

// Schedule applies the rule to the firewall and registers an expiration
// timer. Called by the API layer after a successful CreateRule.
func (m *Manager) Schedule(rule *model.Rule) error {
	ref, err := m.driver.Apply(rule)
	if err != nil {
		rule.Status = model.StatusFailed
		_ = m.store.UpdateRule(rule)
		return err
	}
	rule.DriverName = m.driver.Name()
	rule.DriverRef = ref
	rule.Status = model.StatusActive
	if err := m.store.UpdateRule(rule); err != nil {
		_ = m.driver.Remove(rule)
		return err
	}
	m.armTimer(rule.ID, time.Until(rule.ExpireAt))
	return nil
}

// Extend updates the scheduled expiration. The firewall rule itself does not
// need to change because drivers don't embed TTL; only the in-memory timer
// and the DB row are touched.
func (m *Manager) Extend(rule *model.Rule, newExpire time.Time) error {
	rule.ExpireAt = newExpire
	if err := m.store.UpdateRule(rule); err != nil {
		return err
	}
	m.armTimer(rule.ID, time.Until(newExpire))
	return nil
}

// Revoke removes the rule immediately (UI "提前终止" button). The
// `cleanup` argument controls whether we also drop existing conntrack
// entries for this rule's (src, port, proto) tuple, regardless of the
// rule's own CleanupOnExpire flag - the manual Revoke flow exposes
// the choice in a confirmation dialog so the operator decides every
// time, while the rule-level flag governs all *automatic* removals
// (auto expiry / reconcile-driven cleanup).
func (m *Manager) Revoke(rule *model.Rule, cleanup bool) error {
	m.cancelTimer(rule.ID)
	if err := m.driver.Remove(rule); err != nil {
		return err
	}
	now := time.Now()
	rule.Status = model.StatusRevoked
	rule.TerminatedAt = &now
	if cleanup {
		rule.LastCleanupCount = m.runCleanup(rule, "revoke")
	}
	return m.store.UpdateRule(rule)
}

// Reconcile is exported so tests and the HTTP /api/health probe can trigger
// a forced pass. It is safe to call concurrently with Schedule/Revoke.
func (m *Manager) Reconcile() error {
	active, err := m.store.ListActiveRules()
	if err != nil {
		return err
	}
	live, err := m.driver.List()
	if err != nil {
		return err
	}
	liveByID := make(map[uint]firewall.Applied, len(live))
	for _, a := range live {
		liveByID[a.RuleID] = a
	}

	now := time.Now()
	dbSeen := make(map[uint]struct{}, len(active))
	for i := range active {
		r := &active[i]
		dbSeen[r.ID] = struct{}{}

		if !r.ExpireAt.After(now) {
			m.cancelTimer(r.ID)
			if _, present := liveByID[r.ID]; present {
				if err := m.driver.Remove(r); err != nil {
					log.Printf("[reconcile] remove expired rule %d failed: %v", r.ID, err)
					continue
				}
			}
			terminated := now
			r.Status = model.StatusExpired
			r.TerminatedAt = &terminated
			if r.CleanupOnExpire {
				r.LastCleanupCount = m.runCleanup(r, "reconcile")
			}
			if err := m.store.UpdateRule(r); err != nil {
				log.Printf("[reconcile] mark expired rule %d failed: %v", r.ID, err)
			}
			continue
		}

		if _, present := liveByID[r.ID]; !present {
			ref, err := m.driver.Apply(r)
			if err != nil {
				log.Printf("[reconcile] re-apply rule %d failed: %v", r.ID, err)
				continue
			}
			r.DriverRef = ref
			r.DriverName = m.driver.Name()
			_ = m.store.UpdateRule(r)
			log.Printf("[reconcile] re-applied missing rule %d", r.ID)
		}
		m.armTimer(r.ID, time.Until(r.ExpireAt))
	}

	for id, a := range liveByID {
		if _, ok := dbSeen[id]; ok {
			continue
		}
		orphan := &model.Rule{ID: id, SourceIP: a.SourceIP, Port: a.Port, Protocol: a.Protocol}
		if err := m.driver.Remove(orphan); err != nil {
			log.Printf("[reconcile] remove orphan %d failed: %v", id, err)
			continue
		}
		log.Printf("[reconcile] cleaned orphan firewall rule %d", id)
	}
	return nil
}

func (m *Manager) loop(ctx context.Context) {
	t := time.NewTicker(m.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-t.C:
			if err := m.Reconcile(); err != nil {
				log.Printf("[reconcile] %v", err)
			}
		}
	}
}

// armTimer (re)installs the expiration timer for a rule.
func (m *Manager) armTimer(ruleID uint, d time.Duration) {
	if d < 0 {
		d = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.timers[ruleID]; ok {
		existing.Stop()
	}
	m.timers[ruleID] = time.AfterFunc(d, func() { m.onExpire(ruleID) })
}

func (m *Manager) cancelTimer(ruleID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[ruleID]; ok {
		t.Stop()
		delete(m.timers, ruleID)
	}
}

func (m *Manager) onExpire(ruleID uint) {
	r, err := m.store.GetRule(ruleID)
	if err != nil || r == nil {
		return
	}
	if r.Status != model.StatusActive && r.Status != model.StatusPending {
		return
	}
	if err := m.driver.Remove(r); err != nil {
		log.Printf("[expire] remove rule %d failed: %v", ruleID, err)
		return
	}
	now := time.Now()
	r.Status = model.StatusExpired
	r.TerminatedAt = &now
	if r.CleanupOnExpire {
		r.LastCleanupCount = m.runCleanup(r, "expire")
	}
	if err := m.store.UpdateRule(r); err != nil {
		log.Printf("[expire] update rule %d failed: %v", ruleID, err)
	}
	m.mu.Lock()
	delete(m.timers, ruleID)
	m.mu.Unlock()
}

// runCleanup invokes firewall.FlushConntrack and returns the number of
// connection-tracking entries that were dropped. Failures are logged
// at warn level (so the operator can spot a missing conntrack-tools
// install) but never bubble up: the firewall ACCEPT row is already
// gone, so blocking new connections is intact - severing established
// flows is best-effort. `phase` is a free-form tag so the same helper
// is reusable for revoke / expire / reconcile log lines.
func (m *Manager) runCleanup(rule *model.Rule, phase string) int {
	count, err := firewall.FlushConntrack(rule)
	switch {
	case err == nil:
		if count > 0 {
			log.Printf("[%s] flushed %d conntrack entries for rule %d", phase, count, rule.ID)
		}
	case errors.Is(err, firewall.ErrConntrackUnavailable):
		log.Printf("[%s] cleanup requested for rule %d but conntrack binary not installed; skipping", phase, rule.ID)
	default:
		log.Printf("[%s] cleanup failed for rule %d: %v", phase, rule.ID, err)
	}
	return count
}
