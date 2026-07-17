# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-07-17

### Added

- Initial release.
- CLI + local MCP server that collects a domain's DNS records over DoH from a
  public resolver (Cloudflare / Google, JSON DoH API). Every query goes
  out-of-band over HTTPS/443 and each result states which resolver and endpoint
  answered plus the DNSSEC AD flag — so investigative lookups stay explicitly
  distinguishable from ordinary DNS, with an audit trail, and never touch the
  organization's DNS infrastructure.
- `lookup` command: forward lookup for domains (default profile
  `A/AAAA/MX/TXT/NS/SOA/CAA`, or an explicit `--type` list) and PTR reverse
  lookup for IPs; bulk targets via arguments, `--input`, or stdin; `--json`
  (JSON Lines for bulk), `--raw`, `--provider`, `--cd`/`--no-dnssec`,
  `--refresh`, `--timeout`. Exit codes: `0` at least one target resolved,
  `1` every target NXDOMAIN, `2` error.
- Always sets the DNSSEC DO bit so the AD flag is meaningful (Cloudflare only
  sets AD when DO is present); the resulting RRSIG/NSEC proof records are
  filtered from normalized output unless requested by name.
- `cache` command (`status` / `clear`); per-answer cache honoring each record's
  DNS TTL with a configurable floor.
- `mcp` — local stdio MCP server (JSON-RPC 2.0, standard library only) exposing
  `lookup`, `cache_status`, and `get_usage`. `get_usage` returns an embedded
  operating manual, advertised via the initialize `instructions` field.
- Input validation gate (RFC hostname rules, control-char/CRLF rejection)
  before any network I/O; in-house RFC 3492 punycode for IDN.
- EDNS Client Subnet suppressed by default so the investigator's network is not
  leaked. Sectioned-TOML config + `DOH_LOOKUP_*` environment overrides.
  Zero credentials, zero external dependencies.

[0.1.0]: https://github.com/nlink-jp/doh-lookup/releases/tag/v0.1.0
