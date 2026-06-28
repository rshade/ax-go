package mcpserver

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/schema"
)

const testServerVersion = "v1.2.3-test"

// noopRunE is a do-nothing command body; discovery tests never invoke it.
func noopRunE(*cobra.Command, []string) error { return nil }

// fixedRoot builds a deterministic command tree exercising every discovery
// rule: a root with a flag, a leaf command, a parent group with a child, a
// hidden command, and the two reserved commands (__schema, mcp-server). Only
// hidden and reserved commands must be excluded from the tool set.
func fixedRoot() *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
	root.Flags().String("name", "world", "name to greet")

	greet := &cobra.Command{Use: "greet", Short: "greet someone", RunE: noopRunE}
	greet.Flags().Int("times", 1, "repeat count")
	greet.Flags().StringSlice("tags", []string{"friendly"}, "tags to apply")
	root.AddCommand(greet)

	group := &cobra.Command{Use: "group", Short: "a command group", RunE: noopRunE}
	group.AddCommand(&cobra.Command{Use: "child", Short: "group child", RunE: noopRunE})
	root.AddCommand(group)

	root.AddCommand(&cobra.Command{Use: "secret", Short: "hidden", Hidden: true, RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "__schema", Short: "schema", RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "mcp-server", Short: "server", RunE: noopRunE})

	return root
}

func newTestDispatcher(root *cobra.Command) *dispatcher {
	return newDispatcher(context.Background(), root, Config{
		Version:    testServerVersion,
		ServerName: root.Name(),
		Stderr:     io.Discard,
	})
}

func newTestServer(t *testing.T, root *cobra.Command) *sdk.Server {
	t.Helper()
	return newTestServerCtx(t, context.Background(), root)
}

// newTestServerCtx builds a server whose dispatcher uses ctx as its serve
// context, so canceling ctx cancels in-flight calls (exercised by the shutdown
// drain test). The production Serve threads its own context the same way.
func newTestServerCtx(t *testing.T, ctx context.Context, root *cobra.Command) *sdk.Server {
	t.Helper()
	cfg := Config{Version: testServerVersion, ServerName: root.Name(), Stderr: io.Discard}
	return newMCPServer(newDispatcher(ctx, root, cfg), cfg)
}

// newInMemorySession connects an in-memory client to server and returns the
// initialized client session. The in-memory transport models the stdio
// single-session path without OS pipes.
func newInMemorySession(t *testing.T, server *sdk.Server) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0-test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func toolNameSet(tools []schema.MCPTool) map[string]bool {
	set := make(map[string]bool, len(tools))
	for _, tool := range tools {
		set[tool.Name] = true
	}
	return set
}

// TestDiscoverToolsExcludesHiddenAndReserved asserts the callable tool set is
// exactly the non-hidden, non-reserved commands — parent/group commands and the
// root included, hidden and __schema/mcp-server excluded (FR-004/005/006, C-2,
// INV-3).
func TestDiscoverToolsExcludesHiddenAndReserved(t *testing.T) {
	got := toolNameSet(newTestDispatcher(fixedRoot()).tools)

	for _, want := range []string{"demo", "demo greet", "demo group", "demo group child"} {
		if !got[want] {
			t.Errorf("expected tool %q to be present", want)
		}
	}
	for _, excluded := range []string{"demo secret", "demo __schema", "demo mcp-server"} {
		if got[excluded] {
			t.Errorf("expected tool %q to be excluded", excluded)
		}
	}
}

// TestDiscoverToolsExcludesPositionalArgCommands asserts a command whose Args
// validator rejects zero arguments is excluded from the callable tool set: the
// flat MCP argument object maps only onto flags, so such a command could never
// be satisfied by a tools/call and must not be advertised as a tool that always
// fails.
func TestDiscoverToolsExcludesPositionalArgCommands(t *testing.T) {
	root := &cobra.Command{Use: "demo", Short: "root", RunE: noopRunE}
	root.AddCommand(&cobra.Command{Use: "get", Short: "get", Args: cobra.ExactArgs(1), RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "list", Short: "list", RunE: noopRunE})

	got := toolNameSet(newTestDispatcher(root).tools)
	if got["demo get"] {
		t.Errorf("expected positional-arg command %q to be excluded", "demo get")
	}
	if !got["demo list"] {
		t.Errorf("expected flag-only command %q to be present", "demo list")
	}
}

// TestToolsListGolden guards the discovered tool set's byte-for-byte shape and
// its discovery parity with __schema --as=mcp (minus reserved commands)
// (FR-019, SC-002/006, C-3/C-4). Regenerate with UPDATE_GOLDEN=1.
func TestToolsListGolden(t *testing.T) {
	tools := newTestDispatcher(fixedRoot()).tools

	var buf bytes.Buffer
	if err := contract.WriteJSON(&buf, schema.MCPSchema{Tools: tools}); err != nil {
		t.Fatalf("marshal tools list: %v", err)
	}

	goldenPath := filepath.Join("..", "..", "testdata", "mcp_tools_list.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("golden mismatch for %s\nwant: %s\ngot:  %s", goldenPath, want, buf.Bytes())
	}
}

// TestInitializeAndToolsListOverInMemory drives a live client↔server session
// through initialize and tools/list, asserting the handshake reports the server
// name and injected version and the live tool set matches discovery (FR-003,
// C-1/C-2).
func TestInitializeAndToolsListOverInMemory(t *testing.T) {
	root := fixedRoot()
	server := newTestServer(t, root)
	session := newInMemorySession(t, server)

	init := session.InitializeResult()
	if init.ServerInfo == nil {
		t.Fatal("initialize result missing server info")
	}
	if init.ServerInfo.Name != "demo" {
		t.Errorf("server name = %q, want %q", init.ServerInfo.Name, "demo")
	}
	if init.ServerInfo.Version != testServerVersion {
		t.Errorf("server version = %q, want %q", init.ServerInfo.Version, testServerVersion)
	}

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	live := map[string]bool{}
	for _, tool := range res.Tools {
		live[tool.Name] = true
	}
	for _, want := range []string{"demo", "demo greet", "demo group", "demo group child"} {
		if !live[want] {
			t.Errorf("live tools/list missing %q", want)
		}
	}
	for _, excluded := range []string{"demo secret", "demo __schema", "demo mcp-server"} {
		if live[excluded] {
			t.Errorf("live tools/list should exclude %q", excluded)
		}
	}
}
