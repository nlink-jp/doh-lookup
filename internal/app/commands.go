package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/engine"
	"github.com/nlink-jp/doh-lookup/internal/query"
)

// runLookup implements the lookup command against injected writers so tests
// can capture output.
func runLookup(args []string, version string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		typeList = fs.String("type", "", "comma-separated record types (default: profile)")
		provider = fs.String("provider", "", "DoH provider: cloudflare or google")
		jsonOut  = fs.Bool("json", false, "JSON output")
		raw      = fs.Bool("raw", false, "include each resolver's raw JSON response")
		cd       = fs.Bool("cd", false, "checking disabled (return records despite DNSSEC failure)")
		noDNSSEC = fs.Bool("no-dnssec", false, "alias for --cd")
		refresh  = fs.Bool("refresh", false, "bypass the answer cache and re-query")
		timeout  = fs.Duration("timeout", 0, "network timeout (e.g. 5s; default 10s)")
		input    = fs.String("input", "", "read newline-separated targets from a file")
		cfgPath  = fs.String("config", "", "config file path")
	)
	fs.BoolVar(jsonOut, "j", false, "JSON output (shorthand)")
	fs.StringVar(cfgPath, "c", "", "config file path (shorthand)")

	positionals, err := parseInterspersed(fs, args)
	if err != nil {
		return exitError
	}

	targets, err := readTargets(positionals, *input, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	if len(targets) == 0 {
		fmt.Fprintln(stderr, "lookup: at least one target (domain or IP) is required")
		return exitError
	}

	cfg, err := config.Load(*cfgPath, *timeout)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return exitError
	}
	e := engine.New(cfg, version)

	opts := engine.Options{
		Types:    splitTypes(*typeList),
		Provider: *provider,
		CD:       *cd || *noDNSSEC,
		Refresh:  *refresh,
		Raw:      *raw,
	}

	var results []*engine.Result
	hadError, notFound, resolved := false, 0, 0
	for _, tgt := range targets {
		res, lerr := e.Lookup(tgt, opts)
		switch {
		case errors.Is(lerr, engine.ErrNotFound):
			notFound++
			results = append(results, res)
		case errors.Is(lerr, query.ErrInvalid):
			fmt.Fprintf(stderr, "%s: %v\n", tgt, lerr)
			hadError = true
		case lerr != nil:
			fmt.Fprintf(stderr, "%s: error: %v\n", tgt, lerr)
			hadError = true
		default:
			resolved++
			results = append(results, res)
		}
	}

	if *jsonOut {
		writeJSON(stdout, results)
	} else {
		writeText(stdout, results)
	}

	switch {
	case hadError:
		return exitError
	case resolved == 0 && notFound > 0:
		return exitNotFound
	default:
		return exitOK
	}
}

// stdin is indirected so tests can substitute a reader.
var stdin io.Reader = os.Stdin

// readTargets assembles the target list from positionals, an optional --input
// file, and (when neither is given) stdin. Blank lines and #-comments are
// skipped; each line may hold whitespace-separated targets.
func readTargets(positionals []string, inputPath string, in io.Reader) ([]string, error) {
	targets := append([]string(nil), positionals...)
	if inputPath != "" {
		f, err := os.Open(inputPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		targets = append(targets, scanTargets(f)...)
	} else if len(targets) == 0 {
		targets = append(targets, scanTargets(in)...)
	}
	return targets, nil
}

func scanTargets(r io.Reader) []string {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, strings.Fields(line)...)
	}
	return out
}

func splitTypes(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseInterspersed parses fs while tolerating flags that appear after
// positional arguments. Validated targets never begin with '-', so there is
// no ambiguity.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positionals = append(positionals, args[0])
		args = args[1:]
	}
	return positionals, nil
}

// writeJSON prints one indented object for a single result, or JSONL (one
// compact object per line) for a bulk run.
func writeJSON(w io.Writer, results []*engine.Result) {
	if len(results) == 1 {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results[0])
		return
	}
	enc := json.NewEncoder(w)
	for _, r := range results {
		_ = enc.Encode(r)
	}
}

// writeText renders each result as a header line plus aligned records.
func writeText(w io.Writer, results []*engine.Result) {
	for i, r := range results {
		if i > 0 {
			fmt.Fprintln(w)
		}
		dnssec := "DNSSEC:none"
		if r.Authenticated {
			dnssec = "DNSSEC:validated"
		}
		cached := ""
		if r.Cached {
			cached = ", cached"
		}
		fmt.Fprintf(w, "%s  [%s, %s, %s%s]  via %s (%s)\n",
			r.Query, r.Kind, r.Status, dnssec, cached, r.Provider, r.Endpoint)
		if len(r.Records) == 0 {
			if r.Status == "NXDOMAIN" {
				fmt.Fprintln(w, "  (name does not exist)")
			} else {
				fmt.Fprintln(w, "  (no records)")
			}
			continue
		}
		for _, rec := range r.Records {
			fmt.Fprintf(w, "  %-6s %-30s %-6d %s\n", rec.Type, rec.Name, rec.TTL, rec.Data)
		}
	}
}
