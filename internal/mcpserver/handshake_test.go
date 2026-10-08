package mcpserver

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestInitializeResultMatchesV080Baseline pins the initialize result of a
// declaration-free server byte for byte: a CLI that declares no prompts,
// resources, or instructions must advertise exactly what v0.8.0 did
// (spec 030 SC-002). Regenerate with UPDATE_GOLDEN=1 only from the v0.8.0
// server, never to accommodate a change.
func TestInitializeResultMatchesV080Baseline(t *testing.T) {
	session := newInMemorySession(t, newTestServer(t, fixedRoot()))

	got, err := json.Marshal(session.InitializeResult())
	if err != nil {
		t.Fatalf("marshal initialize result: %v", err)
	}
	got = append(got, '\n')

	goldenPath := filepath.Join("..", "..", "testdata", "mcp_initialize_baseline.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\nwant: %s\ngot:  %s", goldenPath, want, got)
	}
}
