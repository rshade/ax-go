package ax

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/rshade/ax-go/internal/cli"
	"github.com/rshade/ax-go/internal/logcore"
	internaltelemetry "github.com/rshade/ax-go/internal/telemetry"
)

// ExecuteOption configures Execute.
type ExecuteOption func(*executeConfig)

type executeConfig struct {
	stdin           io.Reader
	stdout          io.Writer
	stderr          io.Writer
	env             func(string) string
	stdoutIsTTY     *bool
	version         string
	shutdownTimeout time.Duration
	flushFunc       func(context.Context) error
	// shutdownTelemetry stays per-execution so tests can prove its deadline is
	// created after flush without mutable package-level hooks.
	shutdownTelemetry func(context.Context, *Telemetry) error
}

func defaultExecuteTelemetryShutdown(ctx context.Context, telemetry *Telemetry) error {
	return telemetry.Shutdown(ctx)
}

// WithStdin sets the input stream for Cobra.
func WithStdin(r io.Reader) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.stdin = r
	}
}

// WithStdout sets the machine payload output stream.
func WithStdout(w io.Writer) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.stdout = w
	}
}

// WithStderr sets the operational output stream.
func WithStderr(w io.Writer) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.stderr = w
	}
}

// WithEnv sets the environment lookup used by Execute.
func WithEnv(env func(string) string) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.env = env
	}
}

// WithStdoutIsTTY overrides TTY detection, primarily for tests.
func WithStdoutIsTTY(isTTY bool) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.stdoutIsTTY = &isTTY
	}
}

// WithVersion sets the tool version reported in schema and error envelopes.
// When omitted or empty, Execute falls back to ResolveVersion(""), which
// resolves build metadata and is never empty.
func WithVersion(version string) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.version = version
	}
}

// WithTelemetryShutdownTimeout sets the duration used for the independent
// buffered-output flush and OTel shutdown timeout windows. A non-positive
// timeout is normalized to the default budget, matching how StartTelemetry
// resolves its own shutdown budget; a zero window would otherwise hand both
// shutdown paths an already-expired context and silently skip the drain.
func WithTelemetryShutdownTimeout(timeout time.Duration) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.shutdownTimeout = timeout
	}
}

// WithFlushFunc registers flush to run once during Execute shutdown after
// command execution and before telemetry shutdown. Flush receives a fresh
// context bounded by the duration configured with
// WithTelemetryShutdownTimeout. Its timeout window is independent of the
// telemetry shutdown window.
//
// If flush returns an error, Execute writes a control-character-sanitized
// diagnostic to its configured stderr without changing stdout or the command
// exit code. Sanitization is not redaction; flush errors must not contain PII,
// secrets, tokens, or credentials. A nil flush disables the hook. If supplied
// more than once, the last WithFlushFunc option wins.
func WithFlushFunc(flush func(context.Context) error) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.flushFunc = flush
	}
}

// Execute wraps Cobra execution with AX mode resolution, idempotency, schema,
// error-envelope, and telemetry lifecycle behavior. It returns a deterministic
// exit code and leaves process termination to the caller.
//
// The version reported in __schema output and error envelopes comes from
// WithVersion. When WithVersion is not supplied, Execute falls back to
// ResolveVersion("") — link-time injection, then Go build metadata, then the
// "0.0.0-unknown" sentinel — so the version surfaced to agents is never empty.
//
// When the command returns an *Error, Execute normalizes a copy of it (filling
// in trace ID, tool, and version) before writing the envelope to stderr; the
// caller's *Error value is never mutated.
//
// When WithFlushFunc registers a callback, Execute invokes it once with a fresh,
// bounded context after the root command span ends and before telemetry
// shutdown. Flush and telemetry receive independent timeout windows. A flush
// failure is a sanitized stderr diagnostic only and never changes the command's
// stdout or deterministic exit code.
func Execute(ctx context.Context, root *cobra.Command, opts ...ExecuteOption) int {
	cfg := executeConfig{
		stdin:             os.Stdin,
		stdout:            os.Stdout,
		stderr:            os.Stderr,
		env:               os.Getenv,
		shutdownTimeout:   defaultTelemetryShutdownTimeout,
		shutdownTelemetry: defaultExecuteTelemetryShutdown,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.env == nil {
		cfg.env = os.Getenv
	}
	if cfg.stderr == nil {
		cfg.stderr = os.Stderr
	}
	if cfg.version == "" {
		cfg.version = ResolveVersion("")
	}
	// Normalize once, before StartTelemetry and both shutdown defers read it, so
	// a non-positive option value cannot hand the flush and telemetry windows an
	// already-expired context.
	if cfg.shutdownTimeout <= 0 {
		cfg.shutdownTimeout = defaultTelemetryShutdownTimeout
	}
	// Serialize all writes to stderr. OTel exporters, zerolog hooks, and the
	// shutdown diagnostic may write concurrently; a mutex writer prevents
	// interleaved or torn output lines.
	cfg.stderr = internaltelemetry.NewLockedWriter(cfg.stderr)

	ctx, telemetry, _ := StartTelemetry(
		ctx,
		WithTelemetryEnv(cfg.env),
		WithTelemetryStderr(cfg.stderr),
		WithTelemetryServiceName(root.Name()),
		WithTelemetryServiceVersion(cfg.version),
		WithTelemetryShutdownBudget(cfg.shutdownTimeout),
	)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
		defer cancel()
		if shutdownErr := cfg.shutdownTelemetry(shutdownCtx, telemetry); shutdownErr != nil {
			fmt.Fprintf(cfg.stderr, "ax: otel shutdown failed: %s\n",
				internaltelemetry.SanitizeDiagnostic(shutdownErr.Error()))
		}
	}()
	defer func() {
		if cfg.flushFunc == nil {
			return
		}
		flushCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
		defer cancel()
		if flushErr := cfg.flushFunc(flushCtx); flushErr != nil {
			fmt.Fprintf(cfg.stderr, "ax: flush failed: %s\n",
				internaltelemetry.SanitizeDiagnostic(flushErr.Error()))
		}
	}()

	ctx, span := otel.Tracer("github.com/rshade/ax-go").Start(ctx, root.Name())
	defer span.End()

	prepareCommand(root, cfg)
	root.SetIn(cfg.stdin)
	root.SetOut(cfg.stdout)
	root.SetErr(cfg.stderr)

	// Route any Logger a callee constructs internally via a bare NewLogger(ctx)
	// call (no explicit WithWriter) through the same mutex-wrapped stderr
	// already wired into Cobra's SetErr and the rest of the diagnostic stream.
	ctx = logcore.WithDiagnosticWriter(ctx, cfg.stderr)
	rebindCommandContexts(ctx, root)

	if executeErr := root.ExecuteContext(ctx); executeErr != nil {
		span.SetStatus(codes.Error, executeErr.Error())
		axErr := normalizeExecuteError(root.Context(), root.Name(), cfg.version, executeErr)
		_ = WriteError(cfg.stderr, axErr)
		return axErr.ExitCode()
	}

	return ExitSuccess
}

// rebindCommandContexts makes Execute's decorated context authoritative for
// every command. Cobra does not replace a selected subcommand's non-nil cached
// context during ExecuteContext.
func rebindCommandContexts(ctx context.Context, root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.SetContext(ctx)
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}

func prepareCommand(root *cobra.Command, cfg executeConfig) {
	root.SilenceUsage = true
	root.SilenceErrors = true

	cli.EnsurePersistentStringFlag(root, cli.FlagFormat, "", "output format: json or human")
	cli.EnsurePersistentBoolFlag(root, cli.FlagDryRun, false, "emit the envelope without side effects")
	cli.EnsurePersistentBoolFlag(root, cli.FlagYes, false, "confirm a confirmation-gated operation")
	cli.EnsurePersistentStringFlag(
		root,
		cli.FlagIdempotencyKey,
		"",
		"opaque key used to prevent duplicate-create retries",
	)
	ensureSchemaCommand(root, cfg.version)
	wrapPersistentPreRun(root, cfg)
}

func ensureSchemaCommand(root *cobra.Command, version string) {
	for _, command := range root.Commands() {
		if command.Name() == schemaCommandName {
			return
		}
	}
	root.AddCommand(NewSchemaCommand(root, WithSchemaVersion(version)))
}

func wrapPersistentPreRun(root *cobra.Command, cfg executeConfig) {
	previousE := root.PersistentPreRunE
	previous := root.PersistentPreRun

	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		format := cli.LookupFlagString(cmd, cli.FlagFormat)
		dryRun := cli.LookupFlagBool(cmd, cli.FlagDryRun)
		approval := cli.LookupFlagBool(cmd, cli.FlagYes)
		idempotencyKey := cli.LookupFlagString(cmd, cli.FlagIdempotencyKey)
		if idempotencyKey == "" {
			idempotencyKey = NewIdempotencyKey()
		}

		stdoutIsTTY := stdoutIsTerminal()
		if cfg.stdoutIsTTY != nil {
			stdoutIsTTY = *cfg.stdoutIsTTY
		}

		mode, err := ResolveMode(format, cfg.env("AGENT_MODE"), stdoutIsTTY)
		if err != nil {
			return NewError(cmd.Context(), "validation_error", err.Error(), WithErrorExitCode(ExitValidation))
		}

		ctx := cmd.Context()
		ctx = WithMode(ctx, mode)
		ctx = WithDryRun(ctx, dryRun)
		ctx = WithApproval(ctx, approval)
		ctx = WithIdempotencyKey(ctx, idempotencyKey)
		cmd.SetContext(ctx)
		trace.SpanFromContext(ctx).SetName(cmd.CommandPath())

		if previousE != nil {
			if preRunErr := previousE(cmd, args); preRunErr != nil {
				return preRunErr
			}
		}
		if previous != nil {
			previous(cmd, args)
		}
		return nil
	}
}

// normalizeExecuteError fills empty envelope fields (trace ID, tool, version,
// schema version) on the error the command returned. A caller-supplied *Error
// is copied first: the caller owns that value and Execute must not mutate it,
// so normalization lands on the copy while the caller's fields stay untouched.
func normalizeExecuteError(ctx context.Context, tool, version string, err error) *Error {
	var axErr *Error
	if errors.As(err, &axErr) {
		normalized := *axErr
		if normalized.TraceID == "" {
			normalized.TraceID = TraceIDFromContext(ctx)
		}
		if normalized.Tool == "" {
			normalized.Tool = tool
		}
		if normalized.Version == "" {
			normalized.Version = version
		}
		if normalized.SchemaVersion == "" {
			normalized.SchemaVersion = ErrorSchemaVersion
		}
		return &normalized
	}

	return NewError(
		ctx,
		"internal_error",
		err.Error(),
		WithErrorTool(tool),
		WithErrorVersion(version),
		WithErrorExitCode(ErrorExitCode(err)),
	)
}
