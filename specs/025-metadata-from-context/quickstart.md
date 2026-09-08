# Quickstart: Live-Resolving Metadata Accessor for Root ax

**Feature**: `025-metadata-from-context` | **Date**: 2026-09-07

Read the same trace-correlated metadata a command's own success or error
envelope would carry, without building either payload type yourself.

## Recommended usage

```go
root := &cobra.Command{
    Use: "mytool",
    RunE: func(cmd *cobra.Command, _ []string) error {
        meta := ax.MetadataFromContext(cmd.Context())

        return ax.WriteJSON(cmd.OutOrStdout(), struct {
            OK       bool       `json:"ok"`
            Metadata ax.Metadata `json:"metadata"`
        }{OK: true, Metadata: meta})
    },
}

return ax.Execute(ctx, root)
```

`meta.TraceID` and `meta.SpanID` reflect the root span `ax.Execute` opened
around this command (or a child span you opened yourself, if called after
one is started). `meta.DryRun` and `meta.IdempotencyKey` reflect the same
`--dry-run` and idempotency-key state `ax.NewEnvelope`/`ax.NewError` already
surface.

## Why not `contract.MetadataFromContext`?

```go
// Inside a command run through ax.Execute:
viaContract := contract.MetadataFromContext(cmd.Context()) // TraceID: ZeroTraceID
viaRoot := ax.MetadataFromContext(cmd.Context())            // TraceID: <real trace ID>
```

`contract` is deliberately import-isolated from OpenTelemetry so a
`contract`-only consumer stays small — it cannot see the active span. Calling
`contract.MetadataFromContext` from root-package code silently returns zero
trace/span IDs instead of erroring, because zero IDs are themselves valid W3C
values. `ax.MetadataFromContext` is the live-resolving equivalent: reach for
it whenever you are inside root `ax` code (a command run through
`ax.Execute`, or anywhere else the OpenTelemetry SDK is in play).

## What it guarantees

- `ax.MetadataFromContext(ctx)` always equals `ax.NewEnvelope(ctx, x).Meta`
  for the same context observed at the same point in execution — it is the
  same composition, not a parallel implementation that could drift.
- With no active span, it returns `ax.ZeroTraceID`/`ax.ZeroSpanID` rather
  than an error.
- A `nil` context does not panic; it behaves as `context.Background()`.
- If a context carries an explicit trace/span ID (set via
  `contract.WithMetadata`) and also has an active live span, the live span
  wins — the same precedence `NewEnvelope`/`NewError` already apply.
- `contract.MetadataFromContext`'s own return value is unchanged by this
  feature for every input; only its doc comment gains a warning about the
  isolation boundary.

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

- the equality test against `ax.NewEnvelope(...).Meta` passes with non-zero
  trace/span IDs when run inside `ax.Execute`;
- the no-active-span test returns `ZeroTraceID`/`ZeroSpanID`;
- the nil-context test does not panic;
- the public surface gate reports `MetadataFromContext` in every
  configuration and profile;
- `golangci-lint`'s `require-doc` passes for both the new root function and
  the updated `contract.MetadataFromContext` doc comment.
