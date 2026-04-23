package firewall

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/teacat99/PortPass/internal/model"
)

// IPTables implements Driver by shelling out to iptables / ip6tables. It is
// the default driver because the command is available on virtually every
// Linux distribution without extra daemons.
//
// Rule insertion strategy: rules are inserted at the top of INPUT (-I) so a
// DROP policy earlier in the chain doesn't block the opening. Removal goes
// through iptables-save to find the exact matching line (by comment) because
// iptables -D requires full parameter recall which is fragile.
type IPTables struct {
	mu sync.Mutex
}

// NewIPTables returns a fresh iptables driver. It performs no syscalls; use
// HealthCheck for that.
func NewIPTables() *IPTables { return &IPTables{} }

// Name returns the driver identifier.
func (d *IPTables) Name() string { return "iptables" }

// HealthCheck verifies the iptables binary is reachable and functional.
func (d *IPTables) HealthCheck() error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return fmt.Errorf("iptables not found in PATH: %w", err)
	}
	if err := run("iptables", "-L", "INPUT", "-n"); err != nil {
		return fmt.Errorf("iptables -L failed (need NET_ADMIN?): %w", err)
	}
	return nil
}

// Apply installs the accept rule and performs a read-back to confirm it is
// present. The returned ref encodes which binary owns the rule (iptables or
// ip6tables) so Remove can target the correct table.
func (d *IPTables) Apply(rule *model.Rule) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	bin := pickBinary(rule.SourceIP)
	tag := CommentTag(rule.ID)

	for _, proto := range expandProto(rule.Protocol) {
		args := buildArgs("-I", rule, proto, tag)
		if err := run(bin, args...); err != nil {
			_ = d.removeOne(bin, rule, proto, tag)
			return "", fmt.Errorf("%s insert failed: %w", bin, err)
		}
	}

	if ok, err := d.verify(bin, rule, tag); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("%s rule not present after insert", bin)
	}
	return bin, nil
}

// Remove deletes every line carrying rule.CommentTag. Absence is treated as
// success so reconciliation stays idempotent.
func (d *IPTables) Remove(rule *model.Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	bin := rule.DriverRef
	if bin == "" {
		bin = pickBinary(rule.SourceIP)
	}
	tag := CommentTag(rule.ID)

	for _, proto := range expandProto(rule.Protocol) {
		if err := d.removeOne(bin, rule, proto, tag); err != nil {
			return err
		}
	}
	return nil
}

// removeOne issues a single iptables -D with the exact same predicate as
// the insert. When the rule is absent iptables exits 1; we treat that as a
// no-op rather than surfacing an error to callers.
func (d *IPTables) removeOne(bin string, rule *model.Rule, proto, tag string) error {
	args := buildArgs("-D", rule, proto, tag)
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	if bytes.Contains(out, []byte("does a matching rule exist")) ||
		bytes.Contains(out, []byte("Bad rule")) ||
		bytes.Contains(out, []byte("No chain/target/match")) {
		return nil
	}
	return fmt.Errorf("%s delete failed: %v: %s", bin, err, strings.TrimSpace(string(out)))
}

// List parses iptables-save / ip6tables-save output to recover every rule
// that carries a PortPass comment. Only the INPUT chain is scanned because
// that is where PortPass installs its rules.
func (d *IPTables) List() ([]Applied, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var out []Applied
	for _, bin := range []string{"iptables-save", "ip6tables-save"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		data, err := exec.Command(bin).Output()
		if err != nil {
			continue
		}
		out = append(out, parseIPTablesSave(string(data))...)
	}
	return out, nil
}

// verify does a targeted iptables -C to confirm the rule is in-place. This
// is the "double-write check" referenced in the design doc.
func (d *IPTables) verify(bin string, rule *model.Rule, tag string) (bool, error) {
	for _, proto := range expandProto(rule.Protocol) {
		args := buildArgs("-C", rule, proto, tag)
		err := exec.Command(bin, args...).Run()
		if err != nil {
			return false, nil
		}
	}
	return true, nil
}

// buildArgs produces the argv for the three operations (-I/-D/-C) that all
// share an identical predicate. Keeping them in one place ensures an insert
// can always be matched by a delete.
func buildArgs(op string, rule *model.Rule, proto, tag string) []string {
	args := []string{op, "INPUT"}
	if op == "-I" {
		args = append(args, "1")
	}
	args = append(args, "-p", proto)
	if src := canonicalSource(rule.SourceIP); src != "" {
		args = append(args, "-s", src)
	}
	args = append(args, "--dport", strconv.Itoa(rule.Port))
	args = append(args, "-m", "comment", "--comment", tag)
	args = append(args, "-j", "ACCEPT")
	return args
}

// pickBinary chooses iptables vs ip6tables based on the source IP. A
// colon-containing IP is IPv6; everything else (including "0.0.0.0/0" and
// missing values) uses the IPv4 binary.
func pickBinary(sourceIP string) string {
	if strings.Contains(sourceIP, ":") {
		return "ip6tables"
	}
	return "iptables"
}

// canonicalSource strips "0.0.0.0/0" or "::/0" because iptables treats the
// absence of -s as "any" and the zero-CIDR form is sometimes rejected.
func canonicalSource(src string) string {
	s := strings.TrimSpace(src)
	if s == "" || s == "0.0.0.0/0" || s == "::/0" {
		return ""
	}
	return s
}

// expandProto returns the concrete protocol list to iterate over. "both"
// becomes two separate rules because iptables doesn't accept multiple -p
// values in one invocation.
func expandProto(p string) []string {
	switch strings.ToLower(p) {
	case model.ProtoBoth:
		return []string{model.ProtoTCP, model.ProtoUDP}
	case model.ProtoUDP:
		return []string{model.ProtoUDP}
	default:
		return []string{model.ProtoTCP}
	}
}

// parseIPTablesSave extracts every -A INPUT line that carries a PortPass
// comment and translates it into an Applied record.
func parseIPTablesSave(save string) []Applied {
	var out []Applied
	for _, line := range strings.Split(save, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-A INPUT") {
			continue
		}
		if !strings.Contains(line, "portpass:") {
			continue
		}
		a, ok := parseSaveLine(line)
		if !ok {
			continue
		}
		out = append(out, a)
	}
	return out
}

// parseSaveLine tokenises one iptables-save rule line. The format is stable
// across iptables versions for the small option set PortPass uses.
func parseSaveLine(line string) (Applied, bool) {
	var a Applied
	tokens := strings.Fields(line)
	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "-s":
			if i+1 < len(tokens) {
				a.SourceIP = tokens[i+1]
				i++
			}
		case "-p":
			if i+1 < len(tokens) {
				a.Protocol = tokens[i+1]
				i++
			}
		case "--dport":
			if i+1 < len(tokens) {
				if p, err := strconv.Atoi(tokens[i+1]); err == nil {
					a.Port = p
				}
				i++
			}
		case "--comment":
			if i+1 < len(tokens) {
				c := strings.Trim(tokens[i+1], `"`)
				if id, ok := ParseCommentTag(c); ok {
					a.CommentTag = c
					a.RuleID = id
				}
				i++
			}
		}
	}
	return a, a.RuleID != 0
}

// run executes a command and wraps the stderr output in the returned error.
func run(bin string, args ...string) error {
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
