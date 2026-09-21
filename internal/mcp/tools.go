package mcp

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"

	"github.com/nlink-jp/doh-lookup/internal/engine"
	"github.com/nlink-jp/doh-lookup/internal/query"
)

// usageMarkdown is the operating manual returned by the get_usage tool. Its
// coherence with the real tools/results is pinned by usage_test.go.
//
//go:embed usage.md
var usageMarkdown string

// Instructions is the initialize-time hint (surfaced via the MCP
// `instructions` field) that makes get_usage discoverable and steers clients
// away from common errors.
const Instructions = "doh-lookup collects a domain's DNS records over DoH (DNS over HTTPS) from a public " +
	"resolver (Cloudflare or Google), out-of-band over HTTPS so investigative queries stay distinguishable " +
	"from ordinary DNS. Call the lookup tool with a single domain or IP; a domain is forward-resolved (the " +
	"configured record-type profile, or an explicit list) and an IP is reverse-resolved (PTR). Every result " +
	"states the resolver, endpoint, and DNSSEC AD flag. Tool errors are structured JSON ({code, message}). " +
	"Call get_usage for the full tool reference and error-recovery table. No credentials are required."

// obj builds a tool's input schema. Every schema goes through here so that
// org ADR-021 §10's `additionalProperties: false` is set once instead of being
// remembered per tool — the next tool added gets the closed schema for free.
func obj(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// decodeArgs decodes a tool's arguments strictly: an argument the tool does not
// declare is refused by name, and a malformed argument object is refused rather
// than read as an empty one. Every tool decodes through here.
//
// obj() above is only the declared half of org ADR-021 §4 — what a
// schema-checking client refuses before the call. This is the half that
// actually refuses, and it is needed because not every client checks the
// schema. The `_ = json.Unmarshal` this replaces discarded the decode error as
// well as the unknown field, so both defects were silent in the same way: a
// misspelt `types` collected the configured profile and returned it as the
// record types asked for, and `{"query": 1}` ran as if no target had been
// named.
func decodeArgs(raw json.RawMessage, into any) error {
	raw = bytes.TrimSpace(raw)
	// Omitted or null arguments mean the empty object, not an error: a tool
	// whose arguments are all optional is legitimately called with none.
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return errors.New("arguments: " + err.Error())
	}
	return nil
}

// toolsList returns the advertised tool set with JSON Schema for each input.
func toolsList() any {
	return map[string]any{
		"tools": []map[string]any{
			{
				"name":        "get_usage",
				"description": "Return this server's operating manual (markdown): the tools, the result schema, and the error-recovery table. Call it once before first use.",
				"inputSchema": obj(map[string]any{}),
			},
			{
				"name":        "lookup",
				"description": "Collect DNS records for a domain (forward) or IP (PTR reverse) over DoH. A domain with no explicit types uses the configured profile (A/AAAA/MX/TXT/NS/SOA/CAA). The result records which resolver/endpoint answered and the DNSSEC AD flag. Answers are cached locally, honoring DNS TTLs.",
				"inputSchema": obj(map[string]any{
					"query":    map[string]any{"type": "string", "description": "Domain name (IDN ok) or IP address."},
					"types":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Record types (e.g. [\"A\",\"MX\"]). Ignored for IPs (always PTR). Default: the configured profile."},
					"provider": map[string]any{"type": "string", "enum": []string{"cloudflare", "google"}, "description": "DoH provider (default: configured, usually cloudflare)."},
					"cd":       map[string]any{"type": "boolean", "description": "Checking disabled: return records even if DNSSEC validation fails."},
					"refresh":  map[string]any{"type": "boolean", "description": "Bypass the local cache and re-query."},
					"raw":      map[string]any{"type": "boolean", "description": "Include each resolver's raw JSON response."},
				}, "query"),
			},
			{
				"name":        "cache_status",
				"description": "Report the local answer-cache state: entry count, TTL floor, and the default provider.",
				"inputSchema": obj(map[string]any{}),
			},
		},
	}
}

func (s *server) toolsCall(params json.RawMessage) (toolResult, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolResult{}, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	switch p.Name {
	case "get_usage":
		// No arguments — which still means "none", not "any".
		if err := decodeArgs(p.Arguments, &struct{}{}); err != nil {
			return errorResult("invalid_input", err.Error()), nil
		}
		return textResult(false, usageMarkdown), nil
	case "lookup":
		return s.toolLookup(p.Arguments), nil
	case "cache_status":
		if err := decodeArgs(p.Arguments, &struct{}{}); err != nil {
			return errorResult("invalid_input", err.Error()), nil
		}
		return s.toolCacheStatus(), nil
	default:
		return toolResult{}, &rpcError{Code: -32602, Message: "unknown tool: " + p.Name}
	}
}

func (s *server) toolLookup(args json.RawMessage) toolResult {
	var a struct {
		Query    string   `json:"query"`
		Types    []string `json:"types"`
		Provider string   `json:"provider"`
		CD       bool     `json:"cd"`
		Refresh  bool     `json:"refresh"`
		Raw      bool     `json:"raw"`
	}
	if err := decodeArgs(args, &a); err != nil {
		return errorResult("invalid_input", err.Error())
	}
	if a.Query == "" {
		return errorResult("invalid_input", "provide 'query' (a domain name or IP address)")
	}
	res, err := s.e.Lookup(a.Query, engine.Options{
		Types:    a.Types,
		Provider: a.Provider,
		CD:       a.CD,
		Refresh:  a.Refresh,
		Raw:      a.Raw,
	})
	switch {
	case errors.Is(err, query.ErrInvalid):
		return errorResult("invalid_input", err.Error())
	case errors.Is(err, engine.ErrNotFound):
		// NXDOMAIN is a valid, informative answer for a DNS tool: return the
		// populated result (status NXDOMAIN), not a bare error.
		return jsonResult(res)
	case err != nil:
		return errorResult("network_error", err.Error())
	}
	return jsonResult(res)
}

func (s *server) toolCacheStatus() toolResult {
	return jsonResult(map[string]any{
		"cache_dir":         s.e.Cfg.CacheDir,
		"entries":           s.e.Cache.Count(),
		"ttl_floor_seconds": int(s.e.Cfg.CacheTTLFloor.Seconds()),
		"provider":          s.e.Cfg.Provider,
		"profile":           s.e.Cfg.Profile,
	})
}

// errorResult renders a structured tool error: {code, message}. Codes:
// invalid_input, network_error.
func errorResult(code, message string) toolResult {
	b, _ := json.Marshal(map[string]string{"code": code, "message": message})
	return textResult(true, string(b))
}

// jsonResult marshals v into a non-error text result.
func jsonResult(v any) toolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult("network_error", "encode result: "+err.Error())
	}
	return textResult(false, string(b))
}
