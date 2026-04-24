package firewall

import "testing"

func TestParseNftList(t *testing.T) {
	sample := `table inet portpass {
	chain input {
		type filter hook input priority filter; policy accept;
		ip saddr 1.2.3.4/32 tcp dport 22 accept comment "portpass:10" # handle 5
		ip6 saddr 2001:db8::/32 udp dport 53 accept comment "portpass:11" # handle 6
	}
}`
	got := parseNftList(sample)
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
	if got[0].RuleID != 10 || got[0].Port != 22 || got[0].Protocol != "tcp" {
		t.Errorf("first rule mismatch: %+v", got[0])
	}
	if got[1].RuleID != 11 || got[1].Protocol != "udp" {
		t.Errorf("second rule mismatch: %+v", got[1])
	}
}

func TestParseUFWStatus(t *testing.T) {
	sample := `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    1.2.3.4                    # portpass:42
[ 2] 80/tcp                     ALLOW IN    Anywhere                   # added manually
[ 3] 5000/udp                   ALLOW IN    10.0.0.0/8                 # portpass:7`
	got := parseUFWStatus(sample)
	if len(got) != 2 {
		t.Fatalf("expected 2 portpass rules, got %d", len(got))
	}
	if got[0].RuleID != 42 || got[0].Port != 22 || got[0].Protocol != "tcp" {
		t.Errorf("first rule mismatch: %+v", got[0])
	}
	if got[1].RuleID != 7 || got[1].Port != 5000 || got[1].Protocol != "udp" {
		t.Errorf("second rule mismatch: %+v", got[1])
	}
}

func TestParseFirewalldRich(t *testing.T) {
	sample := `rule family="ipv4" source address="1.2.3.4/32" port port="22" protocol="tcp" log prefix="portpass:99" level="info" limit value="1/m" accept
rule family="ipv6" port port="53" protocol="udp" log prefix="portpass:100" level="info" limit value="1/m" accept`
	got := parseFirewalldRich(sample)
	if len(got) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(got))
	}
	if got[0].RuleID != 99 || got[0].Port != 22 || got[0].SourceIP != "1.2.3.4/32" {
		t.Errorf("first rule mismatch: %+v", got[0])
	}
	if got[1].RuleID != 100 || got[1].Port != 53 || got[1].Protocol != "udp" {
		t.Errorf("second rule mismatch: %+v", got[1])
	}
}

func TestBuildRichRule(t *testing.T) {
	rr := buildRichRule("1.2.3.4/32", "22", "tcp", "portpass:1")
	if !containsAll(rr, `family="ipv4"`, `source address="1.2.3.4/32"`, `port port="22"`, `protocol="tcp"`, `portpass:1`) {
		t.Fatalf("rich rule missing parts: %s", rr)
	}
	rr6 := buildRichRule("2001:db8::/32", "80", "tcp", "portpass:2")
	if !containsAll(rr6, `family="ipv6"`, `source address="2001:db8::/32"`) {
		t.Fatalf("ipv6 rule incorrect: %s", rr6)
	}
	rrAny := buildRichRule("", "443", "tcp", "portpass:3")
	if containsAll(rrAny, `source address`) {
		t.Fatalf("empty source should omit clause, got %s", rrAny)
	}
	rrRange := buildRichRule("10.0.0.0/8", "8080-8090", "tcp", "portpass:4")
	if !containsAll(rrRange, `port port="8080-8090"`) {
		t.Fatalf("range rich rule missing range: %s", rrRange)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
