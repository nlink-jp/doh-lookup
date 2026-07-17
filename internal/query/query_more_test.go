package query

import (
	"strings"
	"testing"
)

func TestClassifyRejectsIPv6Zone(t *testing.T) {
	if _, err := Classify("fe80::1%eth0"); err == nil {
		t.Fatal("IPv6 with zone should be rejected")
	}
}

func TestClassifyRejectsOversizeLabel(t *testing.T) {
	long := strings.Repeat("a", 64)
	if _, err := Classify(long + ".example.com"); err == nil {
		t.Fatal("label >63 chars should be rejected")
	}
}

func TestClassifyRejectsOversizeName(t *testing.T) {
	// 4 * (63 + 1) = 256 > 253.
	label := strings.Repeat("a", 63)
	name := strings.Join([]string{label, label, label, label}, ".")
	if _, err := Classify(name); err == nil {
		t.Fatal("name >253 chars should be rejected")
	}
}

func TestClassifyAcceptsHyphenInMiddle(t *testing.T) {
	tgt, err := Classify("my-host.example.com")
	if err != nil {
		t.Fatalf("valid hyphenated host rejected: %v", err)
	}
	if tgt.Kind != KindDomain {
		t.Errorf("Kind = %q", tgt.Kind)
	}
}

func TestReverseNameIPv6Full(t *testing.T) {
	tgt, err := Classify("2606:4700:4700::1111")
	if err != nil {
		t.Fatal(err)
	}
	rn := tgt.ReverseName()
	if !strings.HasSuffix(rn, ".ip6.arpa") {
		t.Errorf("ip6 reverse name malformed: %s", rn)
	}
	// 32 nibbles + "ip6.arpa" → 33 dot-separated fields, first nibble is the
	// low nibble of the last address byte (0x11 → '1').
	if !strings.HasPrefix(rn, "1.1.1.1.") {
		t.Errorf("ip6 reverse name prefix wrong: %s", rn)
	}
}
