// Package query validates and canonicalizes a lookup target before any
// network I/O. A target is classified as either a domain (forward lookup) or
// an IP address (reverse / PTR lookup); anything that is neither is rejected
// with ErrInvalid. The validation gate — control-character/CRLF rejection and
// RFC hostname rules — runs before the target can reach the DoH client, so it
// blocks request-splitting into the HTTPS query, wasted rate limits, and
// cache-key pollution. IDN U-labels are converted to punycode A-labels here,
// so later stages always see the wire form.
package query
