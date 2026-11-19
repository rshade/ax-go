package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync/atomic"
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

// TestExcludeFlagOmittedFromStaticAndLiveSchemas asserts a flag marked with
// mcp.ExcludeFlag is absent from both "__schema --as=mcp" and live "tools/list"
// inputSchema.properties and inputSchema.required while non-excluded flags stay.
func TestExcludeFlagOmittedFromStaticAndLiveSchemas(t *testing.T) {
	newTree := func() *cobra.Command {
		root := &cobra.Command{Use: "tsacl", Short: "root"}
		testCmd := &cobra.Command{
			Use:   "test",
			Short: "run policy tests",
			RunE:  func(*cobra.Command, []string) error { return nil },
		}
		testCmd.Flags().Bool("watch", false, "watch for changes")
		testCmd.Flags().String("prompt", "", "interactive prompt")
		testCmd.Flags().String("filter", "", "filter tests")
		testCmd.Flags().String("suite", "", "test suite")
		_ = testCmd.MarkFlagRequired("prompt")
		_ = testCmd.MarkFlagRequired("suite")

		mcp.ExcludeFlag(testCmd, "watch")
		mcp.ExcludeFlag(testCmd, "prompt")
		root.AddCommand(testCmd)
		return root
	}

	staticRoot := newTree()
	staticSchema := schema.BuildMCPSchema(staticRoot)
	if len(staticSchema.Tools) != 1 {
		t.Fatalf("static tools count = %d, want 1", len(staticSchema.Tools))
	}
	staticProps, ok := staticSchema.Tools[0].InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("static properties is %T, want map[string]any", staticSchema.Tools[0].InputSchema["properties"])
	}
	for _, excluded := range []string{"watch", "prompt"} {
		if _, present := staticProps[excluded]; present {
			t.Errorf("static inputSchema.properties unexpectedly contains excluded flag %q", excluded)
		}
	}
	for _, kept := range []string{"filter", "suite"} {
		if _, present := staticProps[kept]; !present {
			t.Errorf("static inputSchema.properties missing non-excluded flag %q", kept)
		}
	}
	staticReq, ok := staticSchema.Tools[0].InputSchema["required"].([]string)
	if !ok || !slices.Equal(staticReq, []string{"suite"}) {
		t.Errorf("static required = %v, want [suite]", staticSchema.Tools[0].InputSchema["required"])
	}

	session := serveTestHTTP(t, newTree())
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("live tools count = %d, want 1", len(res.Tools))
	}
	rawSchema, err := json.Marshal(res.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshal live InputSchema: %v", err)
	}
	var liveSchema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	mustUnmarshal(t, string(rawSchema), &liveSchema)
	for _, excluded := range []string{"watch", "prompt"} {
		if _, present := liveSchema.Properties[excluded]; present {
			t.Errorf("live inputSchema.properties unexpectedly contains excluded flag %q", excluded)
		}
	}
	for _, kept := range []string{"filter", "suite"} {
		if _, present := liveSchema.Properties[kept]; !present {
			t.Errorf("live inputSchema.properties missing non-excluded flag %q", kept)
		}
	}
	if !slices.Equal(liveSchema.Required, []string{"suite"}) {
		t.Errorf("live required = %v, want [suite]", liveSchema.Required)
	}
}

// TestExcludeFlagStaysInHelpAndAXSchema asserts that excluding a flag from MCP
// leaves --help, direct CLI execution, and "__schema --as=ax" untouched, and
// records the canonical annotation on the pflag.
func TestExcludeFlagStaysInHelpAndAXSchema(t *testing.T) {
	var watched bool
	root := &cobra.Command{Use: "tsacl", Short: "root"}
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "run policy tests",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	testCmd.Flags().BoolVar(&watched, "watch", false, "watch for changes")
	testCmd.Flags().String("filter", "", "filter tests")
	mcp.ExcludeFlag(testCmd, "watch")
	root.AddCommand(testCmd)
	root.AddCommand(schema.NewSchemaCommand(root))

	var help bytes.Buffer
	root.SetOut(&help)
	root.SetArgs([]string{"test", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("test --help: %v", err)
	}
	if !strings.Contains(help.String(), "--watch") {
		t.Errorf("test --help omits --watch:\n%s", help.String())
	}

	var axOut bytes.Buffer
	root.SetOut(&axOut)
	root.SetArgs([]string{"__schema", "--as=ax"})
	if err := root.Execute(); err != nil {
		t.Fatalf("__schema --as=ax: %v", err)
	}
	var built schema.Schema
	mustUnmarshal(t, axOut.String(), &built)
	var flagNames []string
	for _, child := range built.Command.Commands {
		if child.Use == "test" {
			for _, f := range child.Flags {
				flagNames = append(flagNames, f.Name)
			}
		}
	}
	if !slices.Contains(flagNames, "watch") {
		t.Errorf("__schema --as=ax test flags = %v, want watch present", flagNames)
	}

	flag := testCmd.Flags().Lookup("watch")
	if got := flag.Annotations[excludeAnnotationKey]; !slices.Equal(got, []string{"true"}) {
		t.Errorf("watch flag annotation %q = %v, want [true]", excludeAnnotationKey, got)
	}

	// Reset help flag set by "test --help" above and verify direct CLI execution accepts --watch.
	_ = testCmd.Flags().Lookup("help").Value.Set("false")
	root.SetArgs([]string{"test", "--watch"})
	if err := root.Execute(); err != nil {
		t.Fatalf("direct CLI execution with --watch: %v", err)
	}
	if !watched {
		t.Error("direct CLI execution did not set --watch=true")
	}
}

func TestExcludeFlagNoopAndPreservesAnnotations(t *testing.T) {
	mcp.ExcludeFlag(nil, "watch")

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("watch", false, "watch")
	mcp.ExcludeFlag(cmd, "")
	mcp.ExcludeFlag(cmd, "no-such-flag")

	flag := cmd.Flags().Lookup("watch")
	if got := flag.Annotations[excludeAnnotationKey]; len(got) != 0 {
		t.Errorf("unexpected annotation after no-op calls: %v", got)
	}

	flag.Annotations = map[string][]string{"keep": {"me"}}
	mcp.ExcludeFlag(cmd, "watch")
	if got := flag.Annotations["keep"]; !slices.Equal(got, []string{"me"}) {
		t.Errorf("existing annotation = %v, want [me]", got)
	}
	if got := flag.Annotations[excludeAnnotationKey]; !slices.Equal(got, []string{"true"}) {
		t.Errorf("exclude annotation = %v, want [true]", got)
	}
}

// TestExcludeFlagRejectedOnToolsCallAndNeverRuns asserts that supplying a flag
// excluded with mcp.ExcludeFlag over a live MCP session returns IsError=true
// with a validation_error ax.Error envelope (exit 2) and never invokes RunE,
// while calling the same tool without the excluded flag succeeds.
func TestExcludeFlagRejectedOnToolsCallAndNeverRuns(t *testing.T) {
	// RunE runs on the HTTP server goroutine, so the count is shared across
	// goroutines and must be synchronized explicitly.
	var calls atomic.Int32
	var filter string
	root := &cobra.Command{Use: "tsacl", Short: "root"}
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "run policy tests",
		RunE: func(cmd *cobra.Command, _ []string) error {
			calls.Add(1)
			_, err := cmd.OutOrStdout().Write([]byte(`{"filter":"` + filter + `"}`))
			return err
		},
	}
	testCmd.Flags().Bool("watch", false, "watch for changes")
	testCmd.Flags().StringVar(&filter, "filter", "default", "filter tests")
	mcp.ExcludeFlag(testCmd, "watch")
	root.AddCommand(testCmd)

	session := serveTestHTTP(t, root)

	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "tsacl-test",
		Arguments: map[string]any{"watch": true},
	})
	if err != nil {
		t.Fatalf("tools/call protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("tools/call with excluded flag watch=true succeeded, want IsError=true")
	}
	if len(res.Content) != 1 {
		t.Fatalf("len(res.Content) = %d, want 1", len(res.Content))
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want *sdk.TextContent", res.Content[0])
	}
	var errEnv struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}
	mustUnmarshal(t, text.Text, &errEnv)
	if errEnv.ErrorCode != "validation_error" {
		t.Errorf("error_code = %q, want validation_error", errEnv.ErrorCode)
	}
	if want := `unknown argument "watch" for tool "tsacl-test"`; !strings.Contains(errEnv.Message, want) {
		t.Errorf("message = %q, want it to contain %q", errEnv.Message, want)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("RunE ran %d times when excluded flag was supplied, want 0", got)
	}

	okRes, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "tsacl-test",
		Arguments: map[string]any{"filter": "dns"},
	})
	if err != nil {
		t.Fatalf("tools/call without watch: %v", err)
	}
	if okRes.IsError {
		t.Fatalf("tools/call without watch returned IsError=true: %+v", okRes.Content)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("RunE ran %d times, want 1", got)
	}
}

// TestExcludeFlagInheritanceAndShadowingParity asserts that static
// "__schema --as=mcp" and live "mcp-server" agree on persistent flag exclusion
// inheritance and local flag shadowing.
func TestExcludeFlagInheritanceAndShadowingParity(t *testing.T) {
	newTree := func() *cobra.Command {
		noop := func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write([]byte(`{"ok":true}`))
			return err
		}
		root := &cobra.Command{Use: "tsacl", Short: "root", RunE: noop}
		root.PersistentFlags().Bool("global-watch", false, "excluded persistent")
		root.PersistentFlags().String("mode", "fast", "unexcluded on root")
		root.PersistentFlags().String("region", "us", "excluded on root")
		mcp.ExcludeFlag(root, "global-watch")
		mcp.ExcludeFlag(root, "region")

		child := &cobra.Command{Use: "test", Short: "child", RunE: noop}
		child.Flags().String("mode", "slow", "excluded local shadow on child")
		child.Flags().String("region", "eu", "unexcluded local shadow on child")
		mcp.ExcludeFlag(child, "mode")
		root.AddCommand(child)
		return root
	}

	staticSchema := schema.BuildMCPSchema(newTree())
	session := serveTestHTTP(t, newTree())
	liveList, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(staticSchema.Tools) != 2 || len(liveList.Tools) != 2 {
		t.Fatalf("tools count: static=%d live=%d, want 2", len(staticSchema.Tools), len(liveList.Tools))
	}

	wantByTool := map[string]struct {
		present []string
		absent  []string
	}{
		"tsacl":      {present: []string{"mode"}, absent: []string{"global-watch", "region"}},
		"tsacl-test": {present: []string{"region"}, absent: []string{"global-watch", "mode"}},
	}

	for i, staticTool := range staticSchema.Tools {
		want, ok := wantByTool[staticTool.Name]
		if !ok {
			t.Fatalf("unexpected tool %q", staticTool.Name)
		}
		staticProps := staticTool.InputSchema["properties"].(map[string]any)
		liveRaw, marshalErr := json.Marshal(liveList.Tools[i].InputSchema)
		if marshalErr != nil {
			t.Fatalf("marshal live InputSchema: %v", marshalErr)
		}
		var liveSchema struct {
			Properties map[string]any `json:"properties"`
		}
		mustUnmarshal(t, string(liveRaw), &liveSchema)

		for _, key := range want.present {
			if _, ok := staticProps[key]; !ok {
				t.Errorf("static %s missing expected property %q", staticTool.Name, key)
			}
			if _, ok := liveSchema.Properties[key]; !ok {
				t.Errorf("live %s missing expected property %q", staticTool.Name, key)
			}
		}
		for _, key := range want.absent {
			if _, ok := staticProps[key]; ok {
				t.Errorf("static %s unexpectedly contains excluded property %q", staticTool.Name, key)
			}
			if _, ok := liveSchema.Properties[key]; ok {
				t.Errorf("live %s unexpectedly contains excluded property %q", staticTool.Name, key)
			}
		}
	}

	// Verify live dispatch matches the shadowed schema on tsacl-test.
	for _, rejected := range []string{"global-watch", "mode"} {
		res, callErr := session.CallTool(context.Background(), &sdk.CallToolParams{
			Name:      "tsacl-test",
			Arguments: map[string]any{rejected: "x"},
		})
		if callErr != nil {
			t.Fatalf("tools/call %s protocol error: %v", rejected, callErr)
		}
		if !res.IsError {
			t.Errorf("tools/call tsacl-test with %q succeeded, want IsError=true", rejected)
		}
	}
	okRes, err := session.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "tsacl-test",
		Arguments: map[string]any{"region": "eu"},
	})
	if err != nil || okRes.IsError {
		t.Errorf("tools/call tsacl-test with unexcluded shadowed region failed: err=%v res=%+v", err, okRes)
	}
}
