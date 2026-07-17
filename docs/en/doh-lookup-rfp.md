# RFP: doh-lookup

> Generated: 2026-07-17
> Status: Draft

## 1. Problem Statement

Ordinary DNS lookups (`dig` and friends) are sent to the OS/configured resolver over
UDP/53. When a CTI/IR practitioner investigates a suspicious domain this way, the
investigative query (a) blends into normal application DNS traffic and becomes noise in
the SOC's DNS logs, and (b) pollutes the organization's resolver cache/logs so that it
looks like "someone actually visited the suspicious domain." **doh-lookup is a CLI plus
local MCP server that queries Google / Cloudflare DoH (DNS over HTTPS) endpoints over
HTTPS/443, cleanly separating investigative queries from the organization's DNS
infrastructure so that DNS information is collected in a state that is explicitly
distinguishable as an intentional, out-of-band investigation.** The target users are
CTI/IR practitioners who want to inspect the DNS attributes of a suspicious domain with
sound OpSec. It is the **DNS-resolution** sibling of `asn-lookup` (attribution),
`abuse-lookup` (reputation), `tor-exit-lookup` (Tor exit membership), `whois-lookup`
(registration data), and `icloud-relay-lookup` (Private Relay egress) in the
cybersecurity-series.

## 2. Functional Specification

### Commands / API Surface

**CLI subcommands** (following the sibling-tool conventions):

- `doh-lookup lookup <target...>` — primary operation. `<target>` is a domain or IP
  (multiple allowed)
  - `--type A,AAAA,MX,...` — record types (comma-separated). Defaults to the domain
    profile bundle when omitted
  - `--provider cloudflare|google` — DoH provider (default cloudflare)
  - `--json` — structured output (JSONL for bulk: one object per line per target)
  - `--raw` — emit the resolver's raw JSON response as-is
  - `--no-dnssec` / `--cd` — checking disabled (return records even when DNSSEC
    validation fails)
  - stdin / `--input <file>` — newline-delimited target list for bulk input
- `doh-lookup cache <status|clear>` — show / clear the cache
- `doh-lookup mcp` — start the local MCP server (stdio)
- `doh-lookup --version`

**MCP tools** (identical shape to whois-lookup):

- `lookup` — `{ name, types?, provider? }` → normalized records + meta
  (resolver / endpoint / AD / RCODE)
- `cache_status` — cache statistics
- `get_usage` — tool reference and error-recovery table

### Input / Output

- **Input classification**: domain → forward lookup (profile bundle or `--type`),
  IP → automatic PTR reverse lookup. Non-IP input that fails the RFC hostname validation
  gate (≤253 total / labels 1–63 LDH / dot required / control chars & CRLF rejected) is
  refused (CLI exit 2, MCP `{code:"invalid_input"}`).
- **Default records (profile bundle)**: `A / AAAA / MX / TXT / NS / SOA / CAA` in one shot.
- **Output meta (the core of the distinguishability design)**: every result states the
  **resolver name / endpoint URL / DNSSEC AD flag / RCODE / response time**, making it
  auditable which out-of-band resolver the query was intentionally sent to.
- **Normalization**: CNAME chains, wildcards, and the NXDOMAIN (name does not exist) vs
  NODATA (NOERROR with empty answer) distinction are normalized in one place (engine).
- **Output formats**: human-readable (default; per-target sections for bulk) /
  `--json` (JSONL for bulk) / `--raw`.
- **Exit-code contract (lookup)**: `0` success / `1` NXDOMAIN (a successful answer that
  the name does not exist) / `2` error.

### Configuration

`~/.config/doh-lookup/config.toml` (sectioned TOML, optional). `DOH_LOOKUP_*` environment
variables override. Precedence: **flag > env > config > default**.

```toml
[provider]
# default = "cloudflare"          # cloudflare | google
# cloudflare_url = "https://cloudflare-dns.com/dns-query"
# google_url = "https://dns.google/resolve"

[query]
# profile = ["A","AAAA","MX","TXT","NS","SOA","CAA"]  # bundle when --type omitted
# suppress_ecs = true             # edns_client_subnet=0.0.0.0/0 to not leak the investigator's origin

[cache]
# ttl_floor_seconds = 60          # respect DNS TTL, with a lower floor
# dir = "~/.cache/doh-lookup"

[network]
# timeout_seconds = 10
```

### External Dependencies

- **None** (Go standard library only). DoH is handled entirely with `net/http` +
  `encoding/json`. `miekg/dns` is not used.
- The only external services are the public Google / Cloudflare DoH endpoints.
  **No credentials, no API key.**

## 3. Design Decisions

- **Language = Go, zero external dependencies.** Series standard (same as
  asn/abuse/tor/whois/icloud-relay). The DoH JSON API is tractable with the standard
  library alone and ships as a single signed binary.
- **The DoH-to-public-resolver design itself is the core of distinguishability.** No extra
  User-Agent marker or EDNS0 investigation tag is added. Instead the **output always states
  the resolver/endpoint used**, guaranteeing auditability (the same philosophy as
  whois-lookup stating its `rdap|whois` source in the output).
- **The engine is shared by CLI and MCP** so their behavior cannot diverge. The HTTP client
  is an injected interface, mocked in tests (design-for-testability).
- **A validation gate before any network I/O** is mandatory, blocking CRLF/control-char
  injection, wasted rate limits, and cache-key pollution.
- **ECS suppression by default** (`edns_client_subnet=0.0.0.0/0`), an OpSec default that
  avoids leaking the investigator's network to the resolver.
- **Relationship to siblings**: the DNS-resolution counterpart to `asn-lookup` (IP→AS),
  `abuse-lookup` (IP reputation), `whois-lookup` (registration), and
  `tor-exit-lookup` / `icloud-relay-lookup` (egress IP classification). Enrichment of
  returned IPs is delegated to those siblings (UNIX philosophy).
- **Out of scope (intentional)**:
  - IP enrichment (AS / reputation / geo) — delegated to siblings
  - RFC 8484 wireformat (`application/dns-message`) — v2 candidate
  - Independent DNSSEC signature validation — AD flag reporting only; independent
    validation needs wireformat + crypto and is a v2 candidate
  - Zone transfer / direct authoritative queries / passive-DNS history

## 4. Development Plan

### Phase 1: Core (CLI) — independently reviewable

- `internal/query`: input classification (domain / IP) + RFC hostname validation gate,
  IDN punycode (port the RFC 3492 implementation from whois-lookup)
- `internal/config`: sectioned TOML + `DOH_LOOKUP_*` env + flags (precedence applied)
- `internal/cache`: DNS-TTL-respecting cache (configurable min-TTL floor), atomic writes
- `internal/doh`: DoH client (Cloudflare / Google JSON API, injected HTTP interface,
  ECS suppression, `do=1` to obtain AD)
- `internal/engine`: validate → cache → resolve → normalize (CNAME chains,
  NXDOMAIN vs NODATA, RCODE handling)
- `internal/app`: `lookup` / `cache` subcommands, `--type`/`--json`/`--raw`/`--provider`,
  profile-bundle default, PTR reverse lookup, multiple targets + stdin, output meta,
  exit-code contract
- Table-driven test suite with mocked HTTP

### Phase 2: Features (MCP) — independently reviewable

- `internal/mcp`: zero-dep stdio JSON-RPC 2.0, tools `lookup` / `cache_status` /
  `get_usage`, structured errors `{code, message, details}`
- Large bulk results are file-mediated via a workspace (the abuse-lookup `get_reports` /
  asn-lookup `prefixes_file` pattern)

### Phase 3: Release

- README.md / README.ja.md / CHANGELOG / AGENTS.md / config.example.toml / docs/{en,ja}
- Makefile + scripts (codesign / notarize / brew), build-all (linux amd64/arm64,
  darwin arm64, windows amd64), darwin signing + notarization, homebrew-tap formula
- submodule integration → org profile + web-site catalog sync → check-org.sh

## 5. Required API Scopes / Permissions

**None.** All DoH endpoints are public. No authentication, API key, OAuth scope, or IAM
role of any kind is required.

## 6. Series Placement

Series: **cybersecurity-series**
Reason: it is a CTI/IR support tool that collects the DNS attributes of a suspicious domain
in an OpSec-separated state, belonging to the same "CLI + MCP, zero-credential,
zero-dependency investigative lookup" family as `asn-lookup` / `abuse-lookup` /
`tor-exit-lookup` / `whois-lookup` / `icloud-relay-lookup`.

## 7. External Platform Constraints

- **DoH JSON API schema drift**: Google `dns.google/resolve` is a proprietary interface and
  Cloudflare's is documented, but neither is an RFC. Decode responses leniently and
  normalize in one place (`internal/doh`), mirroring whois-lookup's RDAP-dialect handling.
- **Soft rate limits**: public DoH has no explicit daily quota like AbuseIPDB, but abusive
  volume is throttled / blocked. Respect the TTL cache, pace bulk politely, and never retry
  aggressively.
- **Content type**: Cloudflare requires `Accept: application/dns-json`. Google returns JSON
  from `dns.google/resolve`.
- **DNSSEC**: the AD flag is meaningful only when the resolver validates (Google /
  Cloudflare do). This tool reports AD; it does not independently validate signatures.
- **Response size**: DoH over HTTPS has no 512-byte UDP limit, so large TXT / many-answer
  responses are handled fine.

---

## Discussion Log

- **Origin**: proposal for an abuse-lookup / tor-exit-lookup sibling that collects domain
  information via Google / Cloudflare DoH. Because `dig` is indistinguishable from normal
  DNS, the primary goal is to query in an **explicitly distinguishable** state.
- **Confirmed and agreed on the interpretation of distinguishability**: `dig` (OS resolver
  over UDP/53) buries investigative queries in normal DNS and pollutes the org resolver's
  cache/logs. DoH to a public resolver over HTTPS/443 separates them as an intentional,
  out-of-band investigation — this OpSec separation is the primary goal.
- **Level of distinguishability**: chose "separation via the DoH design." No extra
  User-Agent marker / EDNS0 tag; instead the output states the resolver/endpoint used to
  make it auditable.
- **Scope**: chose "focus on pure DoH lookup." Enrichment of returned IPs is delegated to
  siblings such as asn/abuse (UNIX philosophy).
- **Verified the implementation conventions against real code**: adopted whois-lookup (the
  closest sibling — domain-oriented, zero-credential) as the template. Confirmed the shared
  conventions: `main.go` + `internal/{query,engine,cache,config,app,mcp}` layering; CLI
  subcommands `lookup`/`cache`/`mcp` + `--type`/`--json`/`--raw`; MCP tools
  `lookup`/`cache_status`/`get_usage`; `~/.config/<tool>/config.toml` + `<TOOL>_*` env;
  validation gate; exit codes 0/1/2; output stating its source.
- **Functional-spec decisions (4)**:
  - Default records = **domain profile bundle** (A/AAAA/MX/TXT/NS/SOA/CAA, narrow with
    `--type`)
  - **PTR reverse lookup supported** (IP → automatic PTR)
  - **Bulk included in v1** (multiple positional args + stdin/`--input`, pipe-friendly)
  - Default provider = **Cloudflare (1.1.1.1)** (clear privacy stance; switch to Google via
    `--provider`/config)
- **Development Plan**: Phase 1 Core CLI → Phase 2 MCP → Phase 3 Release. Phases 1 and 2
  are independently reviewable.
- **Supplementary design decisions**: ECS suppression (`edns_client_subnet=0.0.0.0/0`) by
  default so the investigator's network is not leaked. DNSSEC is AD-flag reporting only
  (signature validation to be considered when wireformat support lands in v2).
