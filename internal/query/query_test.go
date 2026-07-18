package query

import (
	"errors"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantKind  Kind
		wantValue string
	}{
		{"ipv4", "1.2.3.4", KindIP, "1.2.3.4"},
		{"ipv4-mapped ipv6 unmapped", "::ffff:1.2.3.4", KindIP, "1.2.3.4"},
		{"ipv6", "2606:4700:4700::1111", KindIP, "2606:4700:4700::1111"},
		{"domain", "Example.COM", KindDomain, "example.com"},
		{"domain trailing dot", "example.com.", KindDomain, "example.com"},
		{"idn domain", "日本語.jp", KindDomain, "xn--wgv71a119e.jp"},
		{"underscore service label", "_dmarc.example.com", KindDomain, "_dmarc.example.com"},
		{"dkim selector", "Selector1._domainkey.example.com", KindDomain, "selector1._domainkey.example.com"},
		{"srv service._proto labels", "_sip._tcp.example.com", KindDomain, "_sip._tcp.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Classify(tt.in)
			if err != nil {
				t.Fatalf("Classify(%q) error: %v", tt.in, err)
			}
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if got.Value != tt.wantValue {
				t.Errorf("Value = %q, want %q", got.Value, tt.wantValue)
			}
		})
	}
}

func TestClassifyRejects(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"exa mple.com",           // embedded whitespace
		"example.com\r\nHost: x", // CRLF injection attempt
		"single",                 // single label
		"-bad.example.com",       // leading hyphen
		"12345",                  // all-numeric, not an IP
	}
	for _, in := range bad {
		if _, err := Classify(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("Classify(%q) = %v, want ErrInvalid", in, err)
		}
	}
}

func TestReverseName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1.2.3.4", "4.3.2.1.in-addr.arpa"},
		{"8.8.8.8", "8.8.8.8.in-addr.arpa"},
		{"2001:db8::1", "1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.b.d.0.1.0.0.2.ip6.arpa"},
	}
	for _, tt := range tests {
		got, err := Classify(tt.in)
		if err != nil {
			t.Fatalf("Classify(%q): %v", tt.in, err)
		}
		if rn := got.ReverseName(); rn != tt.want {
			t.Errorf("ReverseName(%q) = %q, want %q", tt.in, rn, tt.want)
		}
	}
}

func TestReverseNameEmptyForDomain(t *testing.T) {
	tgt, _ := Classify("example.com")
	if rn := tgt.ReverseName(); rn != "" {
		t.Errorf("ReverseName for domain = %q, want empty", rn)
	}
}
