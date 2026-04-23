package firewall

import (
	"fmt"
	"strings"

	"github.com/teacat99/PortPass/internal/model"
)

// Driver abstracts an OS-level firewall backend capable of inserting,
// removing and listing rules tagged by PortPass. Multiple implementations are
// provided so PortPass can run on most Linux distributions out-of-the-box.
//
// All drivers MUST attach a stable comment `portpass:<id>` to rules they own
// so reconciliation can match Rule rows against live firewall state without
// interfering with unrelated rules the operator has configured.
type Driver interface {
	// Name returns a stable identifier (e.g. "iptables", "nftables").
	Name() string
	// HealthCheck verifies the backend is usable at startup; returning an
	// error aborts boot so the operator sees a clear failure.
	HealthCheck() error
	// Apply inserts a rule and returns an opaque reference string that the
	// driver can later use to locate the rule during Remove/Verify.
	Apply(rule *model.Rule) (ref string, err error)
	// Remove deletes the rule previously installed by Apply. Implementations
	// must be idempotent: removing a non-existent rule is not an error.
	Remove(rule *model.Rule) error
	// List returns every rule currently present in the firewall that carries
	// the PortPass comment prefix. Used by reconciliation.
	List() ([]Applied, error)
}

// Applied is a view of a live firewall rule carrying a PortPass comment.
type Applied struct {
	CommentTag string // e.g. "portpass:42"
	RuleID     uint   // parsed from CommentTag
	SourceIP   string
	Port       int
	Protocol   string
}

// CommentTag formats the canonical comment for a Rule. Kept in a single
// place so every driver prints identical tags.
func CommentTag(ruleID uint) string {
	return fmt.Sprintf("portpass:%d", ruleID)
}

// ParseCommentTag is the inverse of CommentTag. Returns (id, true) on
// success; (0, false) if the comment doesn't match the PortPass prefix.
func ParseCommentTag(comment string) (uint, bool) {
	const prefix = "portpass:"
	if !strings.HasPrefix(comment, prefix) {
		return 0, false
	}
	var id uint
	if _, err := fmt.Sscanf(comment[len(prefix):], "%d", &id); err != nil {
		return 0, false
	}
	return id, true
}

// NewDriver constructs a driver by name. Unknown names return an error so
// operators get immediate feedback on typos.
func NewDriver(name string) (Driver, error) {
	switch strings.ToLower(name) {
	case "iptables":
		return NewIPTables(), nil
	case "nftables":
		return NewNFTables(), nil
	case "ufw":
		return NewUFW(), nil
	case "firewalld":
		return NewFirewalld(), nil
	case "mock":
		return NewMock(), nil
	default:
		return nil, fmt.Errorf("unknown firewall driver %q", name)
	}
}
