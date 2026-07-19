package mcpserver

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/id"
	"github.com/rshade/ax-go/internal/cli"
	internalschema "github.com/rshade/ax-go/internal/schema"
	"github.com/rshade/ax-go/schema"
)

// The dispatcher forces machine mode and threads dry-run/idempotency-key back
// through the same persistent flags ax.Execute installs, so both resolve from
// the single source of truth in internal/cli rather than re-declaring literals.
const (
	formatFlagName         = cli.FlagFormat
	dryRunFlagName         = cli.FlagDryRun
	idempotencyKeyFlagName = cli.FlagIdempotencyKey
)

// dispatcher maps tools/call requests onto isolated invocations of the shared
// Cobra command tree.
//
// Dispatch is serialized by mu: each call resets the tree's flags, binds its
// own stdout/stderr buffers and per-call context, and runs to completion before
// the next call acquires the lock. Concurrent calls (e.g. over HTTP) therefore
// observe no shared mutable command/flag state and run -race-clean (FR-021,
// INV-5). Per-call output buffering keeps command stdout off the protocol
// channel (FR-013/FR-014).
type dispatcher struct {
	serveCtx      context.Context
	root          *cobra.Command
	serverName    string
	version       string
	tools         []schema.MCPTool
	toolNames     map[string]struct{}
	targets       map[string]*cobra.Command
	sliceDefaults map[*pflag.Flag][]string

	mu sync.Mutex

	stderrMu sync.Mutex
	stderr   io.Writer
}

// callConfig holds the agent-safety/mode values resolved from a call's
// arguments before the command is invoked.
type callConfig struct {
	mode           contract.Mode
	dryRun         bool
	idempotencyKey string
}

type flagAssignment struct {
	flag   *pflag.Flag
	values []string
}

// newDispatcher prepares root for repeated isolated invocation: it ensures the
// agent-safety persistent flags exist (so a directly-embedded Serve behaves
// like one mounted under ax.Execute), silences Cobra usage/error printing (it
// is captured and surfaced as a structured result instead), and snapshots the
// callable tool set.
func newDispatcher(ctx context.Context, root *cobra.Command, cfg Config) *dispatcher {
	ensurePersistentFlags(root)
	root.SilenceUsage = true
	root.SilenceErrors = true

	tools, targets := discoverTools(root)
	names := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		names[tool.Name] = struct{}{}
	}
	d := &dispatcher{
		serveCtx:      ctx,
		root:          root,
		serverName:    cfg.ServerName,
		version:       cfg.Version,
		tools:         tools,
		toolNames:     names,
		targets:       targets,
		sliceDefaults: snapshotSliceDefaults(root),
		stderr:        cfg.Stderr,
	}
	// Reclassify Cobra flag-parse failures (e.g. a non-numeric value for an int
	// flag) as validation errors (exit 2), matching the dispatcher's own
	// slice-argument validation and the argument-validation contract (C-8). Without
	// this they would surface untyped from ExecuteContext and fall through to
	// internal_error (exit 1), misreporting fixable bad input as a server fault.
	root.SetFlagErrorFunc(func(cmd *cobra.Command, ferr error) error {
		return d.validationError(cmd.Context(), ferr.Error())
	})
	return d
}

// ensurePersistentFlags adds the agent-safety/output persistent flags unless
// they already exist (ax.Execute installs the same set). They must exist so the
// engine can force machine mode and pass through dry-run/idempotency-key during
// dispatch.
func ensurePersistentFlags(root *cobra.Command) {
	cli.EnsurePersistentStringFlag(root, formatFlagName, "", "output format: json or human")
	cli.EnsurePersistentBoolFlag(root, dryRunFlagName, false, "emit the envelope without side effects")
	cli.EnsurePersistentStringFlag(
		root,
		idempotencyKeyFlagName,
		"",
		"opaque key used to prevent duplicate-create retries",
	)
}

// handle is the SDK ToolHandler for every registered tool. It never returns a
// non-nil error (which the SDK would treat as a protocol error): command-level
// failures map to CallToolResult{IsError:true} so the server keeps serving
// (D5, C-7/C-8/C-9).
func (d *dispatcher) handle(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
	name := ""
	if req != nil && req.Params != nil {
		name = req.Params.Name
	}
	callCtx, cancel := d.newCallContext(ctx)
	defer cancel()
	callCtx, span := startCallSpan(callCtx, req, name)
	defer span.End()

	if req == nil || req.Params == nil {
		return d.errorResult(callCtx, d.validationError(callCtx, "malformed tool call request")), nil
	}
	if _, ok := d.toolNames[name]; !ok {
		return d.errorResult(callCtx, d.validationError(callCtx, fmt.Sprintf("unknown tool %q", name))), nil
	}

	args, err := decodeArguments(req.Params.Arguments)
	if err != nil {
		return d.errorResult(callCtx, d.validationError(callCtx, fmt.Sprintf("invalid arguments: %v", err))), nil
	}

	stdout, runErr := d.dispatch(callCtx, name, args)
	if runErr != nil {
		return d.errorResult(callCtx, runErr), nil
	}
	return successResult(stdout), nil
}

// dispatch serializes everything that reads or mutates the shared Cobra tree —
// command lookup, flag-set inspection, argument assembly, and execution — under
// a single lock. Cobra lazily merges inherited flags into a command's flag set
// on inspection, so target resolution and argument mapping mutate shared state
// just as execution does; running them concurrently would race (FR-021, C-20).
func (d *dispatcher) dispatch(ctx context.Context, name string, args map[string]any) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	target, findErr := d.resolveTarget(name)
	if findErr != nil {
		return nil, d.validationError(ctx, findErr.Error())
	}
	argv, assignments, cc, buildErr := d.buildArgs(ctx, target, name, args)
	if buildErr != nil {
		return nil, buildErr
	}
	return d.execute(applyCallConfig(ctx, cc), argv, assignments)
}

// newCallContext derives a per-call context that is canceled when either the
// request context or the server's serve context is done, so in-flight calls
// observe cancellation promptly on shutdown (C-19/C-20).
func (d *dispatcher) newCallContext(reqCtx context.Context) (context.Context, context.CancelFunc) {
	callCtx, cancel := context.WithCancel(reqCtx)
	if d.serveCtx != nil {
		stop := context.AfterFunc(d.serveCtx, cancel)
		return callCtx, func() {
			stop()
			cancel()
		}
	}
	return callCtx, cancel
}

// resolveTarget returns the Cobra command a discovered tool name maps to. The
// mapping is snapshotted at discovery because tool names join path segments
// with "-", which is not safely invertible (command names may themselves
// contain hyphens, e.g. mcp-server).
func (d *dispatcher) resolveTarget(name string) (*cobra.Command, error) {
	target, ok := d.targets[name]
	if !ok {
		return nil, fmt.Errorf("resolve tool %q: not in the discovered tool set", name)
	}
	return target, nil
}

// buildArgs validates the call arguments against target's flags and assembles
// the isolated argument vector. Output mode is forced to machine/JSON (FR-026);
// dry-run and idempotency-key flow through as agent-safety primitives (FR-011),
// with the idempotency key auto-generated (UUID v4) when absent.
func (d *dispatcher) buildArgs(
	ctx context.Context,
	target *cobra.Command,
	name string,
	args map[string]any,
) ([]string, []flagAssignment, callConfig, *contract.Error) {
	allowed := acceptedFlags(target)
	cc := callConfig{mode: contract.ModeJSON}

	keys := make([]string, 0, len(args))
	for key := range args {
		keys = append(keys, key)
	}
	// Sort before validating so a call with several invalid arguments always
	// reports the lexicographically-first offender rather than a random
	// map-iteration winner. The assembled flag vector is sorted again below to
	// make the argv itself deterministic once the forced flags are appended.
	sort.Strings(keys)

	var flagArgs []string
	var assignments []flagAssignment
	// setFlags tracks flag names that receive a value, so missing required flags
	// are reported as validation errors (exit 2) before dispatch rather than
	// surfacing untyped from Cobra as internal_error (exit 1).
	setFlags := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		flag, exists := allowed[key]
		if !exists {
			return nil, nil, cc, d.validationError(ctx, fmt.Sprintf("unknown argument %q for tool %q", key, name))
		}
		value := args[key]
		handled, configErr := d.applyCallConfigArg(ctx, &cc, key, value)
		if configErr != nil {
			return nil, nil, cc, configErr
		}
		if handled {
			setFlags[key] = struct{}{}
			continue
		}
		wasSet, flagErr := d.appendConvertedFlagArg(ctx, key, flag, value, &flagArgs, &assignments)
		if flagErr != nil {
			return nil, nil, cc, flagErr
		}
		if wasSet {
			setFlags[key] = struct{}{}
		}
	}

	// The engine always injects the agent-safety/output flags, so they count as
	// satisfied when checking required flags.
	setFlags[formatFlagName] = struct{}{}
	setFlags[dryRunFlagName] = struct{}{}
	setFlags[idempotencyKeyFlagName] = struct{}{}
	if missing := missingRequiredFlag(allowed, setFlags); missing != "" {
		return nil, nil, cc, d.validationError(ctx,
			fmt.Sprintf("required argument %q not provided for tool %q", missing, name))
	}

	flagArgs = append(flagArgs, "--"+formatFlagName+"="+string(contract.ModeJSON))
	if cc.dryRun {
		flagArgs = append(flagArgs, "--"+dryRunFlagName+"=true")
	}
	if cc.idempotencyKey == "" {
		cc.idempotencyKey = id.NewIdempotencyKey()
	}
	flagArgs = append(flagArgs, "--"+idempotencyKeyFlagName+"="+cc.idempotencyKey)
	sort.Strings(flagArgs)

	return append(commandPathArgs(d.root, target), flagArgs...), assignments, cc, nil
}

func (d *dispatcher) applyCallConfigArg(
	ctx context.Context,
	cc *callConfig,
	key string,
	value any,
) (bool, *contract.Error) {
	switch key {
	case formatFlagName:
		return true, nil // machine mode is forced below regardless of the requested value
	case dryRunFlagName:
		boolVal, isBool := value.(bool)
		if !isBool {
			return true, d.validationError(ctx, fmt.Sprintf("argument %q must be a boolean", key))
		}
		cc.dryRun = boolVal
		return true, nil
	case idempotencyKeyFlagName:
		strVal, isString := value.(string)
		if !isString {
			return true, d.validationError(ctx, fmt.Sprintf("argument %q must be a string", key))
		}
		cc.idempotencyKey = strVal
		return true, nil
	default:
		return false, nil
	}
}

// appendConvertedFlagArg converts one argument onto target's flags, returning
// whether the flag received a value (false for a skipped null) so the caller can
// enforce required flags.
func (d *dispatcher) appendConvertedFlagArg(
	ctx context.Context,
	key string,
	flag *pflag.Flag,
	value any,
	flagArgs *[]string,
	assignments *[]flagAssignment,
) (bool, *contract.Error) {
	if _, isSlice := flag.Value.(pflag.SliceValue); isSlice {
		values, include, convErr := stringifyArgList(value, flag.Value.Type())
		if convErr != nil {
			return false, d.validationError(ctx, fmt.Sprintf("argument %q: %v", key, convErr))
		}
		if include {
			*assignments = append(*assignments, flagAssignment{flag: flag, values: values})
		}
		return include, nil
	}

	str, include, convErr := stringifyArg(value)
	if convErr != nil {
		return false, d.validationError(ctx, fmt.Sprintf("argument %q: %v", key, convErr))
	}
	if include {
		*flagArgs = append(*flagArgs, "--"+key+"="+str)
	}
	return include, nil
}

// missingRequiredFlag returns the name of a required flag on the target command
// that the call did not supply, or "" when every required flag is satisfied.
// Flags are scanned in sorted order so the reported offender is deterministic.
func missingRequiredFlag(allowed map[string]*pflag.Flag, setFlags map[string]struct{}) string {
	names := make([]string, 0, len(allowed))
	for name := range allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !internalschema.IsRequiredFlag(allowed[name]) {
			continue
		}
		if _, ok := setFlags[name]; !ok {
			return name
		}
	}
	return ""
}

// execute runs the prepared argument vector against the shared command tree in
// isolation. The caller (dispatch) holds d.mu, so resetting flags, binding the
// per-call buffers/context, and re-executing the tree never overlap across
// calls. The entire per-call invocation — flag reset, slice assignment, and
// command execution — runs under a single panic boundary, so a panic in any
// adopter-defined flag Value (Set/Replace) or command body is recovered into an
// internal error rather than crashing the server (C-9, FR-010). Captured command
// stderr is forwarded to the server's stderr, never the protocol channel.
func (d *dispatcher) execute(ctx context.Context, argv []string, assignments []flagAssignment) ([]byte, error) {
	var outBuf bytes.Buffer
	runErr := d.runRecovered(ctx, argv, assignments, &outBuf)
	return outBuf.Bytes(), runErr
}

// runRecovered performs the panic-bounded per-call work — flag reset, slice
// assignment, and command execution — writing command stdout into outBuf. A
// panic anywhere in this sequence, including an adopter-defined flag Value's
// Set/Replace (reached during reset/assignment) or the command body, is
// recovered into an internal error so it never crashes the server (C-9, FR-010);
// the partial output buffer is discarded on a panic.
func (d *dispatcher) runRecovered(
	ctx context.Context,
	argv []string,
	assignments []flagAssignment,
	outBuf *bytes.Buffer,
) (runErr error) {
	defer func() {
		if r := recover(); r != nil {
			outBuf.Reset()
			runErr = contract.NewError(ctx, "internal_error",
				fmt.Sprintf("recovered panic during tool dispatch: %v", r),
				contract.WithErrorExitCode(contract.ExitInternal))
		}
	}()

	resetCommandTree(ctx, d.root, d.sliceDefaults)
	if err := applyFlagAssignments(assignments); err != nil {
		return d.validationError(ctx, err.Error())
	}

	var errBuf bytes.Buffer
	d.root.SetArgs(argv)
	d.root.SetIn(bytes.NewReader(nil))
	d.root.SetOut(outBuf)
	d.root.SetErr(&errBuf)

	runErr = d.root.ExecuteContext(ctx)

	d.forwardStderr(errBuf.Bytes())
	d.root.SetArgs(nil)
	return runErr
}

// forwardStderr writes captured command stderr to the server's stderr stream
// under a lock so concurrent dispatch never interleaves output lines.
func (d *dispatcher) forwardStderr(b []byte) {
	if len(b) == 0 || d.stderr == nil {
		return
	}
	d.stderrMu.Lock()
	defer d.stderrMu.Unlock()
	_, _ = d.stderr.Write(b)
}

// successResult wraps verbatim command stdout bytes as a single text content
// block (FR-008, C-6) — no re-parse or re-serialize, preserving byte-identical
// determinism.
func successResult(stdout []byte) *sdk.CallToolResult {
	return &sdk.CallToolResult{
		Content: []sdk.Content{&sdk.TextContent{Text: string(stdout)}},
	}
}

// errorResult maps a dispatch/command error to CallToolResult{IsError:true}
// carrying the ax.Error envelope JSON (FR-009, C-7).
func (d *dispatcher) errorResult(ctx context.Context, err error) *sdk.CallToolResult {
	axErr := d.toAxError(ctx, err)
	payload, marshalErr := json.Marshal(axErr)
	if marshalErr != nil {
		payload = []byte(`{"error_code":"internal_error","message":"failed to marshal error envelope"}`)
	}
	return &sdk.CallToolResult{
		IsError: true,
		Content: []sdk.Content{&sdk.TextContent{Text: string(payload)}},
	}
}

// toAxError normalizes any dispatch error into the ax.Error envelope, attaching
// the server's tool name and version and the active trace ID.
func (d *dispatcher) toAxError(ctx context.Context, err error) *contract.Error {
	var axErr *contract.Error
	if errors.As(err, &axErr) {
		if axErr.Tool == "" {
			axErr.Tool = d.serverName
		}
		if axErr.Version == "" {
			axErr.Version = d.version
		}
		if axErr.TraceID == "" {
			axErr.TraceID = contract.TraceIDFromContext(ctx)
		}
		return axErr
	}
	return contract.NewError(ctx, "internal_error", err.Error(),
		contract.WithErrorTool(d.serverName),
		contract.WithErrorVersion(d.version),
		contract.WithErrorExitCode(contract.ErrorExitCode(err)),
	)
}

// validationError builds a validation envelope (exit 2) for bad input.
func (d *dispatcher) validationError(ctx context.Context, message string) *contract.Error {
	return contract.NewError(ctx, "validation_error", message,
		contract.WithErrorTool(d.serverName),
		contract.WithErrorVersion(d.version),
		contract.WithErrorExitCode(contract.ExitValidation),
	)
}

// applyCallConfig seeds the per-call context with the forced machine mode and
// agent-safety values so directly-embedded servers behave like mounted ones;
// when mounted under ax.Execute, the wrapped pre-run re-derives identical values
// from the flags the engine passes.
func applyCallConfig(ctx context.Context, cc callConfig) context.Context {
	ctx = contract.WithMode(ctx, cc.mode)
	ctx = contract.WithDryRun(ctx, cc.dryRun)
	ctx = contract.WithIdempotencyKey(ctx, cc.idempotencyKey)
	return ctx
}

// decodeArguments unmarshals the raw tool arguments into a flat object. An empty
// payload is a valid no-argument call.
func decodeArguments(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var args map[string]any
	if err := decoder.Decode(&args); err != nil {
		return nil, err
	}
	if args == nil {
		return map[string]any{}, nil
	}
	return args, nil
}

// commandPathArgs returns the subcommand-name path from root to target
// (excluding root): the Cobra argument vector that selects target. It is built
// from parent pointers because the MCP tool name cannot be split back into
// segments (command names may contain the "-" separator themselves).
func commandPathArgs(root, target *cobra.Command) []string {
	var reversed []string
	for cmd := target; cmd != nil && cmd != root; cmd = cmd.Parent() {
		reversed = append(reversed, cmd.Name())
	}
	slices.Reverse(reversed)
	return reversed
}

// acceptedFlags returns the flags accepted by cmd (local and inherited).
func acceptedFlags(cmd *cobra.Command) map[string]*pflag.Flag {
	flags := map[string]*pflag.Flag{}
	add := func(flag *pflag.Flag) {
		if _, ok := flags[flag.Name]; ok {
			return
		}
		flags[flag.Name] = flag
	}
	cmd.NonInheritedFlags().VisitAll(add)
	cmd.InheritedFlags().VisitAll(add)
	return flags
}

func snapshotSliceDefaults(root *cobra.Command) map[*pflag.Flag][]string {
	defaults := map[*pflag.Flag][]string{}
	internalschema.WalkCommands(root, func(cmd *cobra.Command) {
		snapshotFlagSetSliceDefaults(defaults, cmd.Flags())
		snapshotFlagSetSliceDefaults(defaults, cmd.PersistentFlags())
	})
	return defaults
}

func snapshotFlagSetSliceDefaults(defaults map[*pflag.Flag][]string, fs *pflag.FlagSet) {
	if fs == nil {
		return
	}
	fs.VisitAll(func(flag *pflag.Flag) {
		slice, ok := flag.Value.(pflag.SliceValue)
		if !ok {
			return
		}
		defaults[flag] = append([]string(nil), slice.GetSlice()...)
	})
}

// resetCommandTree prepares the shared tree for one isolated invocation: every
// flag is restored to its default with the changed bit cleared, and every
// command's cached context is rebound to ctx. The context rebind is required
// because Cobra propagates a fresh ExecuteContext value to a subcommand only
// when that subcommand's context is nil (command.go: `if cmd.ctx == nil`); on
// the second and later calls the first call's context would otherwise stick,
// leaking that call's mode/dry-run/idempotency metadata into every subsequent
// call.
func resetCommandTree(ctx context.Context, root *cobra.Command, sliceDefaults map[*pflag.Flag][]string) {
	internalschema.WalkCommands(root, func(cmd *cobra.Command) {
		resetFlagSet(cmd.Flags(), sliceDefaults)
		resetFlagSet(cmd.PersistentFlags(), sliceDefaults)
		cmd.SetContext(ctx)
	})
}

func resetFlagSet(fs *pflag.FlagSet, sliceDefaults map[*pflag.Flag][]string) {
	if fs == nil {
		return
	}
	fs.VisitAll(func(flag *pflag.Flag) {
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			defaults := sliceDefaults[flag]
			_ = slice.Replace(append([]string(nil), defaults...))
			flag.Changed = false
			return
		}
		if flag.Changed {
			_ = flag.Value.Set(flag.DefValue)
			flag.Changed = false
		}
	})
}

func applyFlagAssignments(assignments []flagAssignment) error {
	for _, assignment := range assignments {
		slice, ok := assignment.flag.Value.(pflag.SliceValue)
		if !ok {
			return fmt.Errorf("argument %q does not accept multiple values", assignment.flag.Name)
		}
		if err := slice.Replace(assignment.values); err != nil {
			return fmt.Errorf("argument %q: %w", assignment.flag.Name, err)
		}
		assignment.flag.Changed = true
	}
	return nil
}

// stringifyArg renders a JSON-decoded argument value as a flag string. It
// returns ok=false to skip null values and an error for unsupported shapes
// (nested objects/arrays cannot map onto a scalar flag).
func stringifyArg(value any) (string, bool, error) {
	switch v := value.(type) {
	case nil:
		return "", false, nil
	case bool:
		return strconv.FormatBool(v), true, nil
	case string:
		return v, true, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true, nil
	case json.Number:
		return v.String(), true, nil
	default:
		return "", false, fmt.Errorf("unsupported value type %T", value)
	}
}

func stringifyArgList(value any, flagType string) ([]string, bool, error) {
	switch v := value.(type) {
	case nil:
		return nil, false, nil
	case []any:
		values := make([]string, 0, len(v))
		for i, item := range v {
			str, ok, err := stringifyArg(item)
			if err != nil {
				return nil, false, fmt.Errorf("item %d: %w", i, err)
			}
			if !ok {
				return nil, false, fmt.Errorf("item %d must not be null", i)
			}
			values = append(values, str)
		}
		return values, true, nil
	default:
		str, ok, err := stringifyArg(value)
		if err != nil || !ok {
			return nil, ok, err
		}
		values, err := splitScalarSliceArg(str, flagType)
		if err != nil {
			return nil, false, err
		}
		return values, true, nil
	}
}

func splitScalarSliceArg(value, flagType string) ([]string, error) {
	switch flagType {
	case "stringArray":
		return []string{value}, nil
	case "stringSlice":
		if value == "" {
			return []string{}, nil
		}
		reader := csv.NewReader(strings.NewReader(value))
		return reader.Read()
	default:
		return strings.Split(value, ","), nil
	}
}
