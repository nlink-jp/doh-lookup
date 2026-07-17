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
