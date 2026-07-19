package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
)

const echoStderrMarker = "echo-command-diagnostic"

type dispatchPayload struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

type multiFlagPayload struct {
	Tags        []string `json:"tags"`
	Names       []string `json:"names"`
	Counts      []int    `json:"counts"`
	LargeID     int64    `json:"large_id"`
	LargeUintID uint64   `json:"large_uint_id"`
}

// dispatchTestRoot builds a command tree exercising the dispatch paths: an
// "echo" leaf (flag mapping, mode reporting, a stderr diagnostic for
// stream-separation tests), a "fail" leaf (ax.Error envelope, exit 2), and a
// "boom" leaf that panics (panic-recovery).
func dispatchTestRoot() *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "root", RunE: func(cmd *cobra.Command, _ []string) error {
		return contract.WriteJSON(cmd.OutOrStdout(), contract.NewEnvelope(cmd.Context(), dispatchPayload{Name: "root"}))
	}}

	var name string
	echo := &cobra.Command{Use: "echo", Short: "echo", RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintln(cmd.ErrOrStderr(), echoStderrMarker)
		mode, _ := contract.ModeFromContext(cmd.Context())
		return contract.WriteJSON(cmd.OutOrStdout(), contract.NewEnvelope(cmd.Context(), dispatchPayload{
			Name: name,
			Mode: mode.String(),
		}))
	}}
	echo.Flags().StringVar(&name, "name", "anon", "name to echo")
	root.AddCommand(echo)

	root.AddCommand(&cobra.Command{Use: "fail", Short: "fail", RunE: func(cmd *cobra.Command, _ []string) error {
		return contract.NewError(cmd.Context(), "demo_failure", "intentional failure",
			contract.WithErrorExitCode(contract.ExitValidation))
	}})

	root.AddCommand(&cobra.Command{Use: "boom", Short: "boom", RunE: func(*cobra.Command, []string) error {
		panic("kaboom")
	}})

	return root
}

func multiFlagRoot() *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "root", RunE: noopRunE}

	var tags []string
	var names []string
	var counts []int
	var largeID int64
	var largeUintID uint64
	cmd := &cobra.Command{Use: "multi", Short: "multi", RunE: func(cmd *cobra.Command, _ []string) error {
		return contract.WriteJSON(cmd.OutOrStdout(), contract.NewEnvelope(cmd.Context(), multiFlagPayload{
			Tags:        tags,
			Names:       names,
			Counts:      counts,
			LargeID:     largeID,
			LargeUintID: largeUintID,
		}))
	}}
	cmd.Flags().StringSliceVar(&tags, "tags", []string{"default-tag"}, "tags")
	cmd.Flags().StringArrayVar(&names, "names", []string{"default-name"}, "names")
	cmd.Flags().IntSliceVar(&counts, "counts", []int{7}, "counts")
	cmd.Flags().Int64Var(&largeID, "large-id", 0, "large ID")
	cmd.Flags().Uint64Var(&largeUintID, "large-uint-id", 0, "large unsigned ID")
	root.AddCommand(cmd)

	return root
}

func callRequest(name string, args map[string]any) *sdk.CallToolRequest {
	var raw json.RawMessage
	if args != nil {
		raw, _ = json.Marshal(args)
	}
	return &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: name, Arguments: raw}}
}

func mustCall(t *testing.T, d *dispatcher, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	res, err := d.handle(context.Background(), callRequest(name, args))
	if err != nil {
		t.Fatalf("handle returned a protocol error (must be nil): %v", err)
	}
	if res == nil {
		t.Fatal("handle returned nil result")
	}
	return res
}

func resultText(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("want exactly 1 content block, got %d", len(res.Content))
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *mcp.TextContent", res.Content[0])
	}
	return text.Text
}

func decodeEnvelope(t *testing.T, payload string) contract.Envelope[dispatchPayload] {
	t.Helper()
	var env contract.Envelope[dispatchPayload]
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		t.Fatalf("decode envelope from %q: %v", payload, err)
	}
	return env
}

func decodeMultiFlagEnvelope(t *testing.T, payload string) contract.Envelope[multiFlagPayload] {
	t.Helper()
	var env contract.Envelope[multiFlagPayload]
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		t.Fatalf("decode multi-flag envelope from %q: %v", payload, err)
	}
	return env
}

func decodeErrorEnvelope(t *testing.T, payload string) contract.Error {
	t.Helper()
	var envelope contract.Error
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatalf("decode error envelope from %q: %v", payload, err)
	}
	return envelope
}

func mustMultiCall(t *testing.T, d *dispatcher, args map[string]any) contract.Envelope[multiFlagPayload] {
	t.Helper()
	res := mustCall(t, d, "demo-multi", args)
	if res.IsError {
		t.Fatalf("unexpected IsError result: %s", resultText(t, res))
	}
	return decodeMultiFlagEnvelope(t, resultText(t, res))
}

// TestDispatchSuccess asserts a tool call maps arguments to flags, forces
// machine mode (FR-026), and returns the command's verbatim stdout payload
// (FR-007/008, C-5/C-6/C-11).
func TestDispatchSuccess(t *testing.T) {
	d := newTestDispatcher(dispatchTestRoot())

	res := mustCall(t, d, "demo-echo", map[string]any{"name": "Ada"})
	if res.IsError {
		t.Fatalf("unexpected IsError result: %s", resultText(t, res))
	}
	env := decodeEnvelope(t, resultText(t, res))
	if env.Data.Name != "Ada" {
		t.Errorf("data.name = %q, want %q", env.Data.Name, "Ada")
	}
	if env.Data.Mode != string(contract.ModeJSON) {
		t.Errorf("data.mode = %q, want %q (machine mode must be forced)", env.Data.Mode, contract.ModeJSON)
	}
}

// TestDispatchForcesMachineModeOverHumanArg asserts a format=human argument is
// normalized to machine mode (FR-026, C-11).
func TestDispatchForcesMachineModeOverHumanArg(t *testing.T) {
	d := newTestDispatcher(dispatchTestRoot())

	res := mustCall(t, d, "demo-echo", map[string]any{"name": "x", "format": "human"})
	env := decodeEnvelope(t, resultText(t, res))
	if env.Data.Mode != string(contract.ModeJSON) {
		t.Errorf("data.mode = %q, want %q", env.Data.Mode, contract.ModeJSON)
	}
}

// TestDispatchAcceptsJSONArrayArgumentsForMultiValueFlags asserts MCP clients
// can send JSON arrays for Cobra slice/array flags and those values map to the
// command without lossy string encoding (FR-007, C-5).
func TestDispatchAcceptsJSONArrayArgumentsForMultiValueFlags(t *testing.T) {
	d := newTestDispatcher(multiFlagRoot())

	env := mustMultiCall(t, d, map[string]any{
		"tags":   []any{"red", "blue"},
		"names":  []any{"Ada", "Grace"},
		"counts": []any{1, 2, 3},
	})
	if !reflect.DeepEqual(env.Data.Tags, []string{"red", "blue"}) {
		t.Errorf("tags = %#v, want %#v", env.Data.Tags, []string{"red", "blue"})
	}
	if !reflect.DeepEqual(env.Data.Names, []string{"Ada", "Grace"}) {
		t.Errorf("names = %#v, want %#v", env.Data.Names, []string{"Ada", "Grace"})
	}
	if !reflect.DeepEqual(env.Data.Counts, []int{1, 2, 3}) {
		t.Errorf("counts = %#v, want %#v", env.Data.Counts, []int{1, 2, 3})
	}
}

// TestDispatchResetsMultiValueFlagsBetweenCalls asserts a reused Cobra tree
// restores slice/array defaults and does not leak state across MCP calls
// (FR-021, INV-5).
func TestDispatchResetsMultiValueFlagsBetweenCalls(t *testing.T) {
	d := newTestDispatcher(multiFlagRoot())

	first := mustMultiCall(t, d, map[string]any{
		"tags":   []any{"first"},
		"names":  []any{"first-name"},
		"counts": []any{1},
	})
	if !reflect.DeepEqual(first.Data.Tags, []string{"first"}) {
		t.Fatalf("first tags = %#v, want %#v", first.Data.Tags, []string{"first"})
	}

	defaulted := mustMultiCall(t, d, nil)
	if !reflect.DeepEqual(defaulted.Data.Tags, []string{"default-tag"}) {
		t.Errorf("defaulted tags = %#v, want default only", defaulted.Data.Tags)
	}
	if !reflect.DeepEqual(defaulted.Data.Names, []string{"default-name"}) {
		t.Errorf("defaulted names = %#v, want default only", defaulted.Data.Names)
	}
	if !reflect.DeepEqual(defaulted.Data.Counts, []int{7}) {
		t.Errorf("defaulted counts = %#v, want default only", defaulted.Data.Counts)
	}

	second := mustMultiCall(t, d, map[string]any{
		"tags":   []any{"second"},
		"names":  []any{"second-name"},
		"counts": []any{2},
	})
	if !reflect.DeepEqual(second.Data.Tags, []string{"second"}) {
		t.Errorf("second tags = %#v, want no leaked first/default values", second.Data.Tags)
	}
	if !reflect.DeepEqual(second.Data.Names, []string{"second-name"}) {
		t.Errorf("second names = %#v, want no leaked first/default values", second.Data.Names)
	}
	if !reflect.DeepEqual(second.Data.Counts, []int{2}) {
		t.Errorf("second counts = %#v, want no leaked first/default values", second.Data.Counts)
	}
}

// TestDispatchPreservesLargeIntegerArguments asserts JSON numbers are not
// materialized as float64 before reaching pflag, preserving int64 IDs above
// JavaScript's exact integer range (Principle II).
func TestDispatchPreservesLargeIntegerArguments(t *testing.T) {
	d := newTestDispatcher(multiFlagRoot())

	const large = int64(9007199254740993)
	const largeUint = uint64(9007199254740993)
	env := mustMultiCall(t, d, map[string]any{
		"large-id":      large,
		"large-uint-id": largeUint,
	})
	if env.Data.LargeID != large {
		t.Errorf("large_id = %d, want %d", env.Data.LargeID, large)
	}
	if env.Data.LargeUintID != largeUint {
		t.Errorf("large_uint_id = %d, want %d", env.Data.LargeUintID, largeUint)
	}
}

// TestDispatchValidationErrors covers unknown tools, unknown arguments, and
// type-mismatched arguments — each returns IsError with a validation envelope
// and never a protocol error (FR-012, C-8).
func TestDispatchValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "unknown tool", tool: "demo-nope", args: nil},
		{name: "unknown argument", tool: "demo-echo", args: map[string]any{"bogus": "x"}},
		{name: "dry-run not a bool", tool: "demo-echo", args: map[string]any{"dry-run": "yes"}},
		{name: "idempotency-key not a string", tool: "demo-echo", args: map[string]any{"idempotency-key": 7.0}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newTestDispatcher(dispatchTestRoot())
			res := mustCall(t, d, tc.tool, tc.args)
			if !res.IsError {
				t.Fatalf("expected IsError, got success: %s", resultText(t, res))
			}
			envelope := decodeErrorEnvelope(t, resultText(t, res))
			if envelope.ErrorCode != "validation_error" {
				t.Errorf("error_code = %q, want %q", envelope.ErrorCode, "validation_error")
			}
		})
	}
}

// TestDispatchValidationErrorsContinueTraceContext asserts boundary validation
// failures still start the tools/call span and build the ax.Error envelope from
// the continued W3C trace context (FR-025, C-8/C-18).
func TestDispatchValidationErrorsContinueTraceContext(t *testing.T) {
	d := newTestDispatcher(dispatchTestRoot())
	const traceID = "0af7651916cd43dd8448eb211c80319c"
	req := callRequest("demo-nope", nil)
	req.Params.Meta = sdk.Meta{
		traceParentKey: "00-" + traceID + "-b7ad6b7169203331-01",
	}

	res, err := d.handle(context.Background(), req)
	if err != nil {
		t.Fatalf("handle returned a protocol error (must be nil): %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for unknown tool")
	}
	envelope := decodeErrorEnvelope(t, resultText(t, res))
	if envelope.TraceID != traceID {
		t.Errorf("trace_id = %q, want %q", envelope.TraceID, traceID)
	}
}

// TestDispatchFailureKeepsServing asserts a non-zero-exit command returns the
// ax.Error envelope with IsError, and the dispatcher keeps serving subsequent
// calls (FR-009/FR-010, C-7, INV-4).
func TestDispatchFailureKeepsServing(t *testing.T) {
	d := newTestDispatcher(dispatchTestRoot())

	failRes := mustCall(t, d, "demo-fail", nil)
	if !failRes.IsError {
		t.Fatalf("expected IsError for failing command")
	}
	if envelope := decodeErrorEnvelope(t, resultText(t, failRes)); envelope.ErrorCode != "demo_failure" {
		t.Errorf("error_code = %q, want %q", envelope.ErrorCode, "demo_failure")
	}

	okRes := mustCall(t, d, "demo-echo", map[string]any{"name": "after-failure"})
	if okRes.IsError {
		t.Fatalf("server stopped serving after a failure: %s", resultText(t, okRes))
	}
	if env := decodeEnvelope(t, resultText(t, okRes)); env.Data.Name != "after-failure" {
		t.Errorf("data.name = %q, want %q", env.Data.Name, "after-failure")
	}
}

// TestDispatchRecoversPanic asserts a panicking command is recovered into an
// internal_error result and never crashes the server (FR-010, C-9).
func TestDispatchRecoversPanic(t *testing.T) {
	d := newTestDispatcher(dispatchTestRoot())

	res := mustCall(t, d, "demo-boom", nil)
	if !res.IsError {
		t.Fatalf("expected IsError for panicking command")
	}
	if envelope := decodeErrorEnvelope(t, resultText(t, res)); envelope.ErrorCode != "internal_error" {
		t.Errorf("error_code = %q, want %q", envelope.ErrorCode, "internal_error")
	}
	if okRes := mustCall(t, d, "demo-echo", map[string]any{"name": "alive"}); okRes.IsError {
		t.Fatalf("server did not survive a panic: %s", resultText(t, okRes))
	}
}

// TestDispatchAgentSafetyPassthrough asserts dry-run and idempotency-key flow
// through to the command's envelope: dry_run is surfaced and a provided key is
// preserved while an absent key is auto-generated (FR-011, C-10).
func TestDispatchAgentSafetyPassthrough(t *testing.T) {
	d := newTestDispatcher(dispatchTestRoot())

	dryRes := mustCall(t, d, "demo-echo", map[string]any{"name": "x", "dry-run": true})
	if env := decodeEnvelope(t, resultText(t, dryRes)); !env.Meta.DryRun {
		t.Errorf("meta.dry_run = false, want true")
	}

	keyed := mustCall(t, d, "demo-echo", map[string]any{"name": "x", "idempotency-key": "my-key"})
	if env := decodeEnvelope(t, resultText(t, keyed)); env.Meta.IdempotencyKey != "my-key" {
		t.Errorf("meta.idempotency_key = %q, want %q", env.Meta.IdempotencyKey, "my-key")
	}

	auto := mustCall(t, d, "demo-echo", map[string]any{"name": "x"})
	if env := decodeEnvelope(t, resultText(t, auto)); env.Meta.IdempotencyKey == "" {
		t.Errorf("meta.idempotency_key should be auto-generated when absent")
	}
}

// requiredFlagRoot builds a tree with a "deploy" leaf carrying a required
// "target" flag, exercising the required-argument validation path.
func requiredFlagRoot() *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "root", RunE: noopRunE}

	var target string
	deploy := &cobra.Command{Use: "deploy", Short: "deploy", RunE: func(cmd *cobra.Command, _ []string) error {
		return contract.WriteJSON(cmd.OutOrStdout(), contract.NewEnvelope(cmd.Context(), dispatchPayload{Name: target}))
	}}
	deploy.Flags().StringVar(&target, "target", "", "deploy target")
	_ = deploy.MarkFlagRequired("target")
	root.AddCommand(deploy)

	return root
}

// panicOnReplaceSlice is a custom pflag.SliceValue whose Replace panics when
// given caller-supplied values. It models a third-party flag type that panics
// from inside the dispatcher's slice-assignment step — outside the command body
// — so a test can assert the panic boundary covers that step too. Reset (an
// empty Replace) does not panic, so a sibling command can still be served.
type panicOnReplaceSlice struct{ values []string }

func (v *panicOnReplaceSlice) String() string        { return "[" + strings.Join(v.values, ",") + "]" }
func (v *panicOnReplaceSlice) Set(s string) error    { v.values = append(v.values, s); return nil }
func (v *panicOnReplaceSlice) Type() string          { return "panicSlice" }
func (v *panicOnReplaceSlice) Append(s string) error { v.values = append(v.values, s); return nil }
func (v *panicOnReplaceSlice) GetSlice() []string    { return v.values }
func (v *panicOnReplaceSlice) Replace(values []string) error {
	if len(values) > 0 {
		panic("panic-on-replace")
	}
	v.values = nil
	return nil
}

// poisonAssignmentRoot builds a tree with a "poison" leaf whose slice flag
// panics on assignment and a benign "ok" leaf used to prove the server survives.
func poisonAssignmentRoot() *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "root", RunE: noopRunE}

	poison := &cobra.Command{Use: "poison", Short: "poison", RunE: noopRunE}
	poison.Flags().Var(&panicOnReplaceSlice{}, "vals", "panics when assigned a value")
	root.AddCommand(poison)

	root.AddCommand(&cobra.Command{Use: "ok", Short: "ok", RunE: func(cmd *cobra.Command, _ []string) error {
		return contract.WriteJSON(cmd.OutOrStdout(), contract.NewEnvelope(cmd.Context(), dispatchPayload{Name: "ok"}))
	}})

	return root
}

// TestDispatchInvalidScalarFlagValueIsValidationError asserts a mistyped scalar
// argument (a non-numeric value for an integer flag) is rejected by Cobra's flag
// parser and classified as a validation error (exit 2), matching the
// slice-argument path and the argument-validation contract (C-8). A Cobra
// flag-parse failure must not fall through to internal_error (exit 1).
func TestDispatchInvalidScalarFlagValueIsValidationError(t *testing.T) {
	d := newTestDispatcher(multiFlagRoot())

	res := mustCall(t, d, "demo-multi", map[string]any{"large-id": "not-an-int"})
	if !res.IsError {
		t.Fatalf("expected IsError for an invalid scalar flag value, got: %s", resultText(t, res))
	}
	if env := decodeErrorEnvelope(t, resultText(t, res)); env.ErrorCode != "validation_error" {
		t.Errorf("error_code = %q, want %q (a Cobra flag-parse failure must map to exit 2)",
			env.ErrorCode, "validation_error")
	}
}

// TestDispatchMissingRequiredFlagIsValidationError asserts omitting a required
// flag yields a validation error (exit 2), not internal_error (exit 1), so an
// agent learns its input is fixable (C-8).
func TestDispatchMissingRequiredFlagIsValidationError(t *testing.T) {
	d := newTestDispatcher(requiredFlagRoot())

	res := mustCall(t, d, "demo-deploy", nil)
	if !res.IsError {
		t.Fatalf("expected IsError for a missing required flag, got: %s", resultText(t, res))
	}
	if env := decodeErrorEnvelope(t, resultText(t, res)); env.ErrorCode != "validation_error" {
		t.Errorf("error_code = %q, want %q", env.ErrorCode, "validation_error")
	}
}

// TestDispatchRequiredFlagSatisfied asserts a provided required flag dispatches
// normally and reaches the command body.
func TestDispatchRequiredFlagSatisfied(t *testing.T) {
	d := newTestDispatcher(requiredFlagRoot())

	res := mustCall(t, d, "demo-deploy", map[string]any{"target": "prod"})
	if res.IsError {
		t.Fatalf("unexpected IsError result: %s", resultText(t, res))
	}
	if env := decodeEnvelope(t, resultText(t, res)); env.Data.Name != "prod" {
		t.Errorf("data.name = %q, want %q", env.Data.Name, "prod")
	}
}

// TestDispatchRecoversPanicInFlagAssignment asserts a panic raised while
// applying a slice argument — before the command body runs, outside Cobra's
// execution — is recovered into an internal_error result and the server keeps
// serving (FR-010, C-9). It guards the panic boundary around flag reset and
// assignment, not just command execution.
func TestDispatchRecoversPanicInFlagAssignment(t *testing.T) {
	d := newTestDispatcher(poisonAssignmentRoot())

	res := mustCall(t, d, "demo-poison", map[string]any{"vals": []any{"boom"}})
	if !res.IsError {
		t.Fatalf("expected IsError for a panicking flag assignment, got: %s", resultText(t, res))
	}
	if env := decodeErrorEnvelope(t, resultText(t, res)); env.ErrorCode != "internal_error" {
		t.Errorf("error_code = %q, want %q", env.ErrorCode, "internal_error")
	}
	if okRes := mustCall(t, d, "demo-ok", nil); okRes.IsError {
		t.Fatalf("server did not survive a flag-assignment panic: %s", resultText(t, okRes))
	}
}

// TestDispatchStreamSeparation asserts a command's stderr is captured to the
// server's stderr stream and never leaks into the tool result content
// (FR-013/FR-014, SC-005, C-12/C-13).
func TestDispatchStreamSeparation(t *testing.T) {
	var stderr bytes.Buffer
	root := dispatchTestRoot()
	d := newDispatcher(context.Background(), root, Config{
		Version:    testServerVersion,
		ServerName: root.Name(),
		Stderr:     &stderr,
	})

	text := resultText(t, mustCall(t, d, "demo-echo", map[string]any{"name": "x"}))
	if strings.Contains(text, echoStderrMarker) {
		t.Errorf("command stderr leaked into the tool result content:\n%s", text)
	}
	if !strings.Contains(stderr.String(), echoStderrMarker) {
		t.Errorf("command stderr was not forwarded to the server stderr; got: %q", stderr.String())
	}
}
