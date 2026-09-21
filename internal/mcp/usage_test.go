package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestUsageMentionsEveryTool pins the manual to the advertised tool set: if a
// tool is added or renamed, usage.md must mention it too.
func TestUsageMentionsEveryTool(t *testing.T) {
	b, _ := json.Marshal(toolsList())
	var payload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("marshal/unmarshal toolsList: %v", err)
	}
	for _, tool := range payload.Tools {
		if !strings.Contains(usageMarkdown, "`"+tool.Name+"`") {
			t.Errorf("usage.md does not document the %q tool", tool.Name)
		}
	}
}

func TestUsageDocumentsErrorCodes(t *testing.T) {
	for _, code := range []string{"invalid_input", "network_error"} {
		if !strings.Contains(usageMarkdown, code) {
			t.Errorf("usage.md does not document error code %q", code)
		}
	}
}

// TestEveryToolSchemaIsValidAndClosed keeps a mistyped argument from reading as
// a real one: org ADR-021 §10 requires every registered schema to set
// additionalProperties:false, and requires this assertion to exist, because a
// rule stated only in prose is re-decided by whoever adds the next tool.
//
// The flag is the declared half of the contract — what a schema-checking client
// refuses before the call. The enforcing half is decodeArgs
// (DisallowUnknownFields), which refuses an unknown argument that arrives
// anyway; TestUnknownArgumentIsRefusedByName covers it.
func TestEveryToolSchemaIsValidAndClosed(t *testing.T) {
	b, err := json.Marshal(toolsList())
	if err != nil {
		t.Fatalf("marshal tool list: %v", err)
	}
	var list struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Type                 string `json:"type"`
				AdditionalProperties *bool  `json:"additionalProperties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(b, &list); err != nil {
		t.Fatalf("tool list is not valid JSON: %v", err)
	}
	// Without this the loop below passes by having nothing to check.
	if len(list.Tools) == 0 {
		t.Fatal("toolsList returned no tools")
	}
	for _, tool := range list.Tools {
		if tool.InputSchema.Type != "object" {
			t.Errorf("%s: schema type = %q, want object", tool.Name, tool.InputSchema.Type)
		}
		if tool.InputSchema.AdditionalProperties == nil || *tool.InputSchema.AdditionalProperties {
			t.Errorf("%s: schema should set additionalProperties:false so typos are caught", tool.Name)
		}
	}
}
