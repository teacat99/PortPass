package portset

import "testing"

func TestParseBasic(t *testing.T) {
	cases := []struct {
		in       string
		want     string
		wantCnt  int
		wantErr  bool
	}{
		{"", "", 0, false},
		{"80", "80", 1, false},
		{"80,443", "80,443", 2, false},
		{"80, 443 ,8080", "80,443,8080", 3, false},
		{"1-10", "1-10", 10, false},
		{"1-10,5-15", "1-15", 15, false},
		{"1-5,10-12", "1-5,10-12", 8, false},
		{"5,1,3", "1,3,5", 3, false},
		{"1-3,4-5", "1-5", 5, false},
		{"1,2,3", "1-3", 3, false},
		{"0", "", 0, true},
		{"99999", "", 0, true},
		{"abc", "", 0, true},
		{"20-10", "", 0, true},
	}
	for _, tc := range cases {
		got, err := Parse(tc.in)
		if (err != nil) != tc.wantErr {
			t.Fatalf("Parse(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
		if err != nil {
			continue
		}
		if got.String() != tc.want {
			t.Errorf("Parse(%q).String() = %q want %q", tc.in, got.String(), tc.want)
		}
		if got.Count() != tc.wantCnt {
			t.Errorf("Parse(%q).Count() = %d want %d", tc.in, got.Count(), tc.wantCnt)
		}
	}
}

func TestContainsSet(t *testing.T) {
	outer := MustParse("1-100,200,300-400")
	if !outer.ContainsSet(MustParse("50-60")) {
		t.Fatal("expected 50-60 inside 1-100")
	}
	if !outer.ContainsSet(MustParse("1,50,100,200,350")) {
		t.Fatal("expected mixed subset")
	}
	if outer.ContainsSet(MustParse("50-150")) {
		t.Fatal("expected 50-150 not contained")
	}
	if outer.ContainsSet(MustParse("500")) {
		t.Fatal("expected 500 not contained")
	}
}

func TestOverlaps(t *testing.T) {
	a := MustParse("1-10,20-30")
	if !a.Overlaps(MustParse("5-25")) {
		t.Fatal("expected overlap")
	}
	if a.Overlaps(MustParse("11-19")) {
		t.Fatal("expected no overlap")
	}
}

func TestIPTablesFormat(t *testing.T) {
	p := MustParse("80,443,8080-8090")
	if got := p.IPTablesFormat(); got != "80,443,8080:8090" {
		t.Errorf("got %q", got)
	}
}

func TestNFTablesFormat(t *testing.T) {
	p := MustParse("80,443,8080-8090")
	if got := p.NFTablesFormat(); got != "80, 443, 8080-8090" {
		t.Errorf("got %q", got)
	}
}

func TestMaxEntries(t *testing.T) {
	// Build 16 disjoint single ports → should fail to canonicalise.
	parts := make([]byte, 0)
	for i := 1; i <= 16; i++ {
		if i > 1 {
			parts = append(parts, ',')
		}
		parts = append(parts, []byte{'0' + byte(i/10), '0' + byte(i%10)}...)
	}
	// That produces "01,02,..." not a valid port number string but
	// leading zeroes parse to integers fine in Atoi. Use fixed input instead.
	input := "1,3,5,7,9,11,13,15,17,19,21,23,25,27,29,31"
	if _, err := Parse(input); err == nil {
		t.Fatal("expected error for >15 entries")
	}
}

func TestFromPort(t *testing.T) {
	p := FromPort(22)
	if p.String() != "22" || p.Count() != 1 {
		t.Fatalf("FromPort(22) = %+v", p)
	}
}

func TestFlatten(t *testing.T) {
	p := MustParse("22,80-82")
	want := []int{22, 80, 81, 82}
	got := p.Flatten()
	if len(got) != len(want) {
		t.Fatalf("len mismatch")
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("at %d got %d want %d", i, got[i], want[i])
		}
	}
}
