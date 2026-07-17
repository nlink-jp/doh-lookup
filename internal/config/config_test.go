package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseTOMLSections(t *testing.T) {
	in := `
# comment
[provider]
default = "google"
google_url = "https://example.test/resolve"

[query]
profile = "A, AAAA, MX"
suppress_ecs = false

[cache]
ttl_floor_seconds = 30

[network]
timeout_seconds = 5
`
	sections, err := parseTOML(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseTOML: %v", err)
	}
	cfg := &Config{Profile: nil, SuppressECS: true, CacheTTLFloor: DefaultCacheTTLFloor, Timeout: DefaultTimeout}
	if err := applySections(cfg, sections); err != nil {
		t.Fatalf("applySections: %v", err)
	}
	if cfg.Provider != "google" {
		t.Errorf("Provider = %q, want google", cfg.Provider)
	}
	if cfg.GoogleURL != "https://example.test/resolve" {
		t.Errorf("GoogleURL = %q", cfg.GoogleURL)
	}
	if len(cfg.Profile) != 3 || cfg.Profile[2] != "MX" {
		t.Errorf("Profile = %v, want [A AAAA MX]", cfg.Profile)
	}
	if cfg.SuppressECS {
		t.Error("SuppressECS = true, want false")
	}
	if cfg.CacheTTLFloor != 30*time.Second {
		t.Errorf("CacheTTLFloor = %v, want 30s", cfg.CacheTTLFloor)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", cfg.Timeout)
	}
}

func TestLoadDefaults(t *testing.T) {
	// A nonexistent config path falls back to built-in defaults.
	cfg, err := Load("/nonexistent/doh-lookup/config.toml", 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != DefaultProvider {
		t.Errorf("Provider = %q, want %q", cfg.Provider, DefaultProvider)
	}
	if !cfg.SuppressECS {
		t.Error("SuppressECS default = false, want true")
	}
	if len(cfg.Profile) == 0 {
		t.Error("Profile default is empty")
	}
}

func TestEnvOverride(t *testing.T) {
	t.Setenv("DOH_LOOKUP_PROVIDER", "google")
	t.Setenv("DOH_LOOKUP_PROFILE", "a,ns")
	cfg, err := Load("/nonexistent/config.toml", 0)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "google" {
		t.Errorf("Provider = %q, want google (env)", cfg.Provider)
	}
	if len(cfg.Profile) != 2 || cfg.Profile[0] != "A" || cfg.Profile[1] != "NS" {
		t.Errorf("Profile = %v, want [A NS]", cfg.Profile)
	}
}
