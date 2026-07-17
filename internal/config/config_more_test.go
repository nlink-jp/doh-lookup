package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTOMLErrors(t *testing.T) {
	bad := []string{
		"[unterminated\nkey = v",
		"novalue\n",
		"= orphan\n",
	}
	for _, in := range bad {
		if _, err := parseTOML(strings.NewReader(in)); err == nil {
			t.Errorf("parseTOML(%q) = nil error, want error", in)
		}
	}
}

func TestApplySectionsErrors(t *testing.T) {
	cases := []map[string]map[string]string{
		{"query": {"suppress_ecs": "maybe"}},
		{"cache": {"ttl_floor_seconds": "-3"}},
		{"network": {"timeout_seconds": "abc"}},
	}
	for _, sections := range cases {
		cfg := &Config{}
		if err := applySections(cfg, sections); err == nil {
			t.Errorf("applySections(%v) = nil, want error", sections)
		}
	}
}

func TestApplyEnvErrors(t *testing.T) {
	t.Setenv("DOH_LOOKUP_SUPPRESS_ECS", "notabool")
	if _, err := Load("/nonexistent.toml", 0); err == nil {
		t.Error("bad DOH_LOOKUP_SUPPRESS_ECS should fail Load")
	}
}

func TestLoadFromFileAllSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[provider]
default = "google"
cloudflare_url = "https://cf.example/dns-query"
google_url = "https://g.example/resolve"

[query]
profile = "A, NS"
suppress_ecs = false

[cache]
ttl_floor_seconds = 15
dir = "~/somewhere"

[network]
timeout_seconds = 3
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "google" || cfg.CloudflareURL != "https://cf.example/dns-query" {
		t.Errorf("provider section not applied: %+v", cfg)
	}
	if len(cfg.Profile) != 2 || cfg.SuppressECS {
		t.Errorf("query section not applied: %+v", cfg)
	}
	if cfg.CacheTTLFloor != 15*time.Second || cfg.Timeout != 3*time.Second {
		t.Errorf("cache/network not applied: %+v", cfg)
	}
	if strings.HasPrefix(cfg.CacheDir, "~") {
		t.Errorf("~ not expanded in cache dir: %q", cfg.CacheDir)
	}
}

func TestLoadTimeoutOverridePrecedence(t *testing.T) {
	t.Setenv("DOH_LOOKUP_TIMEOUT_SECONDS", "7")
	cfg, err := Load("/nonexistent.toml", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Timeout != 2*time.Second {
		t.Errorf("flag override should win: got %v, want 2s", cfg.Timeout)
	}
}

func TestParseValue(t *testing.T) {
	cases := map[string]string{
		`"quoted"`:       "quoted",
		`'single'`:       "single",
		`bare # comment`: "bare",
		`plain`:          "plain",
	}
	for in, want := range cases {
		if got := parseValue(in); got != want {
			t.Errorf("parseValue(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultPathsHonorXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg/cfg")
	t.Setenv("XDG_CACHE_HOME", "/xdg/cache")
	if p := DefaultConfigPath(); p != "/xdg/cfg/doh-lookup/config.toml" {
		t.Errorf("DefaultConfigPath = %q", p)
	}
	if p := DefaultCacheDir(); p != "/xdg/cache/doh-lookup" {
		t.Errorf("DefaultCacheDir = %q", p)
	}
}
