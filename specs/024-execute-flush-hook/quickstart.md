# Quickstart: Execute-Owned Buffered Log Flush

**Feature**: `024-execute-flush-hook` | **Date**: 2026-09-01

Register buffered-log draining on the lifecycle boundary that already owns
command and telemetry shutdown. The callback is late-bound so the logger can
still be created inside `RunE` with the decorated command context and stderr.

## Recommended wiring

```go
func run(ctx context.Context) int {
    var logger ax.Logger

    root := &cobra.Command{
        Use: "mytool",
        RunE: func(cmd *cobra.Command, _ []string) error {
            logger = ax.NewLogger(
                cmd.Context(),
                ax.WithLoggerWriter(cmd.ErrOrStderr()),
                ax.WithLoggerLabels(ax.Labels{Application: "mytool"}),
                ax.WithLokiFromEnv(),
            )

            logger.Info(cmd.Context()).Msg("command completed")
            return ax.WriteJSON(cmd.OutOrStdout(), struct {
                OK bool `json:"ok"`
            }{OK: true})
        },
    }

    return ax.Execute(
        ctx,
        root,
        ax.WithFlushFunc(func(shutdownCtx context.Context) error {
            return ax.Flush(shutdownCtx, logger)
        }),
    )
}
```

`logger` may still be nil when argument parsing fails or a selected subcommand
does not construct it. `ax.Flush(ctx, nil)` is a documented no-op, so the same
callback is safe on every path.

## What Execute guarantees

- The callback runs exactly once after successful command execution or a normal
  Cobra error return.
- It receives a fresh shutdown context bounded by the duration configured with
  `ax.WithTelemetryShutdownTimeout` (the existing default applies if omitted).
- Flush runs before telemetry shutdown, and telemetry receives a separate full
  timeout window.
- A callback error is sanitized and written to configured stderr as
  `ax: flush failed: ...`.
- The callback error never changes stdout or the command's exit code.

Sanitization prevents control-character line forging; it does not redact error
text. A callback must not return errors containing PII, secrets, tokens, or
credentials.

This replaces command-local boilerplate such as:

```go
defer func() {
    flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    _ = ax.Flush(flushCtx, logger)
}()
```

Keep calling `ax.Flush` directly when another lifecycle owns shutdown and does
not call `ax.Execute`.

## Overriding the callback

Functional options resolve in order. The final callback wins:

```go
ax.Execute(ctx, root,
    ax.WithFlushFunc(first),
    ax.WithFlushFunc(second), // only second runs
)
```

A final nil clears an earlier registration:

```go
ax.Execute(ctx, root,
    ax.WithFlushFunc(first),
    ax.WithFlushFunc(nil), // no callback runs
)
```

## Verify locally

```bash
go test -race ./...
go test -race -tags=ax_no_grpc ./...
go test -race -tags=ax_no_otlp ./...
go test -race -tags=ax_no_grpc,ax_no_otlp ./...
go vet ./...
golangci-lint run
make doc-coverage
make cover-check
make surface-check
```

Expected behavior:

- success and error-path tests observe exactly one callback invocation;
- callback-failure tests preserve stdout and exit code;
- injected control characters do not create extra stderr lines;
- the integration example has no manual flush timeout/defer;
- the public surface gate reports `WithFlushFunc` in every configuration and
  profile.
