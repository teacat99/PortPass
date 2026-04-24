// Package portset parses, validates and serialises the port-group format
// used across PortPass rules. The canonical string representation is a
// comma-separated list of items where each item is either a single port
// or a "from-to" range, e.g. "22,80,443,8080-8090".
//
// PortSet keeps entries sorted and merged (overlapping / adjacent ranges
// are coalesced) so equality checks and driver-level serialisation are
// deterministic.
package portset

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Limits chosen to stay well below what Linux firewall backends allow:
// iptables multiport supports 15 port entries (a single port or a single
// range each count as one entry). Staying at 15 keeps one ruleset = one
// iptables line, which simplifies insert/delete semantics.
const (
	MaxPort    = 65535
	MinPort    = 1
	MaxEntries = 15
)

// Range is a closed interval [From, To] inclusive. A single port is
// represented as From == To.
type Range struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Count returns the number of ports covered by the range.
func (r Range) Count() int { return r.To - r.From + 1 }

// Contains returns true when port lies inside the range.
func (r Range) Contains(port int) bool { return port >= r.From && port <= r.To }

// Overlaps returns true when r and other share at least one port.
func (r Range) Overlaps(other Range) bool {
	return r.From <= other.To && other.From <= r.To
}

// Set is a canonicalised (sorted + merged) collection of Range values.
type Set struct {
	Ranges []Range
}

// Parse accepts a human-entered port-group string and returns a
// canonicalised Set. The empty string returns an empty set with no error
// so callers can distinguish "not provided" from "invalid input".
func Parse(s string) (Set, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return Set{}, nil
	}
	var ranges []Range
	for _, part := range strings.Split(trimmed, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		r, err := parseOne(part)
		if err != nil {
			return Set{}, err
		}
		ranges = append(ranges, r)
	}
	if len(ranges) == 0 {
		return Set{}, errors.New("empty port list")
	}
	return canonicalise(ranges)
}

// MustParse is a Parse that panics on error. Intended for constants in
// tests and seed data only.
func MustParse(s string) Set {
	ps, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return ps
}

// FromPort constructs a Set containing a single port. Convenience used
// during the "old single-port" → "port group" migration.
func FromPort(port int) Set {
	if port < MinPort || port > MaxPort {
		return Set{}
	}
	return Set{Ranges: []Range{{From: port, To: port}}}
}

func parseOne(part string) (Range, error) {
	if strings.Contains(part, "-") {
		segs := strings.SplitN(part, "-", 2)
		from, err := parsePort(segs[0])
		if err != nil {
			return Range{}, err
		}
		to, err := parsePort(segs[1])
		if err != nil {
			return Range{}, err
		}
		if from > to {
			return Range{}, fmt.Errorf("invalid range %q (from > to)", part)
		}
		return Range{From: from, To: to}, nil
	}
	p, err := parsePort(part)
	if err != nil {
		return Range{}, err
	}
	return Range{From: p, To: p}, nil
}

func parsePort(s string) (int, error) {
	s = strings.TrimSpace(s)
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	if n < MinPort || n > MaxPort {
		return 0, fmt.Errorf("port %d out of range [%d,%d]", n, MinPort, MaxPort)
	}
	return n, nil
}

// canonicalise sorts and merges overlapping/adjacent ranges, returning a
// deterministic Set. Returns an error when the merged set exceeds
// MaxEntries which is driver-dependent but kept global to keep the UX
// consistent across drivers.
func canonicalise(in []Range) (Set, error) {
	sort.Slice(in, func(i, j int) bool {
		if in[i].From != in[j].From {
			return in[i].From < in[j].From
		}
		return in[i].To < in[j].To
	})
	merged := make([]Range, 0, len(in))
	for _, r := range in {
		if len(merged) == 0 {
			merged = append(merged, r)
			continue
		}
		last := &merged[len(merged)-1]
		if r.From <= last.To+1 { // overlap or adjacent
			if r.To > last.To {
				last.To = r.To
			}
		} else {
			merged = append(merged, r)
		}
	}
	if len(merged) > MaxEntries {
		return Set{}, fmt.Errorf("too many port entries (%d), limit is %d", len(merged), MaxEntries)
	}
	return Set{Ranges: merged}, nil
}

// String returns the canonical "80,443,8080-8090" representation.
// An empty Set returns "".
func (p Set) String() string {
	if len(p.Ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p.Ranges))
	for _, r := range p.Ranges {
		if r.From == r.To {
			parts = append(parts, strconv.Itoa(r.From))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r.From, r.To))
		}
	}
	return strings.Join(parts, ",")
}

// Count returns the total number of ports covered by the set.
func (p Set) Count() int {
	total := 0
	for _, r := range p.Ranges {
		total += r.Count()
	}
	return total
}

// EntryCount returns the number of ranges (not ports); relevant for
// iptables --match multiport limits.
func (p Set) EntryCount() int { return len(p.Ranges) }

// Empty reports whether the set covers zero ports.
func (p Set) Empty() bool { return len(p.Ranges) == 0 }

// First returns the lowest port in the set (or 0 when empty). Used for
// the legacy single-Port column on Rule.
func (p Set) First() int {
	if len(p.Ranges) == 0 {
		return 0
	}
	return p.Ranges[0].From
}

// Contains reports whether port is covered by any range in the set.
func (p Set) Contains(port int) bool {
	for _, r := range p.Ranges {
		if r.Contains(port) {
			return true
		}
	}
	return false
}

// Overlaps reports whether p and other share at least one port.
func (p Set) Overlaps(other Set) bool {
	for _, a := range p.Ranges {
		for _, b := range other.Ranges {
			if a.Overlaps(b) {
				return true
			}
		}
	}
	return false
}

// ContainsSet reports whether every port in other is also in p (i.e. p
// is a superset of other). Used by the policy engine to check that a
// user's requested ports fall within their allowed ranges.
func (p Set) ContainsSet(other Set) bool {
	for _, r := range other.Ranges {
		if !p.containsRange(r) {
			return false
		}
	}
	return true
}

func (p Set) containsRange(r Range) bool {
	for _, x := range p.Ranges {
		if x.From <= r.From && r.To <= x.To {
			return true
		}
	}
	return false
}

// IPTablesFormat serialises the set as iptables --dports expects:
// comma-separated entries where ranges use ":" instead of "-".
// iptables multiport takes up to 15 entries.
func (p Set) IPTablesFormat() string {
	if len(p.Ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p.Ranges))
	for _, r := range p.Ranges {
		if r.From == r.To {
			parts = append(parts, strconv.Itoa(r.From))
		} else {
			parts = append(parts, fmt.Sprintf("%d:%d", r.From, r.To))
		}
	}
	return strings.Join(parts, ",")
}

// NFTablesFormat serialises the set as an nft set expression content,
// e.g. "80, 443, 8080-8090" (usable inside "{ ... }").
func (p Set) NFTablesFormat() string {
	if len(p.Ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p.Ranges))
	for _, r := range p.Ranges {
		if r.From == r.To {
			parts = append(parts, strconv.Itoa(r.From))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r.From, r.To))
		}
	}
	return strings.Join(parts, ", ")
}

// Flatten returns every individual port in the set (expanding ranges).
// Useful for drivers like UFW/firewalld that need one API call per port.
func (p Set) Flatten() []int {
	out := make([]int, 0, p.Count())
	for _, r := range p.Ranges {
		for port := r.From; port <= r.To; port++ {
			out = append(out, port)
		}
	}
	return out
}
