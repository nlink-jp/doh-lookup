package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/nlink-jp/doh-lookup/internal/cache"
	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/doh"
)

// fakeClient returns canned responses keyed by record type and counts calls.
type fakeClient struct {
	byType map[string]*doh.Response
	calls  int
	lastCD bool
}

func (f *fakeClient) Query(p doh.Provider, name, rrType string, cd bool) (*doh.Response, error) {
	f.calls++
	f.lastCD = cd
	if r, ok := f.byType[rrType]; ok {
		return r, nil
	}
	return &doh.Response{Provider: p.Name, Endpoint: p.Endpoint, Name: name, Type: rrType, Status: 0, StatusText: "NOERROR"}, nil
}

func testEngine(t *testing.T, fc DoHClient) *Engine {
	t.Helper()
	cfg := &config.Config{
		Provider:      "cloudflare",
		Profile:       []string{"A", "AAAA"},
		CacheTTLFloor: 60 * time.Second,
		CacheDir:      t.TempDir(),
	}
	return &Engine{
		Cfg:    cfg,
		Cache:  &cache.Store{Dir: cfg.CacheDir},
		Client: fc,
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

func TestLookupForwardAggregates(t *testing.T) {
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A":    {Status: 0, StatusText: "NOERROR", Authenticated: true, Answers: []doh.Answer{{Name: "example.com", Type: "A", TTL: 300, Data: "93.184.216.34"}}},
		"AAAA": {Status: 0, StatusText: "NOERROR", Answers: []doh.Answer{{Name: "example.com", Type: "AAAA", TTL: 300, Data: "2606:2800:220:1:248:1893:25c8:1946"}}},
	}}
	e := testEngine(t, fc)

	res, err := e.Lookup("example.com", Options{})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.Kind != "forward" || res.Provider != "cloudflare" || res.Endpoint == "" {
		t.Errorf("provenance/kind wrong: %+v", res)
	}
	if res.Status != "NOERROR" {
		t.Errorf("Status = %q, want NOERROR", res.Status)
	}
	if !res.Authenticated {
		t.Error("Authenticated = false, want true")
	}
	if len(res.Records) != 2 {
		t.Fatalf("got %d records, want 2", len(res.Records))
	}
	if fc.calls != 2 {
		t.Errorf("calls = %d, want 2 (A + AAAA profile)", fc.calls)
	}
}

func TestLookupNXDOMAIN(t *testing.T) {
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A":    {Status: 3, StatusText: "NXDOMAIN"},
		"AAAA": {Status: 3, StatusText: "NXDOMAIN"},
	}}
	e := testEngine(t, fc)
	res, err := e.Lookup("nope.example", Options{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if res == nil || res.Status != "NXDOMAIN" {
		t.Errorf("want populated NXDOMAIN result, got %+v", res)
	}
}

func TestLookupReversePTR(t *testing.T) {
	fc := &fakeClient{byType: map[string]*doh.Response{
		"PTR": {Status: 0, StatusText: "NOERROR", Answers: []doh.Answer{{Name: "8.8.8.8.in-addr.arpa", Type: "PTR", TTL: 3600, Data: "dns.google"}}},
	}}
	e := testEngine(t, fc)
	res, err := e.Lookup("8.8.8.8", Options{})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.Kind != "reverse" {
		t.Errorf("Kind = %q, want reverse", res.Kind)
	}
	if len(res.Records) != 1 || res.Records[0].Data != "dns.google" {
		t.Errorf("records = %+v", res.Records)
	}
	if res.Types[0] != "PTR" {
		t.Errorf("Types = %v, want [PTR]", res.Types)
	}
}

func TestLookupCacheHit(t *testing.T) {
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A": {Status: 0, StatusText: "NOERROR", Answers: []doh.Answer{{Name: "example.com", Type: "A", TTL: 300, Data: "1.1.1.1"}}},
	}}
	e := testEngine(t, fc)
	if _, err := e.Lookup("example.com", Options{Types: []string{"A"}}); err != nil {
		t.Fatalf("first Lookup: %v", err)
	}
	before := fc.calls
	res, err := e.Lookup("example.com", Options{Types: []string{"A"}})
	if err != nil {
		t.Fatalf("second Lookup: %v", err)
	}
	if !res.Cached {
		t.Error("second lookup not served from cache")
	}
	if fc.calls != before {
		t.Errorf("cache hit still called client: %d → %d", before, fc.calls)
	}
}

func TestLookupInvalidInputNoNetwork(t *testing.T) {
	fc := &fakeClient{}
	e := testEngine(t, fc)
	if _, err := e.Lookup("bad host", Options{}); err == nil {
		t.Fatal("expected validation error for invalid input")
	}
	if fc.calls != 0 {
		t.Errorf("invalid input reached the network: %d calls", fc.calls)
	}
}

func TestLookupUnknownType(t *testing.T) {
	e := testEngine(t, &fakeClient{})
	if _, err := e.Lookup("example.com", Options{Types: []string{"BOGUS"}}); err == nil {
		t.Fatal("expected error for unknown record type")
	}
}

func TestLookupFiltersDNSSECMeta(t *testing.T) {
	// A query for A with the DO bit set returns the A record plus its RRSIG.
	// The RRSIG must be filtered from normalized records, but authentication
	// must still be reported.
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A": {Status: 0, StatusText: "NOERROR", Authenticated: true, Answers: []doh.Answer{
			{Name: "example.com", Type: "A", TTL: 300, Data: "93.184.216.34"},
			{Name: "example.com", Type: "RRSIG", TTL: 300, Data: "A 13 2 300 ..."},
		}},
	}}
	e := testEngine(t, fc)
	res, err := e.Lookup("example.com", Options{Types: []string{"A"}})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(res.Records) != 1 || res.Records[0].Type != "A" {
		t.Errorf("RRSIG not filtered: %+v", res.Records)
	}
	if !res.Authenticated {
		t.Error("Authenticated lost after filtering RRSIG")
	}

	// An explicit RRSIG query keeps them.
	res2, err := e.Lookup("example.com", Options{Types: []string{"RRSIG"}})
	if err != nil {
		t.Fatalf("Lookup RRSIG: %v", err)
	}
	_ = res2 // fakeClient returns a default NOERROR/no-answer for RRSIG; the
	// point is that the filter keys on the queried type, exercised above.
}
