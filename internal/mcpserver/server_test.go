package mcpserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
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

// TestCapabilitiesAdvertiseOnlyWhatIsDeclared pins spec 030 FR-002/FR-008: the
// prompts and resources capabilities appear exactly when something is declared
// to back them, and declarations never change the tool set.
func TestCapabilitiesAdvertiseOnlyWhatIsDeclared(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name          string
		prompts       bool
		resources     bool
		wantPrompts   bool
		wantResources bool
	}{
		{name: "none"},
		{name: "prompts only", prompts: true, wantPrompts: true},
		{name: "resources only", resources: true, wantResources: true},
		{name: "both", prompts: true, resources: true, wantPrompts: true, wantResources: true},
	}
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
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixedRoot()
			if tc.prompts {
				mustAddPromptTo(t, root, "triage")
			}
			if tc.resources {
				mustAddResourceTo(t, root, "demo://docs")
			}

			caps := newInMemorySession(t, newTestServer(t, root)).InitializeResult().Capabilities
			if caps == nil {
				t.Fatal("initialize result missing capabilities")
			}
			if got := caps.Prompts != nil; got != tc.wantPrompts {
				t.Errorf("prompts capability advertised = %v, want %v", got, tc.wantPrompts)
			}
			if got := caps.Resources != nil; got != tc.wantResources {
				t.Errorf("resources capability advertised = %v, want %v", got, tc.wantResources)
			}
			if got, want := liveToolNames(root), liveToolNames(fixedRoot()); !slices.Equal(got, want) {
				t.Fatalf("declarations changed the live tool set: got %v, want %v", got, want)
			}
		})
	}
}

// TestServeFailsClosedOnDeclarationTreeConflicts pins FR-019: a duplicate
// prompt name, a duplicate resource URI, and a corrupt declaration annotation
// each fail Serve at startup with the validation_error envelope __schema emits
// (exit 2), before any transport starts, so a server that starts is guaranteed
// to serve the same set __schema projects.
func TestServeFailsClosedOnDeclarationTreeConflicts(t *testing.T) {
	cases := []struct {
		name  string
		build func(root *cobra.Command)
		kind  string
	}{
		{
			name: "duplicate prompt name",
			build: func(root *cobra.Command) {
				mustAddPromptTo(t, root, "triage")
				mustAddPromptTo(t, root.Commands()[0], "triage")
			},
			kind: "prompt",
		},
		{
			name: "duplicate resource URI",
			build: func(root *cobra.Command) {
				mustAddResourceTo(t, root, "demo://docs")
				mustAddResourceTo(t, root.Commands()[0], "demo://docs")
			},
			kind: "resource",
		},
		{
			name: "corrupt annotation",
			build: func(root *cobra.Command) {
				root.Annotations = map[string]string{"github.com/rshade/ax-go/schema/prompts": "{not json"}
			},
			kind: "prompt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixedRoot()
			tc.build(root)

			err := Serve(context.Background(), root, Config{Version: testServerVersion, Stderr: io.Discard})

			var envelope *contract.Error
			if !errors.As(err, &envelope) {
				t.Fatalf("Serve error = %v, want *contract.Error", err)
			}
			if envelope.ErrorCode != "validation_error" || envelope.ExitCode() != contract.ExitValidation {
				t.Fatalf("envelope = %s exit %d, want validation_error exit 2", envelope.ErrorCode, envelope.ExitCode())
			}
			if envelope.Context["kind"] != tc.kind {
				t.Fatalf("context.kind = %v, want %s", envelope.Context["kind"], tc.kind)
			}
		})
	}
}

func mustAddPromptTo(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()
	if v := internalschema.AddPrompt(cmd, internalschema.Prompt{Name: name, Template: "t"}); v != nil {
		t.Fatalf("AddPrompt: %+v", v)
	}
}

func mustAddResourceTo(t *testing.T, cmd *cobra.Command, uri string) {
	t.Helper()
	if v := internalschema.AddResource(cmd, internalschema.Resource{URI: uri, Name: "n"}); v != nil {
		t.Fatalf("AddResource: %+v", v)
	}
}

// TestInstructionsReachInitialize pins FR-010: configured instructions arrive
// in the initialize result, and an unset option leaves the field absent so the
// handshake stays what v0.8.0 sent.
func TestInstructionsReachInitialize(t *testing.T) {
	const text = "Read demo://docs/skill before calling any tool."
	forEachTransport(t, fixedRoot(), Config{Instructions: text}, func(t *testing.T, session *sdk.ClientSession) {
		if got := session.InitializeResult().Instructions; got != text {
			t.Fatalf("instructions = %q, want %q", got, text)
		}
	})
	forEachTransport(t, fixedRoot(), Config{}, func(t *testing.T, session *sdk.ClientSession) {
		if got := session.InitializeResult().Instructions; got != "" {
			t.Fatalf("instructions = %q, want none when the option is unset", got)
		}
	})
}

func TestValidateInstructions(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{name: "empty is allowed", text: ""},
		{name: "plain text", text: "Read demo://docs/skill first."},
		{name: "multi-byte text", text: "café ✓ 日本"},
		{name: "exactly at the cap", text: strings.Repeat("a", maxInstructionsBytes)},
		{name: "one byte over the cap", text: strings.Repeat("a", maxInstructionsBytes+1), wantErr: true},
		{name: "invalid utf8", text: "ok" + string([]byte{0xff, 0xfe}), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateInstructions(context.Background(), tc.text)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("validateInstructions = %v, want nil", err)
				}
				return
			}
			var envelope *contract.Error
			if !errors.As(err, &envelope) || envelope.ErrorCode != "validation_error" ||
				envelope.ExitCode() != contract.ExitValidation {
				t.Fatalf("validateInstructions = %v, want a validation_error envelope with exit 2", err)
			}
			if strings.Contains(envelope.Message, "aaaaaaaa") {
				t.Fatalf("message echoes the instructions text: %.120s", envelope.Message)
			}
		})
	}
}

func TestServeRejectsInvalidInstructionsBeforeStarting(t *testing.T) {
	err := Serve(context.Background(), fixedRoot(), Config{
		Version: testServerVersion, Stderr: io.Discard, Instructions: string([]byte{0xff}),
	})
	var envelope *contract.Error
	if !errors.As(err, &envelope) || envelope.ExitCode() != contract.ExitValidation {
		t.Fatalf("Serve error = %v, want a validation envelope with exit 2", err)
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
		if err := schema.DeclareCapability(cmd, class, ""); err != nil {
			t.Fatalf("DeclareCapability(%s): %v", use, err)
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
				if err := schema.DeclareCapability(
					child,
					schema.CapabilityCreate,
					"one greeting per call",
				); err != nil {
					t.Fatalf("DeclareCapability: %v", err)
				}
			}
		}
		if err := schema.DeclareCapability(root, schema.CapabilityExternalNetwork, ""); err != nil {
			t.Fatalf("DeclareCapability: %v", err)
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
