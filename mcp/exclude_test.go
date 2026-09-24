package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/mcp"
	"github.com/rshade/ax-go/schema"
)

const excludeAnnotationKey = "github.com/rshade/ax-go/mcp/exclude"

// finTree is a finfocus-shaped command tree: a runnable TUI root marked
// excluded, six pure group commands, Cobra's default help and completion
// commands, a blocking "analyzer serve" leaf marked excluded, and a hidden
// subtree. The counters record every RunE invocation of the excluded commands.
type finTree struct {
	root       *cobra.Command
	rootCalls  *int
	serveCalls *int
}

func newFinTree() finTree {
	var rootCalls, serveCalls int
	root := &cobra.Command{Use: "fin", Short: "interactive TUI", RunE: func(*cobra.Command, []string) error {
		rootCalls++
		return nil
	}}
	mcp.Exclude(root)

	leaf := func(use string) *cobra.Command {
		return &cobra.Command{Use: use, Short: use, RunE: func(*cobra.Command, []string) error { return nil }}
	}
	group := func(use string, children ...*cobra.Command) *cobra.Command {
		cmd := &cobra.Command{Use: use, Short: use + " commands"}
		cmd.AddCommand(children...)
		return cmd
	}

	serve := &cobra.Command{Use: "serve", Short: "blocking gRPC handshake", RunE: func(*cobra.Command, []string) error {
		serveCalls++
		return nil
	}}
	mcp.Exclude(serve)

	debug := &cobra.Command{Use: "debug", Short: "hidden", Hidden: true, RunE: leaf("x").RunE}
	debug.AddCommand(leaf("dump"))

	root.AddCommand(
		group("analyzer", leaf("install"), serve),
		group("config", leaf("get"), leaf("set")),
		group("cost", leaf("actual"), leaf("projected")),
		group("history", leaf("list")),
		group("plugin", leaf("list"), leaf("install")),
		group("recommendations", leaf("list")),
		debug,
	)
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	return finTree{root: root, rootCalls: &rootCalls, serveCalls: &serveCalls}
}

// staticToolNames runs the real "__schema --as=mcp" command on root and
// returns the tool names it emits.
func staticToolNames(t *testing.T, root *cobra.Command) []string {
	t.Helper()
	root.AddCommand(schema.NewSchemaCommand(root))
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"__schema", "--as=mcp"})
	if err := root.Execute(); err != nil {
		t.Fatalf("__schema --as=mcp: %v", err)
	}
	var out schema.MCPSchema
	mustUnmarshal(t, stdout.String(), &out)
	names := make([]string, 0, len(out.Tools))
	for _, tool := range out.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func liveToolNames(t *testing.T, session *sdk.ClientSession) []string {
	t.Helper()
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// TestExcludeStaticAndLiveParity asserts the static __schema --as=mcp adapter
// and the live tools/list advertise the identical, ordered tool list for a
// finfocus-shaped tree: groups, help, completion, the hidden subtree, the
// excluded root, and the excluded blocking leaf are all absent, and no
// runnable, unannotated command is lost.
func TestExcludeStaticAndLiveParity(t *testing.T) {
	// Every runnable, unannotated leaf of newFinTree, in walk order.
	finCallableTools := []string{
		"fin-analyzer-install",
		"fin-config-get",
		"fin-config-set",
		"fin-cost-actual",
		"fin-cost-projected",
		"fin-history-list",
		"fin-plugin-install",
		"fin-plugin-list",
		"fin-recommendations-list",
	}

	static := staticToolNames(t, newFinTree().root)
	if !slices.Equal(static, finCallableTools) {
		t.Errorf("static tools = %v, want %v", static, finCallableTools)
	}

	live := liveToolNames(t, serveTestHTTP(t, newFinTree().root))
	if !slices.Equal(live, static) {
		t.Errorf("live tools diverged from static\nlive:   %v\nstatic: %v", live, static)
	}
}

// TestExcludedToolCallIsUnknownAndNeverRuns asserts a tools/call on any
// excluded or skipped command's derived name fails exactly as a call on a name
// that was never registered does, and the excluded commands' RunE bodies never
// execute. The SDK rejects unregistered names before the dispatcher runs; the
// dispatcher's own unknown-tool guard is covered in internal/mcpserver.
func TestExcludedToolCallIsUnknownAndNeverRuns(t *testing.T) {
	tree := newFinTree()
	session := serveTestHTTP(t, tree.root)

	callErr := func(name string) string {
		t.Helper()
		res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name})
		if err == nil {
			t.Fatalf("tools/call %s succeeded (IsError=%v), want unknown-tool error", name, res.IsError)
		}
		return strings.ReplaceAll(err.Error(), name, "<name>")
	}
	unregistered := callErr("fin-no-such-tool")

	for _, name := range []string{"fin", "fin-analyzer-serve", "fin-analyzer", "fin-help", "fin-debug-dump"} {
		t.Run(name, func(t *testing.T) {
			if got := callErr(name); got != unregistered {
				t.Errorf("tools/call %s error = %q, want the unregistered-name error %q", name, got, unregistered)
			}
		})
	}

	if *tree.rootCalls != 0 {
		t.Errorf("excluded root RunE ran %d times, want 0", *tree.rootCalls)
	}
	if *tree.serveCalls != 0 {
		t.Errorf("excluded serve RunE ran %d times, want 0", *tree.serveCalls)
	}
}

// TestExcludedCommandStaysInHelpAndAXSchema asserts exclusion is MCP-only: the
// excluded command is still listed in its parent's --help output and in the
// __schema --as=ax command tree, and the mark is inspectable on the Cobra
// command's annotations.
func TestExcludedCommandStaysInHelpAndAXSchema(t *testing.T) {
	tree := newFinTree()
	root := tree.root
	root.AddCommand(schema.NewSchemaCommand(root))

	var help bytes.Buffer
	root.SetOut(&help)
	root.SetArgs([]string{"analyzer", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("analyzer --help: %v", err)
	}
	if !strings.Contains(help.String(), "serve") {
		t.Errorf("analyzer --help omits the excluded serve command:\n%s", help.String())
	}

	var axOut bytes.Buffer
	root.SetOut(&axOut)
	root.SetArgs([]string{"__schema", "--as=ax"})
	if err := root.Execute(); err != nil {
		t.Fatalf("__schema --as=ax: %v", err)
	}
	var built schema.Schema
	if err := json.Unmarshal(axOut.Bytes(), &built); err != nil {
		t.Fatalf("unmarshal ax schema: %v", err)
	}
	var analyzerChildren []string
	for _, child := range built.Command.Commands {
		if child.Use == "analyzer" {
			for _, grandchild := range child.Commands {
				analyzerChildren = append(analyzerChildren, grandchild.Use)
			}
		}
	}
	if !slices.Contains(analyzerChildren, "serve") {
		t.Errorf("__schema --as=ax analyzer children = %v, want serve present", analyzerChildren)
	}
	serve, _, err := root.Find([]string{"analyzer", "serve"})
	if err != nil {
		t.Fatalf("find analyzer serve: %v", err)
	}
	if got := serve.Annotations[excludeAnnotationKey]; got != "true" {
		t.Errorf("serve annotation %q = %q, want %q", excludeAnnotationKey, got, "true")
	}
	if tree.root.Use != built.Command.Use {
		t.Errorf("__schema --as=ax root = %q, want the excluded root %q", built.Command.Use, tree.root.Use)
	}
	if *tree.serveCalls != 0 || *tree.rootCalls != 0 {
		t.Errorf("help/schema ran excluded RunE bodies: root=%d serve=%d", *tree.rootCalls, *tree.serveCalls)
	}
}

func TestExcludeSetsCanonicalAnnotationAndPreservesOthers(t *testing.T) {
	cmd := &cobra.Command{Use: "serve", Annotations: map[string]string{"keep": "me"}}
	mcp.Exclude(cmd)

	if got := cmd.Annotations[excludeAnnotationKey]; got != "true" {
		t.Errorf("annotation %q = %q, want %q", excludeAnnotationKey, got, "true")
	}
	if got := cmd.Annotations["keep"]; got != "me" {
		t.Errorf("existing annotation = %q, want %q", got, "me")
	}
}

func TestExcludeNilIsNoop(t *testing.T) {
	mcp.Exclude(nil)
}

// TestNewCommandExampleUsesRootName asserts the mcp-server help example names
// the adopting CLI rather than a placeholder.
func TestNewCommandExampleUsesRootName(t *testing.T) {
	root := &cobra.Command{Use: "fin"}
	cmd := mcp.NewCommand(root)

	if strings.Contains(cmd.Example, "mycli") {
		t.Errorf("Example still uses the mycli placeholder:\n%s", cmd.Example)
	}
	if !strings.Contains(cmd.Example, "  fin mcp-server\n") {
		t.Errorf("Example does not name the root CLI:\n%s", cmd.Example)
	}
}

func TestNewCommandNilRootDoesNotPanic(t *testing.T) {
	if cmd := mcp.NewCommand(nil); cmd == nil {
		t.Fatal("NewCommand(nil) returned nil")
	}
}
