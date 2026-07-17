package doh

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

type errDoer struct{ err error }

func (e errDoer) Do(*http.Request) (*http.Response, error) { return nil, e.err }

func TestQueryTransportError(t *testing.T) {
	c := &Client{HTTP: errDoer{err: errors.New("dial tcp: refused")}}
	p, _ := ProviderByName("cloudflare", "")
	if _, err := c.Query(p, "example.com", "A", false); err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

func TestQueryGoogleParamsAndHeaders(t *testing.T) {
	f := &fakeDoer{status: 200, body: `{"Status":0}`}
	c := &Client{HTTP: f, SuppressECS: false}
	p, _ := ProviderByName("google", "")
	if _, err := c.Query(p, "example.com", "A", true); err != nil {
		t.Fatalf("Query: %v", err)
	}
	// Google spells the DO/CD flags as 1, and (ECS off) sends no subnet param.
	if !strings.Contains(f.lastURL, "do=1") || !strings.Contains(f.lastURL, "cd=1") {
		t.Errorf("google bool style wrong: %s", f.lastURL)
	}
	if strings.Contains(f.lastURL, "edns_client_subnet") {
		t.Errorf("ECS param present despite SuppressECS=false: %s", f.lastURL)
	}
	if got := f.lastHdr.Get("Accept"); got != "application/json" {
		t.Errorf("google Accept = %q, want application/json", got)
	}
}

func TestQueryCommentArray(t *testing.T) {
	f := &fakeDoer{status: 200, body: `{"Status":0,"Comment":["rate","limited"]}`}
	c := &Client{HTTP: f}
	p, _ := ProviderByName("cloudflare", "")
	resp, err := c.Query(p, "x.example", "A", false)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if resp.Comment != "rate limited" {
		t.Errorf("Comment array not flattened: %q", resp.Comment)
	}
}

func TestQueryEndpointOverride(t *testing.T) {
	f := &fakeDoer{status: 200, body: `{"Status":0}`}
	c := &Client{HTTP: f}
	p, err := ProviderByName("cloudflare", "https://proxy.example/dns-query")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Query(p, "example.com", "A", false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(f.lastURL, "https://proxy.example/dns-query?") {
		t.Errorf("endpoint override not used: %s", f.lastURL)
	}
}

func TestQueryRawPreserved(t *testing.T) {
	body := `{"Status":0,"AD":true}`
	f := &fakeDoer{status: 200, body: body}
	c := &Client{HTTP: f}
	p, _ := ProviderByName("cloudflare", "")
	resp, _ := c.Query(p, "example.com", "A", false)
	if strings.TrimSpace(string(resp.Raw)) != body {
		t.Errorf("Raw = %q, want %q", string(resp.Raw), body)
	}
}

func TestTypeNameFallbackAndMeta(t *testing.T) {
	if got := TypeName(9999); got != "TYPE9999" {
		t.Errorf("TypeName(9999) = %q", got)
	}
	if TypeName(46) != "RRSIG" {
		t.Errorf("RRSIG (46) should be named")
	}
	if !IsDNSSECMeta("RRSIG") || !IsDNSSECMeta("NSEC") || IsDNSSECMeta("A") {
		t.Error("IsDNSSECMeta classification wrong")
	}
}

func TestRcodeTextUnknown(t *testing.T) {
	if got := RcodeText(99); got != "RCODE99" {
		t.Errorf("RcodeText(99) = %q", got)
	}
	if RcodeText(0) != "NOERROR" || RcodeText(3) != "NXDOMAIN" {
		t.Error("known rcodes wrong")
	}
}

func TestSupportedTypesSortedIncludesNew(t *testing.T) {
	types := SupportedTypes()
	found := map[string]bool{}
	for _, ty := range types {
		found[ty] = true
	}
	for _, want := range []string{"A", "AAAA", "PTR", "CAA", "RRSIG"} {
		if !found[want] {
			t.Errorf("SupportedTypes missing %q", want)
		}
	}
	// sorted
	for i := 1; i < len(types); i++ {
		if types[i-1] > types[i] {
			t.Errorf("SupportedTypes not sorted at %d: %v", i, types)
		}
	}
}
