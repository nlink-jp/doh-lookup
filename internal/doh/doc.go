// Package doh is the DNS-over-HTTPS client: it issues one record-type query
// to a public resolver (Cloudflare or Google) over HTTPS/443 using the JSON
// DoH API and decodes the response leniently into a normalized form. It is
// deliberately the only thing in the tree that talks to a resolver, so the
// "out-of-band, explicitly distinguishable" property is enforced in one
// place: every Response records which provider and endpoint answered. The
// HTTP transport is an injected interface (Doer) so the client is fully
// testable without network access. No credentials — the endpoints are
// public.
package doh
