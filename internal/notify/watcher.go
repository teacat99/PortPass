package notify

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/teacat99/PortPass/internal/model"
	"github.com/teacat99/PortPass/internal/runtime"
	"github.com/teacat99/PortPass/internal/store"
)

// ExpiryWatcher periodically scans the rules table and pushes ntfy
// reminders for rules whose lead-time threshold has elapsed but were
// not yet flagged as ntfy-sent. The browser side is driven by the
// frontend polling /api/notify/pending instead — keeping the two
// pipelines independent means the operator's "browser + ntfy" choice
// genuinely fires both, and a temporary ntfy outage never silences
// the local Notification.
type ExpiryWatcher struct {
	rt       *runtime.Settings
	store    *store.Store
	ntfy     *Ntfy
	interval time.Duration

	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewExpiryWatcher wires the dependencies. interval defaults to 30s
// when zero, matching the lifecycle reconcile cadence so a single
// laggy clock cycle is the worst-case timing skew.
func NewExpiryWatcher(rt *runtime.Settings, s *store.Store, n *Ntfy, interval time.Duration) *ExpiryWatcher {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &ExpiryWatcher{
		rt:       rt,
		store:    s,
		ntfy:     n,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start runs the watcher in a background goroutine until ctx is done
// or Stop is called. The first scan fires after the first tick so
// boot is not slowed by any potentially slow ntfy round-trip.
func (w *ExpiryWatcher) Start(ctx context.Context) {
	go w.loop(ctx)
}

// Stop signals the watcher to exit. Safe to call multiple times.
func (w *ExpiryWatcher) Stop() {
	w.stopOnce.Do(func() { close(w.stopCh) })
}

func (w *ExpiryWatcher) loop(ctx context.Context) {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-t.C:
			if err := w.tick(time.Now()); err != nil {
				log.Printf("[notify-watcher] %v", err)
			}
		}
	}
}

// tick is exported for tests so they can drive the cycle without
// waiting for the ticker.
func (w *ExpiryWatcher) tick(now time.Time) error {
	if !w.rt.NotifyChannelIncludes(runtime.NotifyChannelNtfy) {
		return nil
	}
	if w.ntfy == nil {
		return nil
	}
	rules, err := w.store.ListPendingNotify(0, store.NotifyChannelNtfy, now)
	if err != nil {
		return fmt.Errorf("list pending: %w", err)
	}
	if len(rules) == 0 {
		return nil
	}
	delivered := make([]uint, 0, len(rules))
	for i := range rules {
		r := &rules[i]
		title, body := buildExpiryMessage(r, now)
		if err := w.ntfy.NotifyExpiry(title, body, "alarm_clock"); err != nil {
			log.Printf("[notify-watcher] rule %d push failed: %v", r.ID, err)
			continue
		}
		delivered = append(delivered, r.ID)
	}
	if len(delivered) == 0 {
		return nil
	}
	if _, err := w.store.MarkNotifySent(delivered, store.NotifyChannelNtfy, 0, now); err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

// buildExpiryMessage formats the user-facing ntfy title + body. We
// keep the content compact because the ntfy push is delivered to a
// phone where line breaks count: title gives the operator the gist,
// body fills in the details a glance can use.
func buildExpiryMessage(r *model.Rule, now time.Time) (string, string) {
	remaining := r.ExpireAt.Sub(now).Round(time.Second)
	if remaining < 0 {
		remaining = 0
	}
	ports := r.Ports
	if ports == "" {
		ports = fmt.Sprintf("%d", r.Port)
	}
	title := fmt.Sprintf("PortPass · 规则即将到期 / Rule expiring (%s/%s)", ports, r.Protocol)
	parts := []string{
		fmt.Sprintf("剩余 / remaining: %s", formatDuration(remaining)),
		fmt.Sprintf("来源 / source: %s", r.SourceIP),
	}
	if r.Note != "" {
		parts = append(parts, fmt.Sprintf("备注 / note: %s", r.Note))
	}
	parts = append(parts, fmt.Sprintf("到期 / expire: %s", r.ExpireAt.Format(time.RFC3339)))
	return title, strings.Join(parts, "\n")
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	if mins == 0 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%02ds", mins, secs)
}
