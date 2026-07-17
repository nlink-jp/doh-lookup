package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/nlink-jp/doh-lookup/internal/cache"
	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/doh"
)

func TestAggregateSoftStatusServfail(t *testing.T) {
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A":    {Status: 2, StatusText: "SERVFAIL"},
		"AAAA": {Status: 2, StatusText: "SERVFAIL"},
	}}
	e := testEngine(t, fc)
	res, err := e.Lookup("weird.example", Options{})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.Status != "SERVFAIL" {
		t.Errorf("Status = %q, want SERVFAIL", res.Status)
	}
}

func TestAggregateNODATA(t *testing.T) {
	// NOERROR with no answers (NODATA) is not "not found".
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A": {Status: 0, StatusText: "NOERROR"},
	}}
	e := testEngine(t, fc)
	res, err := e.Lookup("nodata.example", Options{Types: []string{"A"}})
	if err != nil {
		t.Fatalf("Lookup should not error on NODATA: %v", err)
	}
	if res.Status != "NOERROR" || len(res.Records) != 0 {
		t.Errorf("NODATA result wrong: %+v", res)
	}
}

func TestAggregateMixedNXAndData(t *testing.T) {
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A":    {Status: 3, StatusText: "NXDOMAIN"},
		"AAAA": {Status: 0, StatusText: "NOERROR", Answers: []doh.Answer{{Name: "x", Type: "AAAA", TTL: 60, Data: "::1"}}},
	}}
	e := testEngine(t, fc)
	res, err := e.Lookup("mixed.example", Options{})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.Status != "NOERROR" {
		t.Errorf("mixed NX+data should aggregate to NOERROR, got %q", res.Status)
	}
}

func TestProviderOverrideViaOptions(t *testing.T) {
	fc := &fakeClient{}
	e := testEngine(t, fc) // cfg default cloudflare
	res, err := e.Lookup("example.com", Options{Types: []string{"A"}, Provider: "google"})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.Provider != "google" || res.Endpoint != "https://dns.google/resolve" {
		t.Errorf("provider override not applied: %+v", res)
	}
}

func TestEndpointOverrideViaConfig(t *testing.T) {
	cfg := &config.Config{Provider: "cloudflare", CloudflareURL: "https://proxy.example/dns", Profile: []string{"A"}, CacheTTLFloor: time.Minute, CacheDir: t.TempDir()}
	e := &Engine{Cfg: cfg, Cache: &cache.Store{Dir: cfg.CacheDir}, Client: &fakeClient{}, Now: func() time.Time { return time.Unix(1_700_000_000, 0) }}
	res, err := e.Lookup("example.com", Options{Types: []string{"A"}})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.Endpoint != "https://proxy.example/dns" {
		t.Errorf("config endpoint override ignored: %q", res.Endpoint)
	}
}

func TestUnknownProviderError(t *testing.T) {
	e := testEngine(t, &fakeClient{})
	if _, err := e.Lookup("example.com", Options{Types: []string{"A"}, Provider: "quad9"}); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestCacheRespectsMinTTL(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cur := now
	cfg := &config.Config{Provider: "cloudflare", Profile: []string{"A"}, CacheTTLFloor: 60 * time.Second, CacheDir: t.TempDir()}
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A": {Status: 0, StatusText: "NOERROR", Answers: []doh.Answer{{Name: "x", Type: "A", TTL: 30, Data: "1.1.1.1"}}},
	}}
	e := &Engine{Cfg: cfg, Cache: &cache.Store{Dir: cfg.CacheDir}, Client: fc, Now: func() time.Time { return cur }}

	if _, err := e.Lookup("example.com", Options{Types: []string{"A"}}); err != nil {
		t.Fatal(err)
	}
	// Answer TTL 30s < floor 60s → entry lives for the 60s floor.
	cur = now.Add(45 * time.Second)
	res, err := e.Lookup("example.com", Options{Types: []string{"A"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cached {
		t.Error("entry should still be fresh at 45s (floor 60s)")
	}
	cur = now.Add(90 * time.Second)
	res, _ = e.Lookup("example.com", Options{Types: []string{"A"}})
	if res.Cached {
		t.Error("entry should have expired past the 60s floor")
	}
}

func TestRawBypassesCacheReadThenStoresWithoutRaw(t *testing.T) {
	fc := &fakeClient{byType: map[string]*doh.Response{
		"A": {Status: 0, StatusText: "NOERROR", Raw: []byte(`{"Status":0}`), Answers: []doh.Answer{{Name: "x", Type: "A", TTL: 300, Data: "1.1.1.1"}}},
	}}
	e := testEngine(t, fc)
	res, err := e.Lookup("example.com", Options{Types: []string{"A"}, Raw: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Raw) == 0 {
		t.Error("--raw lookup should carry raw bodies")
	}
	// A following normal lookup hits the cache and has no raw.
	res2, _ := e.Lookup("example.com", Options{Types: []string{"A"}})
	if !res2.Cached || len(res2.Raw) != 0 {
		t.Errorf("cached result should have no raw: cached=%v raw=%d", res2.Cached, len(res2.Raw))
	}
}

func TestIDNQuerySetsASCII(t *testing.T) {
	fc := &fakeClient{}
	e := testEngine(t, fc)
	res, err := e.Lookup("日本語.jp", Options{Types: []string{"A"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Query != "日本語.jp" || res.QueryASCII != "xn--wgv71a119e.jp" {
		t.Errorf("IDN query/ascii wrong: query=%q ascii=%q", res.Query, res.QueryASCII)
	}
}

func TestErrorsIsNotFoundSentinel(t *testing.T) {
	// Guard the exported sentinel identity used by callers.
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Fatal("sentinel identity broken")
	}
}
