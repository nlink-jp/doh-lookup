package mcp

import (
	"strings"
	"testing"
)

func TestToolGetUsage(t *testing.T) {
	resps := drive(t, testServerEngine(t),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_usage","arguments":{}}}`,
	)
	txt := resps[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(txt, "doh-lookup MCP server") {
		t.Errorf("get_usage did not return the manual:\n%s", txt[:min(80, len(txt))])
	}
}

func TestToolCacheStatus(t *testing.T) {
	resps := drive(t, testServerEngine(t),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"cache_status","arguments":{}}}`,
	)
	txt := resps[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	for _, want := range []string{"cache_dir", "entries", "provider"} {
		if !strings.Contains(txt, want) {
			t.Errorf("cache_status missing %q:\n%s", want, txt)
		}
	}
}

func TestUnknownTool(t *testing.T) {
	resps := drive(t, testServerEngine(t),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
	)
	if resps[0]["error"] == nil {
		t.Errorf("unknown tool should be an error response: %v", resps[0])
	}
}
