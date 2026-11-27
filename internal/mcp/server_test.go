package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/doh-lookup/internal/cache"
	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/doh"
	"github.com/nlink-jp/doh-lookup/internal/engine"
	"github.com/nlink-jp/mcp/client"
)

type fakeClient struct{}

func (fakeClient) Query(p doh.Provider, name, rrType string, cd bool) (*doh.Response, error) {
	return &doh.Response{
		Provider: p.Name, Endpoint: p.Endpoint, Name: name, Type: rrType,
		Status: 0, StatusText: "NOERROR", Authenticated: true,
		Answers: []doh.Answer{{Name: name, Type: rrType, TTL: 300, Data: "93.184.216.34"}},
	}, nil
}

func testServerEngine(t *testing.T) *engine.Engine {
	t.Helper()
	cfg := &config.Config{Provider: "cloudflare", Profile: []string{"A"}, CacheTTLFloor: time.Minute, CacheDir: t.TempDir()}
	return &engine.Engine{
		Cfg:    cfg,
		Cache:  &cache.Store{Dir: cfg.CacheDir},
		Client: fakeClient{},
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

// drive feeds newline-delimited JSON-RPC requests through Serve and returns
// the decoded responses.
func drive(t *testing.T, e *engine.Engine, reqs ...string) []map[string]any {
	t.Helper()
	out := serveOverClient(t, func(ctx context.Context, in io.Reader, w io.Writer) error {
		return Serve(ctx, e, "test", in, w)
	}, strings.Join(reqs, "\n"))
	var resps []map[string]any
	dec := json.NewDecoder(out)
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		resps = append(resps, m)
	}
	return resps
}

func TestInitializeAndToolsList(t *testing.T) {
	resps := drive(t, testServerEngine(t),
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2", len(resps))
	}
	init := resps[0]["result"].(map[string]any)
	if init["serverInfo"].(map[string]any)["name"] != "doh-lookup" {
		t.Errorf("serverInfo name wrong: %v", init["serverInfo"])
	}
	tools := resps[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 3 {
		t.Errorf("got %d tools, want 3 (lookup, cache_status, get_usage)", len(tools))
	}
}

func TestToolsCallLookup(t *testing.T) {
	resps := drive(t, testServerEngine(t),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup","arguments":{"query":"example.com","types":["A"]}}}`,
	)
	res := resps[0]["result"].(map[string]any)
	content := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content, `"provider": "cloudflare"`) {
		t.Errorf("lookup result missing provenance: %s", content)
	}
	if !strings.Contains(content, `"status": "NOERROR"`) {
		t.Errorf("lookup result missing status: %s", content)
	}
}

func TestToolsCallInvalidInput(t *testing.T) {
	resps := drive(t, testServerEngine(t),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup","arguments":{"query":"bad host"}}}`,
	)
	res := resps[0]["result"].(map[string]any)
	if res["isError"] != true {
		t.Errorf("expected isError=true, got %v", res)
	}
	content := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content, "invalid_input") {
		t.Errorf("expected invalid_input code, got %s", content)
	}
}

// ADR-030 §7: the server against the three probe clients (legacy only,
// dual-era, modern only).
func TestAgainstTheProbeClients(t *testing.T) {
	e := testServerEngine(t)
	probeMatrix(t, func(ctx context.Context, in io.Reader, w io.Writer) error {
		return Serve(ctx, e, "test", in, w)
	})
}

// The client that opens the server receives the instructions — the hint that
// points it at get_usage — whichever era it opens in.
func TestOpeningCarriesTheInstructions(t *testing.T) {
	e := testServerEngine(t)
	cl := openWith(t, func(ctx context.Context, in io.Reader, w io.Writer) error {
		return Serve(ctx, e, "test", in, w)
	}, client.Options{})
	if got := cl.Instructions(); got != Instructions {
		t.Errorf("instructions = %q, want the package's Instructions", got)
	}
}
