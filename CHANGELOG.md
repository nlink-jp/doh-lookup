# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Every MCP tool input schema is closed.** The schemas omitted
  `additionalProperties: false`, so a mistyped argument read as a legitimate one
  to any client that validates against them. Schemas are now built through a
  single `obj()` helper that sets the flag, and an arch test fails if a tool's
  schema omits it — org ADR-021 §10 requires the test as well as the flag,
  because a rule stated only in prose is re-decided by whoever adds the next
  tool. The server's own argument decoding is unchanged and still lenient: it
  does not use `DisallowUnknownFields`, so an unknown argument that reaches it
  is ignored rather than refused.

## [0.1.2] - 2026-09-21

### Fixed

- A number in the config file was accepted when it was not one. `NaN` passed the
  range check — it fails every comparison, so "reject what is below the floor"
  lets it through — and `Inf` or `1e300` overflowed the duration it became.
  Ranges are now stated from the inside, with a ceiling.

- `make check` is green again: `make lint` failed on errcheck findings for every
  `fmt.Fprint*` write to the CLI's own stdout/stderr, and on deliberate discards
  that did not say they were deliberate. No behaviour change.
  - `.golangci.yml` excludes only `fmt.Fprint*` from errcheck, so errcheck stays
    meaningful everywhere else (the atomic write checks its `Close`).
  - Every other unchecked return is now `_ =` with the reason beside it.

### Added

- `make verify-release` refuses to publish a darwin zip that carries no
  notarization marker, or one rebuilt after its marker. The vendored Homebrew
  templates and packaging scripts are in sync with the org canonical.

## [0.1.1] - 2026-07-18

### Fixed

- Accept underscore-prefixed DNS labels. The input validation gate applied
  strict hostname (LDH) rules, which rejected legitimate query targets such as
  `_dmarc.example.com`, `selector._domainkey.example.com`, and the
  `_service._proto` labels of SRV/TLSA records. Underscore is a valid DNS
  label octet and is now accepted in any label position; the CRLF/control-
  character injection gate is unchanged.

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

[0.1.1]: https://github.com/nlink-jp/doh-lookup/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/nlink-jp/doh-lookup/releases/tag/v0.1.0
