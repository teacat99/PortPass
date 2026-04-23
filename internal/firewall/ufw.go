package firewall

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/teacat99/PortPass/internal/model"
)

// UFW drives Uncomplicated Firewall via the `ufw` CLI. UFW itself wraps
// iptables/nftables, so this driver is essentially a convenience wrapper
// for systems where the operator has already enabled ufw and expects new
// rules to survive `ufw reload`.
//
// PortPass appends its rules with a `comment "portpass:<id>"` so they are
// visible in `ufw status verbose` and can be recovered during reconcile.
type UFW struct {
	mu sync.Mutex
}

func NewUFW() *UFW { return &UFW{} }

func (d *UFW) Name() string { return "ufw" }

func (d *UFW) HealthCheck() error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("ufw not found in PATH: %w", err)
	}
	return run("ufw", "status")
}

func (d *UFW) Apply(r *model.Rule) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tag := CommentTag(r.ID)
	for _, proto := range expandProto(r.Protocol) {
		args := []string{"allow"}
		if src := canonicalSource(r.SourceIP); src != "" {
			args = append(args, "from", src, "to", "any")
		}
		args = append(args, "port", strconv.Itoa(r.Port), "proto", proto, "comment", tag)
		if err := run("ufw", args...); err != nil {
			return "", err
		}
	}
	return "ufw", nil
}

func (d *UFW) Remove(r *model.Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tag := CommentTag(r.ID)
	for {
		num := d.findRuleNumber(tag)
		if num <= 0 {
			return nil
		}
		if err := runStdin("y\n", "ufw", "--force", "delete", strconv.Itoa(num)); err != nil {
			return err
		}
	}
}

func (d *UFW) List() ([]Applied, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out, err := exec.Command("ufw", "status", "numbered").Output()
	if err != nil {
		return nil, nil
	}
	return parseUFWStatus(string(out)), nil
}

// findRuleNumber parses `ufw status numbered` to find the FIRST line whose
// comment matches `tag` and returns its numeric index. UFW's delete-by-number
// is 1-based and shifts the numbering on each delete, so we loop in Remove.
func (d *UFW) findRuleNumber(tag string) int {
	out, err := exec.Command("ufw", "status", "numbered").Output()
	if err != nil {
		return 0
	}
	re := regexp.MustCompile(`^\[\s*(\d+)\]`)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, tag) {
			continue
		}
		if m := re.FindStringSubmatch(strings.TrimSpace(line)); len(m) == 2 {
			n, _ := strconv.Atoi(m[1])
			return n
		}
	}
	return 0
}

// parseUFWStatus extracts PortPass rules from `ufw status numbered`. The
// format is human-oriented but stable enough for our limited needs.
func parseUFWStatus(out string) []Applied {
	var applied []Applied
	re := regexp.MustCompile(`(portpass:\d+)`)
	reDetail := regexp.MustCompile(`(\d+)/(tcp|udp)`)
	for _, line := range strings.Split(out, "\n") {
		m := re.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		id, ok := ParseCommentTag(m[1])
		if !ok {
			continue
		}
		a := Applied{CommentTag: m[1], RuleID: id}
		if d := reDetail.FindStringSubmatch(line); len(d) == 3 {
			if p, err := strconv.Atoi(d[1]); err == nil {
				a.Port = p
			}
			a.Protocol = d[2]
		}
		applied = append(applied, a)
	}
	return applied
}
