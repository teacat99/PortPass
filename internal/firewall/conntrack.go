package firewall

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/teacat99/PortPass/internal/model"
)

// ErrConntrackUnavailable is returned by FlushConntrack when the
// conntrack(8) binary is not installed on the host. Callers should
// treat it as a soft failure (log + carry on with rule expiry) so a
// missing optional package never wedges the lifecycle pipeline.
var ErrConntrackUnavailable = errors.New("conntrack: binary not available")

// FlushConntrack invalidates every conntrack entry that was permitted
// by `rule` so existing TCP/UDP flows stop receiving packets after
// the firewall ACCEPT row is removed. We delete entries one (proto,
// port) tuple at a time, restricting the match to the rule's exact
// source IP whenever a non-wildcard CIDR is set, so other rules /
// other clients on the same destination port stay alive.
//
// Returns the total number of conntrack entries deleted across the
// proto * port matrix. A zero count is normal (no live flows). An
// error is only returned for unrecoverable conditions; "no entries
// matched" exits 0 and contributes 0.
//
// Production caveat: deleting a conntrack entry only invalidates the
// kernel's record of the flow. If the host's INPUT chain is default-
// ACCEPT (or has no fallback DROP after the rule is removed), the next
// packet of the still-running TCP/UDP flow will be re-accepted by the
// permissive default and the kernel will rebuild the tracking entry —
// the connection effectively self-heals. To make cleanup actually tear
// the flow down, the host should run with INPUT default DROP, a
// trailing -j DROP in INPUT, or rely on a stateful zone like firewalld
// / ufw that already implements that semantic. This is documented in
// README.md / README.en.md under "Deployment requirements for drop
// existing connections on expiry".
func FlushConntrack(rule *model.Rule) (int, error) {
	bin, err := conntrackBinary()
	if err != nil {
		return 0, err
	}
	ports := flushPortList(rule)
	if len(ports) == 0 {
		return 0, nil
	}
	src := canonicalSource(rule.SourceIP)
	family := conntrackFamily(rule.SourceIP)

	total := 0
	for _, proto := range expandProto(rule.Protocol) {
		for _, port := range ports {
			n, err := flushOnce(bin, family, src, proto, port)
			if err != nil {
				return total, err
			}
			total += n
		}
	}
	return total, nil
}

// conntrackBinary returns the path to a conntrack CLI when present.
// We check both the Alpine package name (`conntrack`) and the more
// usual debian/centos name; in practice they all install the same
// `conntrack` symlink so a single LookPath is enough, but we keep
// the indirection so future package renames are easy to absorb.
func conntrackBinary() (string, error) {
	for _, name := range []string{"conntrack"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", ErrConntrackUnavailable
}

// flushOnce executes one conntrack -D invocation. Standard exit codes:
//   - 0 : at least one entry deleted
//   - 1 : no entry matched (treat as success with count=0)
//
// We parse the trailing "deleted N flow entries" line, falling back to
// counting the per-line "tcp ... 22 ..." records when newer conntrack
// builds drop the summary footer.
func flushOnce(bin, family, src, proto string, port int) (int, error) {
	args := []string{"-D"}
	if family != "" {
		args = append(args, "-f", family)
	}
	args = append(args,
		"-p", proto,
		"--orig-port-dst", strconv.Itoa(port),
	)
	if src != "" {
		args = append(args, "--orig-src", srcAddrOnly(src))
	}
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		// `conntrack -D` exits 1 when the table contained no matching
		// entries; that is the common path for cold rules and must
		// not propagate as an error. Anything else (permission denied,
		// kernel module missing, syntax) we surface so the caller can
		// log + abort.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return 0, nil
		}
		return 0, fmt.Errorf("%s %s: %w: %s", bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return parseConntrackDeleteOutput(string(out)), nil
}

// parseConntrackDeleteOutput tries the explicit summary first
// ("conntrack vN.N (conntrack-tools): N flow entries have been
// deleted.") and falls back to counting per-flow lines.
func parseConntrackDeleteOutput(out string) int {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "flow entries have been deleted") {
			continue
		}
		fields := strings.Fields(line)
		for _, f := range fields {
			if n, err := strconv.Atoi(f); err == nil && n >= 0 {
				return n
			}
		}
	}
	// Fallback: each per-flow record begins with the protocol token
	// (e.g. "tcp", "udp"). We only enter this branch when the summary
	// footer was missing, which keeps the legacy path conservative.
	count := 0
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "tcp ") ||
			strings.HasPrefix(l, "udp ") ||
			strings.HasPrefix(l, "icmp ") {
			count++
		}
	}
	return count
}

// flushPortList expands the rule's port group into a flat list of
// concrete dport integers. Conntrack's CLI does not support multiport
// ranges; we issue one delete per port (small overhead even for a 100
// port range, since we only run on rule expiry / revoke).
func flushPortList(r *model.Rule) []int {
	ps := rulePorts(r)
	if ps.Empty() && r.Port > 0 {
		return []int{r.Port}
	}
	if ps.Empty() {
		return nil
	}
	out := make([]int, 0)
	for _, rng := range ps.Ranges {
		for p := rng.From; p <= rng.To; p++ {
			out = append(out, p)
		}
	}
	return out
}

// conntrackFamily returns the conntrack CLI -f value for the rule's
// source-address family. Empty when no source CIDR is set or it is a
// wildcard, in which case we leave -f off and let conntrack default.
func conntrackFamily(src string) string {
	if canonicalSource(src) == "" {
		return ""
	}
	if strings.Contains(src, ":") {
		return "ipv6"
	}
	return "ipv4"
}

// srcAddrOnly trims the /N suffix when the rule stored a host CIDR
// like "1.2.3.4/32". `--orig-src` accepts a bare IP or a CIDR; the
// bare form keeps the match strictly per-host without conntrack
// having to canonicalise.
func srcAddrOnly(src string) string {
	if i := strings.Index(src, "/"); i > 0 {
		return src[:i]
	}
	return src
}

// IsWildcardSource reports whether a rule's source IP matches every
// host on its address family. Used by the API layer to surface a
// stronger UI warning when the operator opts into cleanup on a rule
// that effectively spans the entire internet.
func IsWildcardSource(src string) bool {
	s := strings.TrimSpace(src)
	return s == "" || s == "0.0.0.0/0" || s == "::/0" || s == "any" || s == "all"
}
