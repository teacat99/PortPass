package firewall

import (
	"sync"

	"github.com/teacat99/PortPass/internal/model"
)

// Mock is an in-memory driver used for tests and for running PortPass on
// workstations that lack iptables (developer convenience). It is selected by
// setting PORTPASS_FIREWALL_DRIVER=mock.
type Mock struct {
	mu      sync.Mutex
	applied map[uint]Applied
}

// NewMock returns an empty mock driver.
func NewMock() *Mock { return &Mock{applied: map[uint]Applied{}} }

// Name returns the driver identifier.
func (d *Mock) Name() string { return "mock" }

// HealthCheck is always nil for the mock driver.
func (d *Mock) HealthCheck() error { return nil }

// Apply records the rule in memory and returns a synthetic reference.
func (d *Mock) Apply(r *model.Rule) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.applied[r.ID] = Applied{
		CommentTag: CommentTag(r.ID),
		RuleID:     r.ID,
		SourceIP:   r.SourceIP,
		Port:       r.Port,
		Protocol:   r.Protocol,
	}
	return "mock", nil
}

// Remove drops the rule from memory; missing entries are silently ignored.
func (d *Mock) Remove(r *model.Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.applied, r.ID)
	return nil
}

// List returns a snapshot of every rule currently held in memory.
func (d *Mock) List() ([]Applied, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Applied, 0, len(d.applied))
	for _, v := range d.applied {
		out = append(out, v)
	}
	return out, nil
}
