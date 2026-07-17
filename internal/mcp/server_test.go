package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nlink-jp/doh-lookup/internal/cache"
	"github.com/nlink-jp/doh-lookup/internal/config"
	"github.com/nlink-jp/doh-lookup/internal/doh"
	"github.com/nlink-jp/doh-lookup/internal/engine"
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
	in := strings.NewReader(strings.Join(reqs, "\n"))
	var out bytes.Buffer
	if err := Serve(e, "test", in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resps []map[string]any
	dec := json.NewDecoder(&out)
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

func TestNotificationSkipped(t *testing.T) {
	// A notification (no id) must produce no response.
	resps := drive(t, testServerEngine(t),
		`{"jsonrpc":"2.0","method":"initialized"}`,
	)
	if len(resps) != 0 {
		t.Errorf("notification produced %d responses, want 0", len(resps))
	}
}
