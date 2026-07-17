// Package engine ties validation, provider resolution, the DoH client, and
// the answer cache into a single Lookup. It is shared by the CLI and the MCP
// server so their behaviour cannot diverge, and it is the only clock reader.
// For a domain it fetches the requested record types (or the configured
// profile) and aggregates them into one Result; for an IP it issues the PTR
// reverse query. Every Result records the resolver and endpoint that answered
// and the DNSSEC AD flag — the provenance that keeps investigative lookups
// explicitly distinguishable. Invalid input never reaches the network; a name
// that does not exist returns ErrNotFound alongside a populated (NXDOMAIN)
// Result so bulk callers can still report it.
package engine
