package query

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/nlink-jp/doh-lookup/internal/idn"
)

// Kind is the classified kind of a lookup target.
type Kind string

const (
	// KindDomain is a domain name → forward lookup.
	KindDomain Kind = "domain"
	// KindIP is an IP address → reverse (PTR) lookup.
	KindIP Kind = "ip"
)

// ErrInvalid marks a target that is valid as neither a domain nor an IP.
// Callers must not send such input anywhere near the network.
var ErrInvalid = errors.New("invalid input")

// Target is a validated, canonicalized lookup target.
type Target struct {
	Kind     Kind
	Original string     // input as given (trimmed)
	Value    string     // canonical: unmapped IP, or lowercase A-label domain
	Addr     netip.Addr // set when Kind == KindIP
}

// Classify validates a target and returns its canonical form. Detection is
// automatic and unambiguous: an IP address is a reverse lookup, anything else
// must pass domain validation (a bare IP literal can never be a valid domain —
// its final label is all-numeric). Input that is neither is rejected with
// ErrInvalid before any network I/O.
func Classify(input string) (Target, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return Target{}, fmt.Errorf("%w: empty input", ErrInvalid)
	}
	// The gate against request-splitting into the DoH HTTPS query and cache
	// key: no control character or embedded whitespace may survive into any
	// later stage, regardless of the eventual classification.
	for _, r := range in {
		if r < 0x21 || r == 0x7f {
			return Target{}, fmt.Errorf("%w: control or whitespace character in input", ErrInvalid)
		}
	}

	if t, err := classifyIP(in); err == nil {
		return t, nil
	}
	t, err := classifyDomain(in)
	if err != nil {
		reason := strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")
		return Target{}, fmt.Errorf("%w: %q is not an IP address or a valid domain name (%s)", ErrInvalid, in, reason)
	}
	return t, nil
}

func classifyIP(in string) (Target, error) {
	addr, err := netip.ParseAddr(in)
	if err != nil {
		return Target{}, fmt.Errorf("%w: not an IP address", ErrInvalid)
	}
	if addr.Zone() != "" {
		return Target{}, fmt.Errorf("%w: IPv6 zone is not allowed", ErrInvalid)
	}
	addr = addr.Unmap()
	return Target{Kind: KindIP, Original: in, Value: addr.String(), Addr: addr}, nil
}

// classifyDomain validates DNS-name syntax (total ≤253 after the optional
// trailing dot, labels 1–63, DNS-label charset — LDH plus underscore — last
// label not all-numeric). Underscore is permitted because underscore-prefixed
// labels (_dmarc, _domainkey, and the _service._proto labels of SRV/TLSA) are
// legitimate query targets even though they are not valid hostnames. IDN
// U-labels are converted to punycode A-labels first, so the charset check
// below always sees the wire form.
func classifyDomain(in string) (Target, error) {
	name := strings.ToLower(strings.TrimSuffix(in, "."))
	if name == "" {
		return Target{}, fmt.Errorf("%w: empty domain", ErrInvalid)
	}
	if !isASCII(name) {
		conv, err := idn.ToASCII(name)
		if err != nil {
			return Target{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		name = conv
	}
	if len(name) > 253 {
		return Target{}, fmt.Errorf("%w: domain exceeds 253 characters", ErrInvalid)
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return Target{}, fmt.Errorf("%w: domain needs at least two labels", ErrInvalid)
	}
	for _, l := range labels {
		if err := checkLabel(l); err != nil {
			return Target{}, err
		}
	}
	if allDigits(labels[len(labels)-1]) {
		return Target{}, fmt.Errorf("%w: top-level label cannot be all-numeric", ErrInvalid)
	}
	return Target{Kind: KindDomain, Original: in, Value: name}, nil
}

func checkLabel(l string) error {
	if l == "" {
		return fmt.Errorf("%w: empty label", ErrInvalid)
	}
	if len(l) > 63 {
		return fmt.Errorf("%w: label exceeds 63 characters", ErrInvalid)
	}
	if l[0] == '-' || l[len(l)-1] == '-' {
		return fmt.Errorf("%w: label cannot start or end with a hyphen", ErrInvalid)
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		// LDH plus underscore. Underscore is not a hostname character, but it
		// is a valid DNS label octet and is the canonical form of many record
		// targets we must be able to resolve: _dmarc / _domainkey (email
		// auth), _acme-challenge (ACME), and the _service._proto labels of
		// SRV/TLSA. Unlike '-', it may appear in any position (leading
		// included), so there is no positional restriction on it.
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return fmt.Errorf("%w: label contains %q", ErrInvalid, rune(c))
	}
	return nil
}

// ReverseName returns the PTR query name for an IP target (in-addr.arpa for
// IPv4, ip6.arpa nibble form for IPv6). It returns "" for non-IP targets.
func (t Target) ReverseName() string {
	if t.Kind != KindIP {
		return ""
	}
	if t.Addr.Is4() {
		b := t.Addr.As4()
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", b[3], b[2], b[1], b[0])
	}
	b := t.Addr.As16()
	var sb strings.Builder
	const hex = "0123456789abcdef"
	for i := len(b) - 1; i >= 0; i-- {
		sb.WriteByte(hex[b[i]&0x0f])
		sb.WriteByte('.')
		sb.WriteByte(hex[b[i]>>4])
		sb.WriteByte('.')
	}
	sb.WriteString("ip6.arpa")
	return sb.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
