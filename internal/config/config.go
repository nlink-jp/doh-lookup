package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nlink-jp/doh-lookup/internal/doh"
)

const (
	// DefaultProvider is the DoH provider used when none is configured.
	// Cloudflare has a clear privacy stance and does not forward ECS.
	DefaultProvider = "cloudflare"
	// DefaultTimeout bounds each DoH HTTPS exchange.
	DefaultTimeout = 10 * time.Second
	// DefaultCacheTTLFloor is the minimum time a cached answer stays fresh,
	// even when the record's own TTL is shorter — it bounds how often a bulk
	// sweep can re-hit the resolver.
	DefaultCacheTTLFloor = 60 * time.Second
)

// Config holds resolved runtime settings. No credentials: every endpoint is
// public.
type Config struct {
	Provider      string        // default DoH provider: cloudflare | google
	CloudflareURL string        // endpoint override ("" = built-in)
	GoogleURL     string        // endpoint override ("" = built-in)
	Profile       []string      // record types fetched when --type is omitted
	SuppressECS   bool          // ask the resolver not to forward EDNS Client Subnet
	CacheDir      string        // answer-cache directory
	CacheTTLFloor time.Duration // minimum answer freshness
	Timeout       time.Duration // network timeout per exchange
}

// Load resolves configuration. If configPath is empty the default location
// (~/.config/doh-lookup/config.toml) is used when present. Environment
// variables override file values; a non-zero timeoutOverride wins over both.
func Load(configPath string, timeoutOverride time.Duration) (*Config, error) {
	cfg := &Config{
		Provider:      DefaultProvider,
		Profile:       append([]string(nil), doh.DefaultProfile...),
		SuppressECS:   true,
		CacheDir:      DefaultCacheDir(),
		CacheTTLFloor: DefaultCacheTTLFloor,
		Timeout:       DefaultTimeout,
	}

	if configPath == "" {
		configPath = DefaultConfigPath()
	}
	if configPath != "" {
		if f, err := os.Open(configPath); err == nil {
			defer f.Close()
			sections, perr := parseTOML(f)
			if perr != nil {
				return nil, fmt.Errorf("parse config %s: %w", configPath, perr)
			}
			if aerr := applySections(cfg, sections); aerr != nil {
				return nil, fmt.Errorf("config %s: %w", configPath, aerr)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("open config %s: %w", configPath, err)
		}
	}

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	if timeoutOverride > 0 {
		cfg.Timeout = timeoutOverride
	}
	return cfg, nil
}

func applySections(cfg *Config, sections map[string]map[string]string) error {
	if p := sections["provider"]; p != nil {
		if v := p["default"]; v != "" {
			cfg.Provider = strings.ToLower(v)
		}
		if v := p["cloudflare_url"]; v != "" {
			cfg.CloudflareURL = v
		}
		if v := p["google_url"]; v != "" {
			cfg.GoogleURL = v
		}
	}
	if q := sections["query"]; q != nil {
		if v := q["profile"]; v != "" {
			cfg.Profile = splitProfile(v)
		}
		if v := q["suppress_ecs"]; v != "" {
			b, err := parseBool(v)
			if err != nil {
				return fmt.Errorf("[query] suppress_ecs: %w", err)
			}
			cfg.SuppressECS = b
		}
	}
	if c := sections["cache"]; c != nil {
		if v := c["ttl_floor_seconds"]; v != "" {
			d, err := parseSeconds(v)
			if err != nil {
				return fmt.Errorf("[cache] ttl_floor_seconds: %w", err)
			}
			cfg.CacheTTLFloor = d
		}
		if v := c["dir"]; v != "" {
			cfg.CacheDir = expandHome(v)
		}
	}
	if n := sections["network"]; n != nil {
		if v := n["timeout_seconds"]; v != "" {
			d, err := parseSeconds(v)
			if err != nil {
				return fmt.Errorf("[network] timeout_seconds: %w", err)
			}
			cfg.Timeout = d
		}
	}
	return nil
}

func applyEnv(cfg *Config) error {
	if v := os.Getenv("DOH_LOOKUP_PROVIDER"); v != "" {
		cfg.Provider = strings.ToLower(v)
	}
	if v := os.Getenv("DOH_LOOKUP_CLOUDFLARE_URL"); v != "" {
		cfg.CloudflareURL = v
	}
	if v := os.Getenv("DOH_LOOKUP_GOOGLE_URL"); v != "" {
		cfg.GoogleURL = v
	}
	if v := os.Getenv("DOH_LOOKUP_PROFILE"); v != "" {
		cfg.Profile = splitProfile(v)
	}
	if v := os.Getenv("DOH_LOOKUP_SUPPRESS_ECS"); v != "" {
		b, err := parseBool(v)
		if err != nil {
			return fmt.Errorf("DOH_LOOKUP_SUPPRESS_ECS: %w", err)
		}
		cfg.SuppressECS = b
	}
	if v := os.Getenv("DOH_LOOKUP_CACHE_DIR"); v != "" {
		cfg.CacheDir = expandHome(v)
	}
	if v := os.Getenv("DOH_LOOKUP_CACHE_TTL_FLOOR_SECONDS"); v != "" {
		d, err := parseSeconds(v)
		if err != nil {
			return fmt.Errorf("DOH_LOOKUP_CACHE_TTL_FLOOR_SECONDS: %w", err)
		}
		cfg.CacheTTLFloor = d
	}
	if v := os.Getenv("DOH_LOOKUP_TIMEOUT_SECONDS"); v != "" {
		d, err := parseSeconds(v)
		if err != nil {
			return fmt.Errorf("DOH_LOOKUP_TIMEOUT_SECONDS: %w", err)
		}
		cfg.Timeout = d
	}
	return nil
}

// splitProfile parses a comma-separated record-type list, trimming and
// upper-casing each entry and dropping empties. Type validity is checked at
// query time by doh.NormalizeType.
func splitProfile(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("%q is not a boolean", v)
}

func parseSeconds(v string) (time.Duration, error) {
	s, err := strconv.ParseFloat(v, 64)
	if err != nil || s <= 0 {
		return 0, fmt.Errorf("%q is not a positive number", v)
	}
	return time.Duration(s * float64(time.Second)), nil
}

// DefaultConfigPath returns the default config file location, honoring
// XDG_CONFIG_HOME.
func DefaultConfigPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "doh-lookup", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "doh-lookup", "config.toml")
}

// DefaultCacheDir returns the default cache directory, honoring
// XDG_CACHE_HOME. Cached answers are re-fetchable transient state, so they
// belong under the cache home, not data.
func DefaultCacheDir() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "doh-lookup")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "doh-lookup-cache"
	}
	return filepath.Join(home, ".cache", "doh-lookup")
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// parseTOML parses the minimal subset doh-lookup needs: [section] headers and
// key = value lines, where value is an optionally quoted string. Comments
// start with '#'. It intentionally does not support arrays, nested tables, or
// typed values.
func parseTOML(r io.Reader) (map[string]map[string]string, error) {
	sections := map[string]map[string]string{}
	current := ""
	sections[current] = map[string]string{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if strings.HasPrefix(raw, "[") {
			end := strings.IndexByte(raw, ']')
			if end < 0 {
				return nil, fmt.Errorf("line %d: unterminated section header", line)
			}
			current = strings.TrimSpace(raw[1:end])
			if _, ok := sections[current]; !ok {
				sections[current] = map[string]string{}
			}
			continue
		}
		eq := strings.IndexByte(raw, '=')
		if eq < 0 {
			return nil, fmt.Errorf("line %d: expected key = value", line)
		}
		key := strings.TrimSpace(raw[:eq])
		val := parseValue(strings.TrimSpace(raw[eq+1:]))
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", line)
		}
		sections[current][key] = val
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return sections, nil
}

// parseValue strips surrounding quotes, or trims a trailing inline comment
// from a bare value.
func parseValue(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		q := v[0]
		if end := strings.IndexByte(v[1:], q); end >= 0 {
			return v[1 : 1+end]
		}
	}
	if hash := strings.IndexByte(v, '#'); hash >= 0 {
		v = strings.TrimSpace(v[:hash])
	}
	return v
}
