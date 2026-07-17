package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nlink-jp/doh-lookup/internal/engine"
)

func TestRunVersionAndUnknown(t *testing.T) {
	if code := Run([]string{"version"}, "1.2.3"); code != exitOK {
		t.Errorf("version exit = %d, want %d", code, exitOK)
	}
	if code := Run([]string{"bogus"}, "1.2.3"); code != exitError {
		t.Errorf("unknown-command exit = %d, want %d", code, exitError)
	}
	if code := Run(nil, "1.2.3"); code != exitError {
		t.Errorf("no-args exit = %d, want %d", code, exitError)
	}
}

func TestSplitTypes(t *testing.T) {
	got := splitTypes(" a, aaaa ,mx ")
	if len(got) != 3 || got[0] != "a" || got[2] != "mx" {
		t.Errorf("splitTypes = %v", got)
	}
	if splitTypes("") != nil {
		t.Errorf("splitTypes(empty) should be nil")
	}
}

func TestScanTargets(t *testing.T) {
	in := strings.NewReader("example.com\n# comment\n\n1.2.3.4 8.8.8.8\n")
	got := scanTargets(in)
	want := []string{"example.com", "1.2.3.4", "8.8.8.8"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("scanTargets = %v, want %v", got, want)
	}
}

func TestReadTargetsFromStdin(t *testing.T) {
	in := strings.NewReader("a.example\nb.example\n")
	got, err := readTargets(nil, "", in)
	if err != nil {
		t.Fatalf("readTargets: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("readTargets from stdin = %v", got)
	}
}

func TestReadTargetsPositionalsWinOverStdin(t *testing.T) {
	in := strings.NewReader("from-stdin.example\n")
	got, err := readTargets([]string{"positional.example"}, "", in)
	if err != nil {
		t.Fatalf("readTargets: %v", err)
	}
	if len(got) != 1 || got[0] != "positional.example" {
		t.Errorf("positionals should take precedence, got %v", got)
	}
}

func TestWriteJSONSingleVsBulk(t *testing.T) {
	one := []*engine.Result{{Query: "a.example", Status: "NOERROR"}}
	var buf bytes.Buffer
	writeJSON(&buf, one)
	if !strings.Contains(buf.String(), "\n  \"query\"") { // indented object
		t.Errorf("single result should be indented JSON:\n%s", buf.String())
	}

	two := []*engine.Result{{Query: "a.example"}, {Query: "b.example"}}
	buf.Reset()
	writeJSON(&buf, two)
	lines := strings.Count(strings.TrimSpace(buf.String()), "\n") + 1
	if lines != 2 {
		t.Errorf("bulk should be JSONL (2 lines), got %d:\n%s", lines, buf.String())
	}
}

func TestWriteTextRendersProvenance(t *testing.T) {
	res := []*engine.Result{{
		Query: "example.com", Kind: "forward", Status: "NOERROR", Authenticated: true,
		Provider: "cloudflare", Endpoint: "https://cloudflare-dns.com/dns-query",
		Records: []engine.Record{{Type: "A", Name: "example.com", TTL: 300, Data: "93.184.216.34"}},
	}}
	var buf bytes.Buffer
	writeText(&buf, res)
	out := buf.String()
	for _, want := range []string{"via cloudflare", "DNSSEC:validated", "93.184.216.34"} {
		if !strings.Contains(out, want) {
			t.Errorf("writeText output missing %q:\n%s", want, out)
		}
	}
}

func TestRunCacheStatusAndClear(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOH_LOOKUP_CACHE_DIR", dir)
	var out, errb bytes.Buffer
	if code := runCache([]string{"status"}, &out, &errb); code != exitOK {
		t.Fatalf("cache status exit = %d (%s)", code, errb.String())
	}
	if !strings.Contains(out.String(), dir) {
		t.Errorf("cache status did not report dir:\n%s", out.String())
	}
	out.Reset()
	if code := runCache([]string{"clear"}, &out, &errb); code != exitOK {
		t.Fatalf("cache clear exit = %d (%s)", code, errb.String())
	}
}
