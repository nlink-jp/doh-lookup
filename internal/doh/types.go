package doh

import (
	"fmt"
	"sort"
	"strings"
)

// typeByName maps an uppercase DNS record-type name to its numeric code.
// Scope is the record types doh-lookup surfaces; the JSON DoH API accepts the
// name form in the query and returns the numeric form in answers, so we need
// the mapping in both directions.
var typeByName = map[string]int{
	"A":      1,
	"NS":     2,
	"CNAME":  5,
	"SOA":    6,
	"PTR":    12,
	"MX":     15,
	"TXT":    16,
	"AAAA":   28,
	"SRV":    33,
	"NAPTR":  35,
	"DS":     43,
	"RRSIG":  46,
	"NSEC":   47,
	"DNSKEY": 48,
	"NSEC3":  50,
	"TLSA":   52,
	"CAA":    257,
}

// dnssecMeta are the record types the resolver adds only because we set the
// DO bit (needed for the AD flag). They are signature/proof records, not
// answer data, so they are filtered from normalized output unless the caller
// asked for them by name.
var dnssecMeta = map[string]bool{"RRSIG": true, "NSEC": true, "NSEC3": true}

// IsDNSSECMeta reports whether a record-type name is DNSSEC signature/proof
// metadata rather than answer data.
func IsDNSSECMeta(typeName string) bool {
	return dnssecMeta[typeName]
}

// nameByType is the reverse of typeByName, built once at init.
var nameByType = func() map[int]string {
	m := make(map[int]string, len(typeByName))
	for n, c := range typeByName {
		m[c] = n
	}
	return m
}()

// DefaultProfile is the record-type bundle fetched when no explicit --type is
// given: a one-shot "what does this domain look like" picture aimed at CTI/IR
// triage. Narrow it with --type.
var DefaultProfile = []string{"A", "AAAA", "MX", "TXT", "NS", "SOA", "CAA"}

// NormalizeType upper-cases and validates a record-type name, returning the
// canonical name and its numeric code. An unknown type is rejected before any
// network I/O.
func NormalizeType(t string) (string, int, error) {
	up := strings.ToUpper(strings.TrimSpace(t))
	code, ok := typeByName[up]
	if !ok {
		return "", 0, fmt.Errorf("unknown record type %q (supported: %s)", t, strings.Join(SupportedTypes(), ", "))
	}
	return up, code, nil
}

// TypeName maps a numeric record type back to its name, falling back to
// "TYPE<n>" for codes outside the supported set (so raw answers are never
// dropped).
func TypeName(code int) string {
	if n, ok := nameByType[code]; ok {
		return n
	}
	return fmt.Sprintf("TYPE%d", code)
}

// SupportedTypes returns the sorted list of record-type names doh-lookup
// understands (for help text and error messages).
func SupportedTypes() []string {
	out := make([]string, 0, len(typeByName))
	for n := range typeByName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// rcodeText maps a DNS RCODE to its mnemonic. Unknown codes render as
// "RCODE<n>".
var rcodeText = map[int]string{
	0: "NOERROR",
	1: "FORMERR",
	2: "SERVFAIL",
	3: "NXDOMAIN",
	4: "NOTIMP",
	5: "REFUSED",
}

// RcodeText returns the mnemonic for a DNS response code.
func RcodeText(code int) string {
	if s, ok := rcodeText[code]; ok {
		return s
	}
	return fmt.Sprintf("RCODE%d", code)
}
