# doh-lookup

Collect a domain's DNS records over **DoH (DNS over HTTPS)** from a public
resolver (**Cloudflare** or **Google**) — as a CLI and a local MCP server.

`dig` sends its queries to the OS/configured resolver over UDP/53, where an
investigative lookup blends into ordinary traffic and pollutes the
organization's resolver cache and logs. **doh-lookup** instead queries a public
resolver **out-of-band over HTTPS/443**, and every result states **which
resolver and endpoint answered** plus the **DNSSEC AD flag** — so an
investigation stays explicitly distinguishable from normal DNS, with an audit
trail, and never touches your DNS infrastructure.

The DNS-resolution sibling of [`asn-lookup`](https://github.com/nlink-jp/asn-lookup)
(attribution), [`abuse-lookup`](https://github.com/nlink-jp/abuse-lookup)
(reputation), [`whois-lookup`](https://github.com/nlink-jp/whois-lookup)
(registration), and [`tor-exit-lookup`](https://github.com/nlink-jp/tor-exit-lookup)
(Tor exit membership). **Zero credentials, zero external dependencies.**

## Install

```bash
# Homebrew (nlink-jp tap; prebuilt, Developer ID signed + notarized, arm64 macOS)
brew install nlink-jp/tap/doh-lookup

# From source (Go 1.25+)
make build      # → dist/doh-lookup
```

## Usage

```bash
# Forward lookup — the default profile (A/AAAA/MX/TXT/NS/SOA/CAA) in one shot
doh-lookup lookup example.com

# Narrow to specific record types
doh-lookup lookup --type A,MX example.com

# Underscore-prefixed service labels resolve too (DMARC / DKIM / SRV / TLSA)
doh-lookup lookup --type TXT _dmarc.example.com
doh-lookup lookup --type SRV _sip._tcp.example.com

# Reverse (PTR) — pass an IP
doh-lookup lookup 8.8.8.8

# Choose the resolver (default: cloudflare)
doh-lookup lookup --provider google example.com

# JSON output (JSONL when multiple targets); bulk via args, --input, or stdin
doh-lookup lookup --json example.com
printf 'example.com\ncloudflare.com\n' | doh-lookup lookup --type A --json
doh-lookup lookup --input targets.txt

# Include the resolver's raw JSON, or bypass the cache
doh-lookup lookup --raw example.com
doh-lookup lookup --refresh example.com

# Cache management
doh-lookup cache status
doh-lookup cache clear
```

Example:

```
$ doh-lookup lookup example.com
example.com  [forward, NOERROR, DNSSEC:validated]  via cloudflare (https://cloudflare-dns.com/dns-query)
  A      example.com                    201    104.20.23.154
  AAAA   example.com                    43     2606:4700:10::6814:179a
  MX     example.com                    300    0 .
  TXT    example.com                    300    "v=spf1 -all"
  NS     example.com                    86400  hera.ns.cloudflare.com.
  SOA    example.com                    1800   elliott.ns.cloudflare.com. dns.cloudflare.com. ...
```

### Exit codes (`lookup`)

| code | meaning |
|------|---------|
| `0`  | at least one target resolved |
| `1`  | every target returned NXDOMAIN (does not exist) |
| `2`  | error (invalid input, network failure, …) |

## MCP server

```bash
doh-lookup mcp   # stdio JSON-RPC 2.0
```

Tools: `lookup` (domain or IP), `cache_status`, `get_usage`. Call `get_usage`
first for the full reference and error-recovery table. Errors are structured
JSON (`{code, message}`). Example registration (Claude Code):

```json
{
  "mcpServers": {
    "doh-lookup": { "command": "doh-lookup", "args": ["mcp"] }
  }
}
```

## Configuration

Optional. Copy [`config.example.toml`](config.example.toml) to
`~/.config/doh-lookup/config.toml`. Precedence is **flag > env var > file >
default**. No credentials.

| Setting | TOML | Env | Default |
|---------|------|-----|---------|
| Provider | `[provider] default` | `DOH_LOOKUP_PROVIDER` | `cloudflare` |
| Cloudflare endpoint | `[provider] cloudflare_url` | `DOH_LOOKUP_CLOUDFLARE_URL` | `https://cloudflare-dns.com/dns-query` |
| Google endpoint | `[provider] google_url` | `DOH_LOOKUP_GOOGLE_URL` | `https://dns.google/resolve` |
| Default profile | `[query] profile` | `DOH_LOOKUP_PROFILE` | `A,AAAA,MX,TXT,NS,SOA,CAA` |
| Suppress ECS | `[query] suppress_ecs` | `DOH_LOOKUP_SUPPRESS_ECS` | `true` |
| Cache TTL floor | `[cache] ttl_floor_seconds` | `DOH_LOOKUP_CACHE_TTL_FLOOR_SECONDS` | `60` |
| Cache dir | `[cache] dir` | `DOH_LOOKUP_CACHE_DIR` | `~/.cache/doh-lookup` |
| Network timeout | `[network] timeout_seconds` | `DOH_LOOKUP_TIMEOUT_SECONDS` | `10` |

## How it stays distinguishable

- **Out-of-band, over HTTPS.** Queries go to `1.1.1.1` / `8.8.8.8` over 443, not
  to your local resolver — separable at the network layer from browsing DNS.
- **Provenance in every result.** The resolver name and endpoint URL are always
  reported, so an investigation is auditable rather than anonymous.
- **DNSSEC AD reported.** The `authenticated` flag reflects whether the resolver
  validated the answer chain (the tool requests the DO bit so the flag is
  meaningful; it does not verify signatures itself).
- **ECS suppressed by default.** The resolver is asked not to forward an EDNS
  Client Subnet, so your network is not leaked to authoritative servers.

## Notes

- **DNSSEC:** `authenticated: true` means the resolver validated the chain, not
  that doh-lookup verified signatures. Independent validation (RFC 8484
  wireformat + crypto) is a v2 candidate.
- **Rate limits:** public DoH has soft per-IP limits. Answers are cached
  honoring DNS TTLs (floored), and bulk runs are paced politely — no aggressive
  retries.

## Development

```bash
make build      # → dist/doh-lookup  (never `go build` directly)
make test       # offline unit/integration tests (mocked HTTP)
make e2e        # live end-to-end vs real Cloudflare/Google DoH (network required)
make check      # lint + test + build-all
```

Go 1.25+, standard library only. See [AGENTS.md](AGENTS.md) for architecture.

## License

MIT — see [LICENSE](LICENSE).
