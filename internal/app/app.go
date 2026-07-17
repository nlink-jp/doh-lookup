// Package app implements the doh-lookup command-line interface: subcommand
// dispatch plus the lookup / cache / mcp commands. Core logic lives in the
// query, doh, cache, config, and engine packages; this package is the thin
// I/O shell around them.
package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/doh"
	"github.com/nlink-jp/doh-lookup/internal/engine"
	"github.com/nlink-jp/doh-lookup/internal/mcp"
)

// Exit codes. lookup is not a membership test: a not-found is a successful
// answer that the name does not exist, distinct from an operational failure.
const (
	exitOK       = 0 // at least one target resolved
	exitNotFound = 1 // every target returned NXDOMAIN
	exitError    = 2 // usage / validation / network error
)

// Run dispatches a subcommand and returns a process exit code.
func Run(args []string, version string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return exitError
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "lookup":
		return runLookup(rest, version, os.Stdout, os.Stderr)
	case "cache":
		return runCache(rest, os.Stdout, os.Stderr)
	case "mcp":
		return cmdMCP(rest, version)
	case "version", "--version", "-v":
		fmt.Println("doh-lookup " + version)
		fmt.Println("Resolvers: Cloudflare (cloudflare-dns.com) / Google (dns.google) — JSON DoH, no credentials.")
		return exitOK
	case "help", "-h", "--help":
		usage(os.Stdout)
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage(os.Stderr)
		return exitError
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `doh-lookup — collect a domain's DNS records over DoH (out-of-band, distinguishable)

Usage:
  doh-lookup <command> [flags] [target...]

Commands:
  lookup <domain|ip ...>   Look up DNS records over DoH (input type auto-detected)
  cache status             Show the answer-cache state
  cache clear              Clear the answer cache
  mcp                      Run as a local MCP server (stdio)
  version                  Print the version

lookup flags:
  --type <list>            Comma-separated record types (default: the profile
                           %s). Ignored for IPs (always PTR).
  --provider <name>        cloudflare (default) or google
  -j, --json               JSON output (JSONL when multiple targets)
  --raw                    Include each resolver's raw JSON response
  --cd, --no-dnssec        Checking disabled: return records even if DNSSEC
                           validation fails
  --refresh                Bypass the answer cache and re-query
  --timeout <dur>          Network timeout (e.g. 5s; default 10s)
  --input <file>           Read newline-separated targets from a file
  -c, --config <path>      Config file (default ~/.config/doh-lookup/config.toml)

Bulk input: pass multiple targets, --input <file>, or pipe them on stdin.

lookup exit codes:
  0  at least one target resolved
  1  every target returned NXDOMAIN (does not exist)
  2  error (invalid input, network failure, ...)

An IP target is reverse-resolved (PTR); a domain is forward-resolved. Input
that is neither a valid IP nor a valid domain is rejected before any network
I/O. Every result states which resolver and endpoint answered and the DNSSEC
AD flag — that provenance is what makes the query distinguishable from dig's
UDP/53 traffic to the OS resolver. All endpoints are public; no credentials.
`, strings.Join(doh.DefaultProfile, "/"))
}

// cmdMCP runs the stdio MCP server until stdin closes (MCP has no protocol
// cancel; a closing stdin is the shutdown signal).
func cmdMCP(args []string, version string) int {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	cfgPath := fs.String("config", "", "config file path")
	fs.StringVar(cfgPath, "c", "", "config file path (shorthand)")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	cfg, err := config.Load(*cfgPath, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return exitError
	}
	if err := mcp.Serve(engine.New(cfg, version), version, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "mcp: %v\n", err)
		return exitError
	}
	return exitOK
}
