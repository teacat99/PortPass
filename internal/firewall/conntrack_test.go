package firewall

import (
	"errors"
	"testing"

	"github.com/teacat99/PortPass/internal/model"
)

func TestParseConntrackDeleteOutput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{
			name: "summary footer",
			in: `tcp      6 src=1.2.3.4 dst=5.6.7.8 sport=42000 dport=22 ...
tcp      6 src=1.2.3.4 dst=5.6.7.8 sport=42001 dport=22 ...
conntrack v1.4.7 (conntrack-tools): 2 flow entries have been deleted.`,
			want: 2,
		},
		{
			name: "no footer falls back to per-flow count",
			in: `tcp      6 src=1.2.3.4 dst=5.6.7.8 sport=42000 dport=22 ...
udp      17 src=1.2.3.4 dst=5.6.7.8 sport=42001 dport=53 ...`,
			want: 2,
		},
		{
			name: "empty output",
			in:   "",
			want: 0,
		},
		{
			name: "only header lines",
			in:   "conntrack v1.4.7 (conntrack-tools): 0 flow entries have been deleted.",
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseConntrackDeleteOutput(tt.in); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFlushPortListLegacyAndGroup(t *testing.T) {
	r := &model.Rule{Ports: "22,80,8080-8082", Protocol: model.ProtoTCP}
	got := flushPortList(r)
	want := []int{22, 80, 8080, 8081, 8082}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: %v vs %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("port[%d]=%d want %d", i, got[i], want[i])
		}
	}

	legacy := &model.Rule{Port: 22, Protocol: model.ProtoTCP}
	if got := flushPortList(legacy); len(got) != 1 || got[0] != 22 {
		t.Fatalf("legacy fallback: %v", got)
	}

	empty := &model.Rule{}
	if got := flushPortList(empty); got != nil {
		t.Fatalf("empty rule should yield nil, got %v", got)
	}
}

func TestConntrackFamily(t *testing.T) {
	cases := map[string]string{
		"":             "",
		"0.0.0.0/0":    "",
		"::/0":         "",
		"1.2.3.4/32":   "ipv4",
		"1.2.3.4":      "ipv4",
		"2001:db8::/32": "ipv6",
	}
	for in, want := range cases {
		if got := conntrackFamily(in); got != want {
			t.Errorf("conntrackFamily(%q)=%q want %q", in, got, want)
		}
	}
}

// TestSrcAddrOnly only documents the CIDR -> bare-host helper. We
// never call it on wildcard sources because canonicalSource() filters
// those out earlier in flushOnce, so no test case for "0.0.0.0/0".
func TestSrcAddrOnly(t *testing.T) {
	cases := map[string]string{
		"":              "",
		"1.2.3.4/32":    "1.2.3.4",
		"1.2.3.4":       "1.2.3.4",
		"2001:db8::/32": "2001:db8::",
		"2001:db8::":    "2001:db8::",
	}
	for in, want := range cases {
		if got := srcAddrOnly(in); got != want {
			t.Errorf("srcAddrOnly(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsWildcardSource(t *testing.T) {
	cases := map[string]bool{
		"":          true,
		"0.0.0.0/0": true,
		"::/0":      true,
		"any":       true,
		"all":       true,
		"1.2.3.4":   false,
		"10.0.0.0/8": false,
	}
	for in, want := range cases {
		if got := IsWildcardSource(in); got != want {
			t.Errorf("IsWildcardSource(%q)=%v want %v", in, got, want)
		}
	}
}

// TestFlushConntrackUnavailableEnvelope confirms that callers can
// distinguish the "binary missing" condition from a real error - the
// lifecycle layer relies on this to downgrade to a debug log instead
// of failing the rule expiry. We force the unavailable condition by
// passing an empty PATH so LookPath cannot resolve `conntrack`.
func TestFlushConntrackUnavailableEnvelope(t *testing.T) {
	t.Setenv("PATH", "")
	r := &model.Rule{SourceIP: "1.2.3.4/32", Port: 22, Protocol: model.ProtoTCP}
	n, err := FlushConntrack(r)
	if !errors.Is(err, ErrConntrackUnavailable) {
		t.Fatalf("expected ErrConntrackUnavailable, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected count=0 when binary is absent, got %d", n)
	}
}
