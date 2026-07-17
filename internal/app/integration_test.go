package app

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// dohStack stands up an httptest server speaking the JSON DoH API and points
// the CLI at it via env, so runLookup exercises the full flags → config →
// engine → doh → output path without the network. It returns the server and a
// pointer to the request counter (for cache assertions).
func dohStack(t *testing.T) *int32 {
	t.Helper()
	var count int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		name := strings.TrimSuffix(r.URL.Query().Get("name"), ".")
		typ := r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/dns-json")
		switch {
		case strings.Contains(name, "nxdomain"):
			fmt.Fprint(w, `{"Status":3,"AD":false}`)
		case strings.Contains(name, "boom"):
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "upstream error")
		case typ == "A":
			// Include an RRSIG (type 46) to prove it gets filtered from output.
			fmt.Fprint(w, `{"Status":0,"AD":true,"Answer":[
				{"name":"`+name+`.","type":1,"TTL":300,"data":"93.184.216.34"},
				{"name":"`+name+`.","type":46,"TTL":300,"data":"A 13 2 300 sig"}]}`)
		case typ == "PTR":
			fmt.Fprint(w, `{"Status":0,"AD":false,"Answer":[{"name":"`+name+`.","type":12,"TTL":3600,"data":"dns.google."}]}`)
		default:
			fmt.Fprint(w, `{"Status":0,"AD":true}`) // NODATA
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Isolate from the user's real config/cache and route both providers at
	// the test server.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DOH_LOOKUP_CACHE_DIR", t.TempDir())
	t.Setenv("DOH_LOOKUP_CLOUDFLARE_URL", srv.URL)
	t.Setenv("DOH_LOOKUP_GOOGLE_URL", srv.URL)
	return &count
}

func TestRunLookupTextForward(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	code := runLookup([]string{"--type", "A", "example.com"}, "test", &out, &errb)
	if code != exitOK {
		t.Fatalf("exit = %d, stderr=%s", code, errb.String())
	}
	s := out.String()
	for _, want := range []string{"via cloudflare", "DNSSEC:validated", "93.184.216.34"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "RRSIG") || strings.Contains(s, "type 46") || strings.Contains(s, "13 2 300 sig") {
		t.Errorf("RRSIG leaked into output:\n%s", s)
	}
}

func TestRunLookupJSONSingle(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	if code := runLookup([]string{"--type", "A", "--json", "example.com"}, "test", &out, &errb); code != exitOK {
		t.Fatalf("exit = %d, %s", code, errb.String())
	}
	// A single target renders one indented object (starts with '{').
	if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Errorf("expected single JSON object:\n%s", out.String())
	}
	if !strings.Contains(out.String(), `"provider": "cloudflare"`) {
		t.Errorf("missing provenance:\n%s", out.String())
	}
}

func TestRunLookupBulkJSONL(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	code := runLookup([]string{"--type", "A", "--json", "a.example", "b.example"}, "test", &out, &errb)
	if code != exitOK {
		t.Fatalf("exit = %d, %s", code, errb.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d:\n%s", len(lines), out.String())
	}
	for _, ln := range lines {
		if !strings.HasPrefix(ln, "{") || !strings.HasSuffix(ln, "}") {
			t.Errorf("line is not compact JSON: %s", ln)
		}
	}
}

func TestRunLookupNXDOMAINExit(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	code := runLookup([]string{"--type", "A", "nxdomain.example"}, "test", &out, &errb)
	if code != exitNotFound {
		t.Fatalf("exit = %d, want %d", code, exitNotFound)
	}
	if !strings.Contains(out.String(), "name does not exist") {
		t.Errorf("expected NXDOMAIN note:\n%s", out.String())
	}
}

func TestRunLookupNetworkErrorExit(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	code := runLookup([]string{"--type", "A", "boom.example"}, "test", &out, &errb)
	if code != exitError {
		t.Fatalf("exit = %d, want %d (stderr=%s)", code, exitError, errb.String())
	}
	if !strings.Contains(errb.String(), "boom.example") {
		t.Errorf("expected per-target error on stderr:\n%s", errb.String())
	}
}

func TestRunLookupPTR(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	if code := runLookup([]string{"8.8.8.8"}, "test", &out, &errb); code != exitOK {
		t.Fatalf("exit = %d, %s", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "reverse") || !strings.Contains(s, "dns.google") {
		t.Errorf("PTR output wrong:\n%s", s)
	}
}

func TestRunLookupProviderGoogle(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	if code := runLookup([]string{"--type", "A", "--provider", "google", "example.com"}, "test", &out, &errb); code != exitOK {
		t.Fatalf("exit = %d, %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "via google") {
		t.Errorf("expected google provenance:\n%s", out.String())
	}
}

func TestRunLookupRawIncludesBody(t *testing.T) {
	dohStack(t)
	var out, errb bytes.Buffer
	if code := runLookup([]string{"--type", "A", "--raw", "--json", "example.com"}, "test", &out, &errb); code != exitOK {
		t.Fatalf("exit = %d, %s", code, errb.String())
	}
	if !strings.Contains(out.String(), `"raw"`) {
		t.Errorf("--raw did not include raw body:\n%s", out.String())
	}
}

func TestRunLookupCacheHitAvoidsSecondRequest(t *testing.T) {
	count := dohStack(t)
	var out, errb bytes.Buffer
	runLookup([]string{"--type", "A", "example.com"}, "test", &out, &errb)
	first := atomic.LoadInt32(count)
	out.Reset()
	runLookup([]string{"--type", "A", "example.com"}, "test", &out, &errb)
	if atomic.LoadInt32(count) != first {
		t.Errorf("second lookup hit the network: %d → %d", first, atomic.LoadInt32(count))
	}
	if !strings.Contains(out.String(), "cached") {
		t.Errorf("second lookup not marked cached:\n%s", out.String())
	}
}

func TestRunLookupStdinBulk(t *testing.T) {
	dohStack(t)
	saved := stdin
	stdin = strings.NewReader("a.example\nb.example\n")
	t.Cleanup(func() { stdin = saved })
	var out, errb bytes.Buffer
	if code := runLookup([]string{"--type", "A"}, "test", &out, &errb); code != exitOK {
		t.Fatalf("exit = %d, %s", code, errb.String())
	}
	if strings.Count(out.String(), "via cloudflare") != 2 {
		t.Errorf("expected 2 results from stdin:\n%s", out.String())
	}
}
