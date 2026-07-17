//go:build e2e

// Live end-to-end tests against the real Cloudflare and Google DoH endpoints.
// Network is required. Run with: make e2e  (or go test -tags e2e ./e2e/...).
package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/engine"
)

func liveEngine(t *testing.T) *engine.Engine {
	t.Helper()
	// An absent config path → built-in defaults; isolate the cache in a temp
	// dir so repeated runs are hermetic and never read a stale answer.
	cfg, err := config.Load(filepath.Join(t.TempDir(), "absent.toml"), 0)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.CacheDir = t.TempDir()
	return engine.New(cfg, "e2e")
}

func TestLiveForwardBothProviders(t *testing.T) {
	e := liveEngine(t)
	for _, provider := range []string{"cloudflare", "google"} {
		t.Run(provider, func(t *testing.T) {
			res, err := e.Lookup("example.com", engine.Options{Types: []string{"A"}, Provider: provider, Refresh: true})
			if err != nil {
				t.Fatalf("lookup example.com via %s: %v", provider, err)
			}
			if res.Provider != provider {
				t.Errorf("provider = %q, want %q", res.Provider, provider)
			}
			if res.Status != "NOERROR" {
				t.Errorf("status = %q, want NOERROR", res.Status)
			}
			var haveA bool
			for _, rec := range res.Records {
				if rec.Type == "A" {
					haveA = true
				}
			}
			if !haveA {
				t.Errorf("no A record returned: %+v", res.Records)
			}
			t.Logf("%s: %d records, authenticated=%v, endpoint=%s", provider, len(res.Records), res.Authenticated, res.Endpoint)
		})
	}
}

func TestLiveDNSSECAuthenticated(t *testing.T) {
	// cloudflare.com is a DNSSEC-signed zone; the AD flag must come back set
	// (this is exactly the case that requires the DO bit on Cloudflare).
	e := liveEngine(t)
	res, err := e.Lookup("cloudflare.com", engine.Options{Types: []string{"A"}, Refresh: true})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !res.Authenticated {
		t.Errorf("cloudflare.com should be DNSSEC-authenticated (AD), got authenticated=false")
	}
}

func TestLiveReversePTR(t *testing.T) {
	e := liveEngine(t)
	res, err := e.Lookup("8.8.8.8", engine.Options{Refresh: true})
	if err != nil {
		t.Fatalf("reverse 8.8.8.8: %v", err)
	}
	if res.Kind != "reverse" {
		t.Errorf("kind = %q, want reverse", res.Kind)
	}
	var found bool
	for _, rec := range res.Records {
		if rec.Type == "PTR" && strings.Contains(rec.Data, "dns.google") {
			found = true
		}
	}
	if !found {
		t.Errorf("PTR for 8.8.8.8 did not contain dns.google: %+v", res.Records)
	}
}

func TestLiveNXDOMAIN(t *testing.T) {
	e := liveEngine(t)
	// .example is reserved and never resolves.
	res, err := e.Lookup("no-such-host-doh-lookup-e2e.example", engine.Options{Types: []string{"A"}, Refresh: true})
	if err != engine.ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if res == nil || res.Status != "NXDOMAIN" {
		t.Errorf("want populated NXDOMAIN result, got %+v", res)
	}
}

func TestLiveProfileBundle(t *testing.T) {
	// The default profile (no explicit types) should return a mix of record
	// types for a rich domain, with RRSIG filtered out.
	e := liveEngine(t)
	res, err := e.Lookup("cloudflare.com", engine.Options{Refresh: true})
	if err != nil {
		t.Fatalf("profile lookup: %v", err)
	}
	seen := map[string]bool{}
	for _, rec := range res.Records {
		seen[rec.Type] = true
		if rec.Type == "RRSIG" {
			t.Errorf("RRSIG leaked into normalized records: %+v", rec)
		}
	}
	if !seen["A"] || !seen["NS"] {
		t.Errorf("profile bundle missing expected types; saw %v", seen)
	}
	t.Logf("profile types seen: %v", seen)
}
