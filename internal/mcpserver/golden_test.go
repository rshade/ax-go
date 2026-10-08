package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// assertGolden compares got, as compact JSON plus a newline, against
// testdata/<name>.golden.json. Regenerate with UPDATE_GOLDEN=1.
func assertGolden(t *testing.T, name string, got any) {
	t.Helper()
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	encoded = append(encoded, '\n')

	path := filepath.Join("..", "..", "testdata", name+".golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, encoded, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("golden mismatch for %s\nwant: %s\ngot:  %s", path, want, encoded)
	}
}

// TestLiveServerResponsesGolden pins the wire shape of every response this
// feature adds (spec 030 FR-001, FR-003, FR-008, FR-010) as new golden files;
// no existing golden changes.
func TestLiveServerResponsesGolden(t *testing.T) {
	ctx := context.Background()
	const instructions = "Read demo://docs/skill before calling any tool."
	session := newInMemorySession(t, newServerWithConfig(t, ctx, declaredRoot(t), Config{Instructions: instructions}))

	assertGolden(t, "mcp_initialize_instructions", session.InitializeResult())

	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("prompts/list: %v", err)
	}
	assertGolden(t, "mcp_prompts_list", prompts)

	rendered, err := session.GetPrompt(ctx, &sdk.GetPromptParams{
		Name: "decide", Arguments: map[string]string{"question": "Should we ship?"},
	})
	if err != nil {
		t.Fatalf("prompts/get: %v", err)
	}
	assertGolden(t, "mcp_prompts_get", rendered)

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	assertGolden(t, "mcp_resources_list", resources)

	read, err := session.ReadResource(ctx, &sdk.ReadResourceParams{URI: "demo://docs/skill"})
	if err != nil {
		t.Fatalf("resources/read: %v", err)
	}
	assertGolden(t, "mcp_resources_read", read)
}
