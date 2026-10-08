package mcpserver

import (
	"bytes"
	"context"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	internalschema "github.com/rshade/ax-go/internal/schema"
	"github.com/rshade/ax-go/schema"
)

const testServerVersion = "v1.2.3-test"

// noopRunE is a do-nothing command body; discovery tests never invoke it.
func noopRunE(*cobra.Command, []string) error { return nil }

// fixedRoot builds a deterministic command tree exercising every discovery
// rule: a root with a flag, a leaf command, a parent group with a child, a
// hidden leaf, a hidden group with a visible child (subtree pruning), and the
// reserved commands (__schema, mcp-server, and completion with a child). Only
// hidden subtrees and reserved commands must be excluded from the tool set.
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
	admin := &cobra.Command{Use: "admin", Short: "hidden group", Hidden: true, RunE: noopRunE}
	admin.AddCommand(&cobra.Command{Use: "reset", Short: "visible child of a hidden group", RunE: noopRunE})
	root.AddCommand(admin)

	root.AddCommand(&cobra.Command{Use: "__schema", Short: "schema", RunE: noopRunE})
	root.AddCommand(&cobra.Command{Use: "mcp-server", Short: "server", RunE: noopRunE})
	completion := &cobra.Command{Use: "completion", Short: "completion scripts", RunE: noopRunE}
	completion.AddCommand(&cobra.Command{Use: "bash", Short: "bash script", RunE: noopRunE})
	root.AddCommand(completion)

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
// root included; hidden subtrees (a hidden group's visible children included)
// and the reserved __schema, mcp-server, and completion commands excluded
// (FR-004/005/006, C-2, INV-3).
func TestDiscoverToolsExcludesHiddenAndReserved(t *testing.T) {
	got := toolNameSet(newTestDispatcher(fixedRoot()).tools)

	for _, want := range []string{"demo", "demo-greet", "demo-group", "demo-group-child"} {
		if !got[want] {
			t.Errorf("expected tool %q to be present", want)
		}
	}
	for _, excluded := range []string{
		"demo-secret", "demo-admin", "demo-admin-reset",
		"demo-__schema", "demo-mcp-server", "demo-completion", "demo-completion-bash",
	} {
		if got[excluded] {
			t.Errorf("expected tool %q to be excluded", excluded)
		}
	}
}

// TestDiscoverToolsMatchesStaticAdapter asserts the live server's tool set is
// exactly the static __schema --as=mcp set (fixedRoot has no positional-arg
// commands, which are the only live-only exclusion): both paths share
// internal/mcp's walk and reserved-command exclusions, so they cannot diverge
// (D8, FR-004/005/006, INV-3).
func TestDiscoverToolsMatchesStaticAdapter(t *testing.T) {
	static := schema.BuildMCPSchema(fixedRoot())
	staticNames := toolNameSet(static.Tools)

	liveNames := toolNameSet(newTestDispatcher(fixedRoot()).tools)

	if !maps.Equal(liveNames, staticNames) {
		t.Errorf("live and static tool sets diverged\nlive:   %v\nstatic: %v", liveNames, staticNames)
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
	if got["demo-get"] {
		t.Errorf("expected positional-arg command %q to be excluded", "demo-get")
	}
	if !got["demo-list"] {
		t.Errorf("expected flag-only command %q to be present", "demo-list")
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
// name and injected version, the live tool set matches discovery, and every
// advertised tool name satisfies the MCP tool-name rule ^[a-zA-Z0-9_.-]+$
// (FR-003, C-1/C-2).
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
	namePattern := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	live := map[string]bool{}
	for _, tool := range res.Tools {
		if !namePattern.MatchString(tool.Name) {
			t.Errorf("live tool name %q violates the MCP name rule %s", tool.Name, namePattern)
		}
		live[tool.Name] = true
	}
	for _, want := range []string{"demo", "demo-greet", "demo-group", "demo-group-child"} {
		if !live[want] {
			t.Errorf("live tools/list missing %q", want)
		}
	}
	for _, excluded := range []string{
		"demo-secret", "demo-admin", "demo-admin-reset",
		"demo-__schema", "demo-mcp-server", "demo-completion", "demo-completion-bash",
	} {
		if live[excluded] {
			t.Errorf("live tools/list should exclude %q", excluded)
		}
	}
}

// TestDeclarationsAreNotServedInPhaseOne pins the Phase 2 trigger boundary
// (spec 028 FR-011) over the wire: prompt and resource declarations on the
// command tree are projected by __schema only. The live server registers
// neither, so initialize advertises no prompts or resources capability,
// prompts/list and resources/list return nothing, and the tools/list response
// names exactly the tools of the same tree without declarations.
func TestDeclarationsAreNotServedInPhaseOne(t *testing.T) {
	ctx := context.Background()
	liveToolNames := func(root *cobra.Command) []string {
		t.Helper()
		res, err := newInMemorySession(t, newTestServer(t, root)).ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("tools/list: %v", err)
		}
		names := make([]string, 0, len(res.Tools))
		for _, tool := range res.Tools {
			names = append(names, tool.Name)
		}
		return names
	}

	root := fixedRoot()
	if v := internalschema.AddPrompt(root, internalschema.Prompt{Name: "triage", Template: "t"}); v != nil {
		t.Fatalf("AddPrompt: %+v", v)
	}
	if v := internalschema.AddResource(root, internalschema.Resource{URI: "demo://docs", Name: "docs"}); v != nil {
		t.Fatalf("AddResource: %+v", v)
	}

	session := newInMemorySession(t, newTestServer(t, root))
	caps := session.InitializeResult().Capabilities
	if caps == nil {
		t.Fatal("initialize result missing capabilities")
	}
	if caps.Prompts != nil || caps.Resources != nil {
		t.Fatalf("capabilities advertise prompts=%v resources=%v, want neither", caps.Prompts, caps.Resources)
	}
	// The SDK answers prompts/list and resources/list even with nothing
	// registered, so the boundary is that both come back empty.
	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil || len(prompts.Prompts) != 0 {
		t.Fatalf("prompts/list = %+v (err %v), want no prompts in Phase 1", prompts, err)
	}
	resources, err := session.ListResources(ctx, nil)
	if err != nil || len(resources.Resources) != 0 {
		t.Fatalf("resources/list = %+v (err %v), want no resources in Phase 1", resources, err)
	}

	if got, want := liveToolNames(root), liveToolNames(fixedRoot()); !slices.Equal(got, want) {
		t.Fatalf("declarations changed the live tool set: got %v, want %v", got, want)
	}
}

// TestToolsListCarriesCapabilityAnnotations asserts the live tools/list
// registers the research.md R6 hint mapping as the SDK's ToolAnnotations on a
// classified tool and leaves Annotations nil on an unclassified one.
func TestToolsListCarriesCapabilityAnnotations(t *testing.T) {
	yes, no := true, false
	root := &cobra.Command{Use: "demo", Short: "demo root", RunE: noopRunE}
	classes := map[string]schema.Capability{
		"get":    schema.CapabilityReadOnly,
		"add":    schema.CapabilityCreate,
		"set":    schema.CapabilityMutate,
		"rm":     schema.CapabilityDelete,
		"fetch":  schema.CapabilityExternalNetwork,
		"reboot": schema.CapabilityAdmin,
	}
	for use, class := range classes {
		cmd := &cobra.Command{Use: use, Short: use, RunE: noopRunE}
		if err := schema.WithCapability(cmd, class, ""); err != nil {
			t.Fatalf("WithCapability(%s): %v", use, err)
		}
		root.AddCommand(cmd)
	}
	root.AddCommand(&cobra.Command{Use: "plain", Short: "plain", RunE: noopRunE})

	res, err := newInMemorySession(t, newTestServer(t, root)).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	want := map[string]*sdk.ToolAnnotations{
		"demo-get":    {ReadOnlyHint: true},
		"demo-add":    {DestructiveHint: &no},
		"demo-set":    {DestructiveHint: &yes},
		"demo-rm":     {DestructiveHint: &yes},
		"demo-fetch":  {OpenWorldHint: &yes},
		"demo-reboot": {DestructiveHint: &yes},
		"demo-plain":  nil,
		"demo":        nil,
	}
	seen := 0
	for _, tool := range res.Tools {
		expected, tracked := want[tool.Name]
		if !tracked {
			continue
		}
		seen++
		if !reflect.DeepEqual(tool.Annotations, expected) {
			t.Errorf("%s annotations = %+v, want %+v", tool.Name, tool.Annotations, expected)
		}
	}
	if seen != len(want) {
		t.Fatalf("tools/list returned %d of the %d expected tools", seen, len(want))
	}
}

// TestDiscoverToolsCapabilityMatchesStaticAdapter asserts the live tool values
// equal the static --as=mcp values field for field on a classified tree, so the
// two conversions from internal/mcp.Tool cannot drift apart.
func TestDiscoverToolsCapabilityMatchesStaticAdapter(t *testing.T) {
	newRoot := func() *cobra.Command {
		root := fixedRoot()
		for _, child := range root.Commands() {
			if child.Name() == "greet" {
				if err := schema.WithCapability(child, schema.CapabilityCreate, "one greeting per call"); err != nil {
					t.Fatalf("WithCapability: %v", err)
				}
			}
		}
		if err := schema.WithCapability(root, schema.CapabilityExternalNetwork, ""); err != nil {
			t.Fatalf("WithCapability: %v", err)
		}
		return root
	}
	static := schema.BuildMCPSchema(newRoot()).Tools
	live := newTestDispatcher(newRoot()).tools
	byName := func(tools []schema.MCPTool) map[string]schema.MCPTool {
		out := make(map[string]schema.MCPTool, len(tools))
		for _, tool := range tools {
			tool.InputSchema = nil
			out[tool.Name] = tool
		}
		return out
	}
	for name, liveTool := range byName(live) {
		if !reflect.DeepEqual(liveTool, byName(static)[name]) {
			t.Errorf("tool %s diverged\nlive:   %+v\nstatic: %+v", name, liveTool, byName(static)[name])
		}
	}
}
