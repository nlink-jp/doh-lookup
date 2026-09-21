# doh-lookup MCP server

Collects a domain's DNS records over **DoH (DNS over HTTPS)** from a public
resolver (Cloudflare or Google). Unlike `dig` — which queries the OS resolver
over UDP/53 and blends into ordinary traffic — every query goes out-of-band
over HTTPS/443 and each result records **which resolver and endpoint answered**
plus the **DNSSEC AD flag**, so investigative lookups stay explicitly
distinguishable. No credentials.

## Tools

### `lookup`
Look up DNS records for one target.

Arguments:
- `query` (string, required) — a domain name (IDN ok) or an IP address.
- `types` (string[], optional) — record types, e.g. `["A","MX"]`. Ignored for
  IPs (always `PTR`). Default: the configured profile
  (`A/AAAA/MX/TXT/NS/SOA/CAA`).
- `provider` (string, optional) — `cloudflare` (default) or `google`.
- `cd` (boolean, optional) — checking disabled: return records even if DNSSEC
  validation fails.
- `refresh` (boolean, optional) — bypass the local cache and re-query.
- `raw` (boolean, optional) — include each resolver's raw JSON response.

A **domain** is forward-resolved; an **IP** is reverse-resolved (PTR). Input
that is neither a valid domain nor a valid IP is rejected before any network
I/O (`invalid_input`).

### `cache_status`
Report the local answer-cache state: entry count, TTL floor, default provider,
and profile.

### `get_usage`
Return this manual.

## Result schema (`lookup`)

```json
{
  "query": "example.com",
  "query_ascii": "example.com",
  "kind": "forward",
  "provider": "cloudflare",
  "endpoint": "https://cloudflare-dns.com/dns-query",
  "types": ["A", "AAAA", "MX", "TXT", "NS", "SOA", "CAA"],
  "status": "NOERROR",
  "authenticated": false,
  "records": [
    {"type": "A", "name": "example.com", "ttl": 300, "data": "93.184.216.34"}
  ],
  "queried_at": "2026-07-17T00:00:00Z"
}
```

- `kind` — `forward` (domain) or `reverse` (IP → PTR).
- `provider` / `endpoint` — the out-of-band resolver that answered (the
  provenance that makes the query distinguishable).
- `status` — aggregate DNS response code: `NOERROR` if any type answered,
  `NXDOMAIN` if every type says the name does not exist, otherwise the first
  soft failure (`SERVFAIL`, `REFUSED`, ...).
- `authenticated` — true if any queried type came back with the DNSSEC **AD**
  (authenticated data) flag set. It signals the answer chain was
  DNSSEC-validated by the resolver; it does **not** mean doh-lookup verified
  signatures itself.
- `records` — flattened resource records across all queried types.
- `raw` — present only with `raw: true`; the per-type raw resolver JSON.

A `NXDOMAIN` result is returned as a normal (non-error) result with
`status: "NXDOMAIN"` — the name not existing is itself the answer.

## Arguments are strict

Every tool refuses an argument it does not declare, naming it:
`arguments: json: unknown field "type"` (the field is `types`). A wrong-typed
argument is refused the same way. Nothing runs before the arguments decode, so
a rejected call sends no DNS query — fix the name or the type and call again.
This is the enforcing half of the closed schemas (org ADR-021 §4); a misspelt
`types` used to be dropped, which returned the configured profile as if it were
the record types requested.

## Error recovery

Errors are structured JSON: `{"code": "...", "message": "..."}`.

| code            | meaning                                   | recovery                                                        |
|-----------------|-------------------------------------------|----------------------------------------------------------------|
| `invalid_input` | not a valid domain or IP (or empty query) | fix the target; do not retry unchanged                         |
| `invalid_input` + `arguments: json: unknown field "…"` | an argument name this tool does not declare — usually a typo | fix the spelling and call again; the named field is the offending one |
| `invalid_input` + `arguments: json: cannot unmarshal …` | an argument of the wrong JSON type (`types` is an array, `cd`/`refresh`/`raw` booleans) | check the argument's type in the tool list above and call again |
| `network_error` | DoH request failed / HTTP error           | retry after a short delay; try the other `provider`; a soft rate-limit resolves on its own |

## Notes

- **No credentials.** Cloudflare and Google DoH are public.
- **Caching** honors each record's DNS TTL, floored to a configured minimum, so
  repeated lookups do not hammer the resolver. Use `refresh` to force a
  re-query.
- **OpSec.** By default the client asks the resolver not to forward an EDNS
  Client Subnet, so your network is not leaked to authoritative servers.
