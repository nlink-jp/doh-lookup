package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// toolError extracts the text of a tools/call result and whether it was an
// error result.
func toolError(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	res, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("response carries no result: %v", resp)
	}
	content, ok := res["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("result carries no content: %v", res)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	return text, res["isError"] == true
}

// structured decodes a {code, message} tool error, which is the shape this
// server's errorResult produces.
func structured(t *testing.T, text string) (string, string) {
	t.Helper()
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(text), &e); err != nil {
		t.Fatalf("tool error is not structured JSON: %v (%s)", err, text)
	}
	return e.Code, e.Message
}

// TestUnknownArgumentIsRefusedByName is the enforcing half of org ADR-021 §4:
// additionalProperties:false only tells a client what is allowed, and a client
// that does not check the schema sends the typo anyway. Every tool must refuse
// it, and the message must name the offending field — a caller that is told
// only "invalid arguments" has to re-read the schema to find its own typo.
//
// The misspellings below are the ones that would otherwise be dangerous: drop
// `types` and the configured profile's records come back as if they were the
// ones requested; drop `provider` and the query goes to the other resolver
// than the one named, which is exactly what an out-of-band lookup is chosen
// to control.
func TestUnknownArgumentIsRefusedByName(t *testing.T) {
	cases := []struct {
		tool  string
		args  string
		field string
	}{
		{"lookup", `{"query":"example.com","type":["A"]}`, "type"},
		{"lookup", `{"query":"example.com","providers":"google"}`, "providers"},
		{"lookup", `{"query":"example.com","no_cache":true}`, "no_cache"},
		{"cache_status", `{"verbose":true}`, "verbose"},
		{"get_usage", `{"topic":"dnssec"}`, "topic"},
	}
	for _, tc := range cases {
		t.Run(tc.tool+"/"+tc.field, func(t *testing.T) {
			req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tc.tool, tc.args)
			text, isErr := toolError(t, drive(t, testServerEngine(t), req)[0])
			if !isErr {
				t.Fatalf("%s accepted unknown argument %q: %s", tc.tool, tc.field, text)
			}
			code, msg := structured(t, text)
			if code != "invalid_input" {
				t.Errorf("%s: error code = %q, want invalid_input", tc.tool, code)
			}
			// Matching the decoder's own phrasing, not just the field name:
			// "provide 'hash…'" happens to contain "hash", so a bare substring
			// test passes for the wrong reason. The mutation check caught it.
			want := `unknown field "` + tc.field + `"`
			if !strings.Contains(msg, want) {
				t.Errorf("%s: error does not name the offending argument: want %s, got %s", tc.tool, want, msg)
			}
		})
	}
}

// TestMalformedArgumentsAreRefused covers the other half of the discarded
// error: `_ = json.Unmarshal` left `a` at its zero value when the object did
// not decode, so a wrong-typed argument produced the same call as an absent
// one — and "provide 'query'" is a misleading answer to a request that did
// provide it.
func TestMalformedArgumentsAreRefused(t *testing.T) {
	cases := []struct {
		name string
		args string
	}{
		{"number for string", `{"query":1}`},
		{"string for array", `{"query":"example.com","types":"A"}`},
		{"array for object", `["example.com"]`},
		{"string for boolean", `{"query":"example.com","cd":"yes"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"lookup","arguments":%s}}`, tc.args)
			text, isErr := toolError(t, drive(t, testServerEngine(t), req)[0])
			if !isErr {
				t.Fatalf("lookup accepted malformed arguments: %s", text)
			}
			if strings.Contains(text, "provide 'query'") {
				t.Errorf("lookup reported the argument as missing instead of malformed: %s", text)
			}
			if !strings.Contains(text, "arguments:") {
				t.Errorf("error is not a decode error: %s", text)
			}
		})
	}
}

// TestOmittedArgumentsStillMeanNone pins the boundary of the change: strict
// decoding must not turn a legitimately argument-less call into an error.
func TestOmittedArgumentsStillMeanNone(t *testing.T) {
	for _, args := range []string{``, `,"arguments":{}`, `,"arguments":null`} {
		req := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"cache_status"%s}}`, args)
		text, isErr := toolError(t, drive(t, testServerEngine(t), req)[0])
		if isErr {
			t.Errorf("cache_status with arguments %q was refused: %s", args, text)
		}
	}
}
