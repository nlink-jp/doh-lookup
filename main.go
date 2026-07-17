// Command doh-lookup collects a domain's DNS records over DoH (DNS over
// HTTPS) against a public resolver (Cloudflare or Google), as a CLI and a
// local MCP server. Unlike dig — which queries the OS resolver over UDP/53
// and is indistinguishable from ordinary traffic — every doh-lookup query
// goes out-of-band over HTTPS/443 and the result states which resolver and
// endpoint answered, so investigative lookups stay explicitly distinguishable
// from an organization's normal DNS. The DNS-resolution, credential-zero
// sibling of asn-lookup (attribution), abuse-lookup (reputation),
// whois-lookup (registration), and tor-exit-lookup (Tor exit membership).
package main

import (
	"os"

	"github.com/nlink-jp/doh-lookup/internal/app"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(app.Run(os.Args[1:], version))
}
