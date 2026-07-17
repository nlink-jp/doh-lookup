package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/nlink-jp/doh-lookup/internal/cache"
	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/doh"
	"github.com/nlink-jp/doh-lookup/internal/query"
)

// ErrNotFound means every record-type query returned NXDOMAIN: the name does
// not exist. It accompanies a populated Result (Status NXDOMAIN) so bulk
// callers can still render the target.
var ErrNotFound = errors.New("name does not exist")

// DoHClient is the resolver-facing surface (the doh package; faked in tests).
type DoHClient interface {
	Query(p doh.Provider, name, rrType string, cd bool) (*doh.Response, error)
}

// Options modify a single lookup.
type Options struct {
	Types    []string // explicit record types; empty ⇒ configured profile
	Provider string   // override the configured provider ("" ⇒ config default)
	CD       bool     // checking disabled (return records despite DNSSEC failure)
	Refresh  bool     // bypass the answer cache
	Raw      bool     // include each resolver's raw JSON response
}

// Record is one normalized resource record in a Result.
type Record struct {
	Type string `json:"type"`
	Name string `json:"name"`
	TTL  int    `json:"ttl"`
	Data string `json:"data"`
}

// Result is the aggregated answer for one target, carrying the provenance
// (provider + endpoint) that makes the lookup distinguishable.
type Result struct {
	Query         string            `json:"query"`
	QueryASCII    string            `json:"query_ascii,omitempty"`
	Kind          string            `json:"kind"` // "forward" | "reverse"
	Provider      string            `json:"provider"`
	Endpoint      string            `json:"endpoint"`
	Types         []string          `json:"types"`
	Status        string            `json:"status"`        // aggregate RCODE mnemonic
	Authenticated bool              `json:"authenticated"` // DNSSEC AD across queries
	Records       []Record          `json:"records"`
	Raw           []json.RawMessage `json:"raw,omitempty"` // per-type resolver responses (--raw)
	Cached        bool              `json:"cached,omitempty"`
	QueriedAt     string            `json:"queried_at"`
}

// Engine ties the pieces together. It is shared by the CLI and MCP server.
type Engine struct {
	Cfg    *config.Config
	Cache  *cache.Store
	Client DoHClient
	Now    func() time.Time
}

// New wires a production engine from resolved configuration. The User-Agent
// identifies the tool to the resolver — part of keeping the query
// distinguishable rather than masquerading as a browser.
func New(cfg *config.Config, version string) *Engine {
	ua := "doh-lookup/" + version + " (+https://github.com/nlink-jp/doh-lookup)"
	return &Engine{
		Cfg:   cfg,
		Cache: &cache.Store{Dir: cfg.CacheDir},
		Client: &doh.Client{
			HTTP:        &http.Client{Timeout: cfg.Timeout},
			UserAgent:   ua,
			SuppressECS: cfg.SuppressECS,
		},
		Now: time.Now,
	}
}

// Lookup validates the target, resolves the provider, consults the cache, and
// (on a miss) queries the DoH endpoint for each requested type, aggregating
// the answers. Invalid input never reaches the network (query.ErrInvalid); a
// nonexistent name returns ErrNotFound with a populated NXDOMAIN Result.
func (e *Engine) Lookup(input string, opts Options) (*Result, error) {
	target, err := query.Classify(input)
	if err != nil {
		return nil, err
	}

	provName := opts.Provider
	if provName == "" {
		provName = e.Cfg.Provider
	}
	prov, err := e.provider(provName)
	if err != nil {
		return nil, err
	}

	types, err := e.resolveTypes(target, opts)
	if err != nil {
		return nil, err
	}

	qname := target.Value
	kind := "forward"
	if target.Kind == query.KindIP {
		qname = target.ReverseName()
		kind = "reverse"
	}

	key := cache.Key(prov.Name, kind, qname, strings.Join(types, "-"), cdTag(opts.CD))
	now := e.Now()

	// A --raw request always re-fetches: the cache stores results without the
	// bulky per-type raw bodies, so serving raw from cache would be empty.
	if !opts.Refresh && !opts.Raw {
		if raw, ok := e.Cache.Get(key, now); ok {
			var res Result
			if json.Unmarshal(raw, &res) == nil {
				res.Cached = true
				return &res, notFoundIf(res.Status)
			}
		}
	}

	res := &Result{
		Query:     target.Original,
		Kind:      kind,
		Provider:  prov.Name,
		Endpoint:  prov.Endpoint,
		Types:     types,
		QueriedAt: now.UTC().Format(time.RFC3339),
	}
	if target.Original != target.Value {
		res.QueryASCII = target.Value
	}

	minTTL := 0
	nxCount, okCount := 0, 0
	var softStatus string
	for _, tp := range types {
		resp, qerr := e.Client.Query(prov, qname, tp, opts.CD)
		if qerr != nil {
			return nil, qerr // transport/HTTP failure is fatal to the whole lookup
		}
		if resp.Authenticated {
			res.Authenticated = true
		}
		if opts.Raw && len(resp.Raw) > 0 {
			res.Raw = append(res.Raw, resp.Raw)
		}
		switch resp.Status {
		case 3: // NXDOMAIN
			nxCount++
		case 0: // NOERROR (may be NODATA — no answers)
			okCount++
		default:
			if softStatus == "" {
				softStatus = resp.StatusText
			}
		}
		for _, a := range resp.Answers {
			// The DO bit (set so the AD flag is meaningful, esp. on Cloudflare)
			// makes the resolver attach RRSIG/NSEC proof records. They are not
			// answer data — drop them unless this query explicitly asked for
			// that type. The DNSSEC signal is carried by res.Authenticated.
			if doh.IsDNSSECMeta(a.Type) && a.Type != tp {
				continue
			}
			res.Records = append(res.Records, Record{Type: a.Type, Name: a.Name, TTL: a.TTL, Data: a.Data})
			if a.TTL > 0 && (minTTL == 0 || a.TTL < minTTL) {
				minTTL = a.TTL
			}
		}
	}

	res.Status = aggregateStatus(len(types), nxCount, okCount, softStatus)

	ttl := e.Cfg.CacheTTLFloor
	if d := time.Duration(minTTL) * time.Second; d > ttl {
		ttl = d
	}
	// Cache without the bulky raw bodies (they are only for the immediate
	// --raw view); restore afterward so the caller still sees them.
	rawSaved := res.Raw
	res.Raw = nil
	if b, merr := json.Marshal(res); merr == nil {
		_ = e.Cache.Put(key, b, now, ttl) // best-effort; a cache miss must not fail a lookup
	}
	res.Raw = rawSaved

	return res, notFoundIf(res.Status)
}

// provider resolves a provider name plus any per-provider endpoint override
// from config.
func (e *Engine) provider(name string) (doh.Provider, error) {
	override := ""
	switch strings.ToLower(name) {
	case "cloudflare":
		override = e.Cfg.CloudflareURL
	case "google":
		override = e.Cfg.GoogleURL
	}
	return doh.ProviderByName(name, override)
}

// resolveTypes returns the validated, de-duplicated record types to query. An
// IP target is always a single PTR reverse query; a domain uses the explicit
// types or the configured profile.
func (e *Engine) resolveTypes(target query.Target, opts Options) ([]string, error) {
	if target.Kind == query.KindIP {
		return []string{"PTR"}, nil
	}
	raw := opts.Types
	if len(raw) == 0 {
		raw = e.Cfg.Profile
	}
	var out []string
	seen := map[string]bool{}
	for _, x := range raw {
		name, _, err := doh.NormalizeType(x)
		if err != nil {
			return nil, err
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no record types to query")
	}
	return out, nil
}

// aggregateStatus reduces the per-type response codes to one mnemonic:
// NOERROR if any type answered (or returned NODATA), NXDOMAIN if every type
// said the name does not exist, otherwise the first soft failure seen.
func aggregateStatus(total, nx, ok int, soft string) string {
	switch {
	case ok > 0:
		return "NOERROR"
	case nx == total && total > 0:
		return "NXDOMAIN"
	case soft != "":
		return soft
	default:
		return "NOERROR"
	}
}

func notFoundIf(status string) error {
	if status == "NXDOMAIN" {
		return ErrNotFound
	}
	return nil
}

func cdTag(cd bool) string {
	if cd {
		return "cd"
	}
	return "std"
}

// SortedProfile returns the configured profile sorted, for stable display.
func SortedProfile(p []string) []string {
	out := append([]string(nil), p...)
	sort.Strings(out)
	return out
}
