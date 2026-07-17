package idn

import "testing"

func TestToASCII(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"pure ascii unchanged", "example.com", "example.com"},
		{"uppercased lowered", "Example.COM", "example.com"},
		{"japanese label", "日本語.jp", "xn--wgv71a119e.jp"},
		{"mixed labels", "例え.example.com", "xn--r8jz45g.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToASCII(tt.in)
			if err != nil {
				t.Fatalf("ToASCII(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ToASCII(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestToASCIIRejectsMixedPunycode(t *testing.T) {
	if _, err := ToASCII("xn--あ.jp"); err == nil {
		t.Fatal("expected error for xn-- label with non-ASCII, got nil")
	}
}
