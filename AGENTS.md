# AGENTS.md — doh-lookup

## What this is

A CLI + local MCP server that collects a domain's **DNS records over DoH**
(DNS over HTTPS) from a public resolver (**Cloudflare** or **Google**, JSON DoH
API). Unlike `dig` — which queries the OS resolver over UDP/53 and is
indistinguishable from ordinary traffic — every query goes **out-of-band over
HTTPS/443** and each result records **which resolver and endpoint answered**
plus the **DNSSEC AD flag**, so investigative lookups stay explicitly
distinguishable from an organization's normal DNS (an OpSec separation: no
resolver-cache pollution, no blending into browsing noise). A domain is
forward-resolved (a record-type profile, or an explicit list); an IP is
reverse-resolved (PTR). Zero credentials, zero external dependencies. The
DNS-resolution sibling of `asn-lookup` (attribution), `abuse-lookup`
(reputation), `whois-lookup` (registration), and `tor-exit-lookup` (Tor exit
membership).

## Build & test

```bash
make build      # → dist/doh-lookup  (NEVER `go build` directly)
make test       # go test -race -cover ./...  (offline; mocked HTTP)
make check      # lint + test + build-all
make build-all  # cross-compile linux/{amd64,arm64}, darwin/arm64, windows/amd64
make e2e        # LIVE end-to-end vs real Cloudflare/Google DoH (network required)
```

Go 1.25+. **No external dependencies** — standard library only.

## Tests

- **Offline unit/integration** (`make test`): every package has table-driven
  tests; the HTTP transport is an injected `Doer` (doh) or a real
  `httptest.Server` reached via `DOH_LOOKUP_*_URL` env (app), so the full
  flags → config → engine → doh → output path is exercised without the network.
  ~92 test functions; coverage 74–93% per package.
- **Live E2E** (`make e2e`, network required):
  - `e2e/live_test.go` — `//go:build e2e` Go tests hitting real
    Cloudflare + Google (both providers, DNSSEC AD on a signed zone, PTR
    reverse, NXDOMAIN, profile bundle with RRSIG filtered). Excluded from
    `go test ./...` by the build tag; `e2e/doc.go` keeps the package compiling.
  - `scripts/e2e.sh` — drives the **built binary** end-to-end (lookup output,
    exit-code contract, JSON provenance, bulk stdin, MCP stdio round-trip);
    prints a PASS/FAIL summary, isolates the cache in a tempdir.

## Layout

```
main.go                 Entry point; sets main.version, calls app.Run.
internal/query/         Input classification (domain vs IP) + validation gate + PTR name.
internal/idn/           In-house RFC 3492 punycode (U-label → A-label).
internal/doh/           JSON DoH client (Cloudflare/Google), type tables, lenient decode.
internal/cache/         Per-answer TTL cache (per-record expiry), atomic writes.
internal/config/        Sectioned-TOML subset + DOH_LOOKUP_* env/flags.
internal/engine/        validate → provider → cache → query-per-type → aggregate.
internal/app/           CLI dispatch; lookup/cache/mcp; --type/--provider/--json/--raw.
internal/mcp/           Zero-dep stdio JSON-RPC 2.0 server + tools (usage.md embedded).
```

## Key design decisions

- **Out-of-band DoH is the distinguishability mechanism.** No fake browser
  User-Agent, no EDNS0 investigation tag. The tool identifies itself with a
  plain `doh-lookup/<version>` UA and, crucially, **every result states the
  resolver + endpoint used** — that provenance is the audit trail (mirrors
  whois-lookup stating its `rdap|whois` source).
- **DO bit is always set, RRSIG is filtered.** Cloudflare only sets the AD flag
  when the DO bit is present (Google sets it regardless), so we always request
  `do=1`. That pulls RRSIG/NSEC proof records into the answer; they are
  filtered from normalized `records` unless the caller asked for that type by
  name. The DNSSEC signal is carried by `authenticated`.
- **`authenticated` = any queried type returned AD.** It means the resolver
  validated the chain; the tool does **not** verify signatures itself
  (that would need wireformat + crypto — a v2 candidate).
- **Validation gate before any network I/O.** Not-IP input must pass RFC
  hostname validation (≤253, labels 1–63 LDH, ≥2 labels, TLD not all-numeric)
  or the request is refused (CLI exit 2, MCP `invalid_input`). This blocks
  request-splitting into the HTTPS query and cache-key pollution.
- **No credentials.** Cloudflare/Google DoH are public.
- **Engine is shared** by CLI and MCP so their behaviour cannot diverge; the
  HTTP transport is an injected `Doer` interface, mocked in tests.
- **Cache honors DNS TTL, floored.** Each entry stores its own expiry
  (min answer TTL, floored to `ttl_floor_seconds`), so short-TTL records
  expire sooner and bulk sweeps can't hammer the resolver. `--raw` bypasses the
  read (raw bodies aren't cached).
- **Default provider Cloudflare** (clear privacy stance; does not forward ECS).
  **ECS suppressed by default** (`edns_client_subnet=0.0.0.0/0`) so the
  investigator's network isn't leaked.

## Gotchas

- **Exit-code contract (lookup):** `0` at least one target resolved / `1` every
  target returned NXDOMAIN / `2` error (invalid input, network). NXDOMAIN is a
  successful answer that the name does not exist, distinct from failure, and is
  still rendered (the Result carries `status: NXDOMAIN`).
- **Provider AD divergence:** never drop the DO bit to reduce noise — it would
  silently make Cloudflare report `authenticated: false` for signed zones.
  Filter RRSIG instead.
- **JSON DoH is not an RFC:** Google's `resolve` is proprietary, Cloudflare's is
  documented; decode leniently and normalize in one place (`internal/doh`).
  `Comment` can be a string or an array across providers.
- **Direct RRSIG/ANY queries** may return SERVFAIL from the resolver — that is a
  real upstream response, surfaced as-is, not a bug.
- **Status: scaffold + tests (Phase 2 of the RFP).** Core CLI + MCP are live
  and network-verified against Cloudflare/Google; the offline suite and the
  live E2E harness both pass. Remaining before release: docs polish and the
  release/sign/notarize + homebrew-tap + submodule/catalog integration.

## Roadmap (post-scaffold)

- v2: RFC 8484 wireformat (`application/dns-message`) for arbitrary resolvers
  (Quad9, self-hosted) and independent DNSSEC signature validation.
- MCP bulk lookup with workspace file-mediation for large result sets
  (abuse-lookup `get_reports` pattern).
