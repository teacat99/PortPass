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

// Firewalld drives firewalld via `firewall-cmd --runtime`. We install rich
// rules (supports both IPv4 and IPv6 in the "any" family) so a single call
// covers IPv4 and IPv6 sources. Rules are NOT persisted to permanent config
// because PortPass already manages persistence via its SQLite store.
//
// Rich rule shape:
//   rule family="ipv4" source address="..." port port="..." protocol="tcp" accept
//
// To recognise our own rules, we append a "log prefix" that embeds the
// PortPass tag. firewalld has no per-rule comment, so log prefix is the
// most reliable stable annotation supported across versions.
type Firewalld struct {
	mu sync.Mutex
}

func NewFirewalld() *Firewalld { return &Firewalld{} }

func (d *Firewalld) Name() string { return "firewalld" }

func (d *Firewalld) HealthCheck() error {
	if _, err := exec.LookPath("firewall-cmd"); err != nil {
		return fmt.Errorf("firewall-cmd not found in PATH: %w", err)
	}
	return run("firewall-cmd", "--state")
}

// Apply emits one rich-rule per (family, protocol) combination so the
// operator can still inspect them granularly with `firewall-cmd --list-rich-rules`.
func (d *Firewalld) Apply(r *model.Rule) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tag := CommentTag(r.ID)
	for _, proto := range expandProto(r.Protocol) {
		rule := buildRichRule(r.SourceIP, r.Port, proto, tag)
		if err := run("firewall-cmd", "--add-rich-rule="+rule); err != nil {
			return "", err
		}
	}
	return "firewalld", nil
}

func (d *Firewalld) Remove(r *model.Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tag := CommentTag(r.ID)
	rules, err := d.findRichRules(tag)
	if err != nil {
		return err
	}
	for _, rr := range rules {
		if err := run("firewall-cmd", "--remove-rich-rule="+rr); err != nil {
			return err
		}
	}
	return nil
}

func (d *Firewalld) List() ([]Applied, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out, err := exec.Command("firewall-cmd", "--list-rich-rules").Output()
	if err != nil {
		return nil, nil
	}
	return parseFirewalldRich(string(out)), nil
}

func (d *Firewalld) findRichRules(tag string) ([]string, error) {
	out, err := exec.Command("firewall-cmd", "--list-rich-rules").Output()
	if err != nil {
		return nil, nil
	}
	var matched []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, tag) {
			matched = append(matched, strings.TrimSpace(line))
		}
	}
	return matched, nil
}

// buildRichRule emits the rich-rule string. When SourceIP is empty or the
// zero CIDR we omit the source clause so the rule applies to any client.
func buildRichRule(source string, port int, proto, tag string) string {
	family := "ipv4"
	if strings.Contains(source, ":") {
		family = "ipv6"
	}
	var srcClause string
	if s := canonicalSource(source); s != "" {
		srcClause = fmt.Sprintf(` source address="%s"`, s)
	}
	return fmt.Sprintf(
		`rule family="%s"%s port port="%d" protocol="%s" log prefix="%s" level="info" limit value="1/m" accept`,
		family, srcClause, port, proto, tag,
	)
}

// parseFirewalldRich recovers PortPass rules from `--list-rich-rules`.
func parseFirewalldRich(out string) []Applied {
	var applied []Applied
	reTag := regexp.MustCompile(`prefix="?(portpass:\d+)"?`)
	rePort := regexp.MustCompile(`port="(\d+)"`)
	reProto := regexp.MustCompile(`protocol="(tcp|udp)"`)
	reAddr := regexp.MustCompile(`source address="([^"]+)"`)
	for _, line := range strings.Split(out, "\n") {
		m := reTag.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		id, ok := ParseCommentTag(m[1])
		if !ok {
			continue
		}
		a := Applied{CommentTag: m[1], RuleID: id}
		if p := rePort.FindStringSubmatch(line); len(p) == 2 {
			if n, err := strconv.Atoi(p[1]); err == nil {
				a.Port = n
			}
		}
		if p := reProto.FindStringSubmatch(line); len(p) == 2 {
			a.Protocol = p[1]
		}
		if p := reAddr.FindStringSubmatch(line); len(p) == 2 {
			a.SourceIP = p[1]
		}
		applied = append(applied, a)
	}
	return applied
}
