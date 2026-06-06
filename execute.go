package ax

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/cli"
)

const defaultTelemetryShutdownTimeout = 2 * time.Second

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
func WithVersion(version string) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.version = version
	}
}

// WithTelemetryShutdownTimeout sets the OTel shutdown timeout.
func WithTelemetryShutdownTimeout(timeout time.Duration) ExecuteOption {
	return func(cfg *executeConfig) {
		cfg.shutdownTimeout = timeout
	}
}

// Execute wraps Cobra execution with AX mode resolution, idempotency, schema,
// error-envelope, and telemetry lifecycle behavior. It returns a deterministic
// exit code and leaves process termination to the caller.
func Execute(ctx context.Context, root *cobra.Command, opts ...ExecuteOption) int {
	cfg := executeConfig{
		stdin:           os.Stdin,
		stdout:          os.Stdout,
		stderr:          os.Stderr,
		env:             os.Getenv,
		shutdownTimeout: defaultTelemetryShutdownTimeout,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.env == nil {
		cfg.env = os.Getenv
	}

	ctx, telemetry, telemetryErr := StartTelemetry(ctx, WithTelemetryEnv(cfg.env))
	if telemetryErr != nil {
		_ = WriteError(cfg.stderr, NewError(
			ctx,
			"telemetry_error",
			telemetryErr.Error(),
			WithErrorTool(root.Name()),
			WithErrorVersion(cfg.version),
		))
		return ExitInternal
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
		defer cancel()
		if shutdownErr := telemetry.Shutdown(shutdownCtx); shutdownErr != nil {
			fmt.Fprintf(cfg.stderr, "ax: otel shutdown failed: %v\n", shutdownErr)
		}
	}()

	prepareCommand(root, cfg)
	root.SetIn(cfg.stdin)
	root.SetOut(cfg.stdout)
	root.SetErr(cfg.stderr)

	if executeErr := root.ExecuteContext(ctx); executeErr != nil {
		axErr := normalizeExecuteError(root.Context(), root.Name(), cfg.version, executeErr)
		_ = WriteError(cfg.stderr, axErr)
		return axErr.ExitCode()
	}

	return ExitSuccess
}

func prepareCommand(root *cobra.Command, cfg executeConfig) {
	root.SilenceUsage = true
	root.SilenceErrors = true

	cli.EnsurePersistentStringFlag(root, "format", "", "output format: json or human")
	cli.EnsurePersistentBoolFlag(root, "dry-run", false, "emit the envelope without side effects")
	cli.EnsurePersistentStringFlag(
		root,
		"idempotency-key",
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
		format := cli.LookupFlagString(cmd, "format")
		dryRun := cli.LookupFlagBool(cmd, "dry-run")
		idempotencyKey := cli.LookupFlagString(cmd, "idempotency-key")
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
		ctx = WithIdempotencyKey(ctx, idempotencyKey)
		cmd.SetContext(ctx)

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

func normalizeExecuteError(ctx context.Context, tool, version string, err error) *Error {
	var axErr *Error
	if errors.As(err, &axErr) {
		if axErr.TraceID == "" {
			axErr.TraceID = TraceIDFromContext(ctx)
		}
		if axErr.Tool == "" {
			axErr.Tool = tool
		}
		if axErr.Version == "" {
			axErr.Version = version
		}
		if axErr.SchemaVersion == "" {
			axErr.SchemaVersion = ErrorSchemaVersion
		}
		return axErr
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
