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

// NFTables drives nftables via the `nft` CLI. We own a dedicated table
// `portpass` so rules coexist cleanly with operator-defined nftables setups
// and can be listed / flushed as a unit.
//
// Rule shape:
//   table inet portpass {
//     chain input { type filter hook input priority filter ; }
//     rule ... accept comment "portpass:<id>"
//   }
//
// Using the `inet` family lets a single table match IPv4 and IPv6; source
// IP address family is discriminated via `ip saddr` / `ip6 saddr`.
type NFTables struct {
	mu sync.Mutex
}

func NewNFTables() *NFTables { return &NFTables{} }

func (d *NFTables) Name() string { return "nftables" }

func (d *NFTables) HealthCheck() error {
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("nft not found in PATH: %w", err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	// Ensure the dedicated table + chain exist so subsequent Apply() works.
	script := "add table inet portpass\n" +
		"add chain inet portpass input { type filter hook input priority filter ; }\n"
	return runStdin(script, "nft", "-f", "-")
}

func (d *NFTables) Apply(r *model.Rule) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tag := CommentTag(r.ID)
	lines := make([]string, 0, 2)
	for _, proto := range expandProto(r.Protocol) {
		lines = append(lines, fmt.Sprintf(
			`add rule inet portpass input %s %s dport %d accept comment "%s"`,
			saddrExpr(r.SourceIP), proto, r.Port, tag,
		))
	}
	if err := runStdin(strings.Join(lines, "\n"), "nft", "-f", "-"); err != nil {
		return "", err
	}
	return "inet/portpass", nil
}

func (d *NFTables) Remove(r *model.Rule) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tag := CommentTag(r.ID)
	handles, err := d.findHandles(tag)
	if err != nil {
		return err
	}
	for _, h := range handles {
		if err := run("nft", "delete", "rule", "inet", "portpass", "input", "handle", strconv.Itoa(h)); err != nil {
			return err
		}
	}
	return nil
}

func (d *NFTables) List() ([]Applied, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "portpass", "input").Output()
	if err != nil {
		return nil, nil // treat missing table as empty, matches iptables behaviour
	}
	return parseNftList(string(out)), nil
}

// findHandles parses `nft -a list` to recover the handle IDs for every
// rule carrying the given comment tag. Handles are the only stable way
// to delete rules in nftables.
func (d *NFTables) findHandles(tag string) ([]int, error) {
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "portpass", "input").Output()
	if err != nil {
		return nil, nil
	}
	var handles []int
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, `"`+tag+`"`) {
			continue
		}
		if h, ok := extractHandle(line); ok {
			handles = append(handles, h)
		}
	}
	return handles, nil
}

var nftHandleRE = regexp.MustCompile(`# handle (\d+)`)

func extractHandle(line string) (int, bool) {
	m := nftHandleRE.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0, false
	}
	h, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return h, true
}

// parseNftList is the reconciliation counterpart of findHandles.
func parseNftList(out string) []Applied {
	var applied []Applied
	reDport := regexp.MustCompile(`dport (\d+)`)
	reProto := regexp.MustCompile(`\b(tcp|udp)\b`)
	reAddr := regexp.MustCompile(`(?:ip|ip6) saddr ([0-9a-fA-F\.:\/]+)`)
	reComment := regexp.MustCompile(`comment "(portpass:\d+)"`)
	for _, line := range strings.Split(out, "\n") {
		c := reComment.FindStringSubmatch(line)
		if len(c) != 2 {
			continue
		}
		id, ok := ParseCommentTag(c[1])
		if !ok {
			continue
		}
		a := Applied{CommentTag: c[1], RuleID: id}
		if m := reDport.FindStringSubmatch(line); len(m) == 2 {
			if p, err := strconv.Atoi(m[1]); err == nil {
				a.Port = p
			}
		}
		if m := reProto.FindStringSubmatch(line); len(m) == 2 {
			a.Protocol = m[1]
		}
		if m := reAddr.FindStringSubmatch(line); len(m) == 2 {
			a.SourceIP = m[1]
		}
		applied = append(applied, a)
	}
	return applied
}

// saddrExpr returns the correct source-address matcher for an IPv4 or
// IPv6 rule; the empty case ("any source") returns an empty string.
func saddrExpr(src string) string {
	if canonicalSource(src) == "" {
		return ""
	}
	if strings.Contains(src, ":") {
		return fmt.Sprintf("ip6 saddr %s", src)
	}
	return fmt.Sprintf("ip saddr %s", src)
}

// runStdin invokes bin with args and feeds `stdin` on standard input.
// Used by drivers that need to send scripts (nft, firewalld) rather than
// command-line arguments.
func runStdin(stdin, bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %w: %s", bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
