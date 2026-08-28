# Quickstart: default-on audited Guard/Perform

**Feature**: `020-guard-audit-logging` | **Date**: 2026-08-26

`ax.Guard` and `ax.Perform` now log an audit trail around every real (non-dry-run)
invocation **by default** — you don't have to change anything to get it.

## What changes for existing code — nothing you have to write

```go
RunE: func(cmd *cobra.Command, _ []string) error {
    ctx := cmd.Context()

    wrote, err := ax.Guard(ctx, func(ctx context.Context) error {
        return destroyStack(ctx, stackName) // unchanged call site
    })
    if err != nil {
        return err
    }
    return ax.WriteJSON(cmd.OutOrStdout(),
        ax.NewEnvelope(ctx, payload{Stack: stackName, Destroyed: wrote}))
}
```

On the next real run, `stderr` now also carries:

```json
{"level":"info","ax_helper":"Guard","description":"","trace_id":"...","span_id":"...","message":"ax: about to run effect"}
{"level":"info","ax_helper":"Guard","description":"","trace_id":"...","span_id":"...","message":"ax: effect succeeded"}
```

— or, if `destroyStack` returns an error:

```json
{"level":"error","ax_helper":"Guard","description":"","error":"...","trace_id":"...","span_id":"...","message":"ax: effect failed"}
```

`--dry-run` behavior is byte-for-byte unchanged: you still get exactly the one
existing suppression line, nothing more.

## Getting a meaningful description instead of the generic default

Switch the call site to the named variant and supply a description — everything
else about the call is identical:

```go
wrote, err := ax.GuardWithAudit(ctx, "destroy stack "+stackName,
    func(ctx context.Context) error {
        return destroyStack(ctx, stackName)
    })
```

```json
{"level":"info","ax_helper":"Guard","description":"destroy stack prod-east","trace_id":"...","span_id":"...","message":"ax: about to run effect"}
```

`ax.PerformWithAudit` is the same treatment for `Perform`:

```go
err := ax.PerformWithAudit(ctx, "reconcile stack "+stackName,
    func(ctx context.Context) error { // rehearse: validate, do not mutate
        return validatePlan(ctx, stackName)
    },
    func(ctx context.Context) error { // commit: the real mutation
        return applyPlan(ctx, stackName)
    },
)
```

## Suppressing the default for a subtree of calls

Some effects are high-frequency or not consequential enough to warrant an audit
line on every call (e.g. an internal cache write inside a hot loop). Opt out by
wrapping the context with `ax.WithAudit(ctx, false)`, which suppresses the audit
lines for every Guard/Perform invocation reached through that context:

```go
quietCtx := ax.WithAudit(ctx, false)
_, err := ax.Guard(quietCtx, func(ctx context.Context) error {
    return updateInMemoryCache(ctx, key, value)
})
```

`WithAudit(ctx, false)` only affects the real-run audit lines; it has no effect on
`--dry-run` behavior, which is always the same single suppression line regardless.
It propagates to all Guard/Perform invocations reached through that context (including
calls inside the effect closure), following standard context-value semantics. It also
works with the named variants (`GuardWithAudit`/`PerformWithAudit`) — opting out
silences the audit lines even when a description was supplied.

## What you get for free

- **Baseline audit visibility on every existing call site**, the moment you
  upgrade — no code change required (this is the whole point of the feature: an
  opt-in mechanism nobody remembers to use provides no safety net; a default does).
- **A richer, described audit trail** by switching a specific call site to
  `GuardWithAudit`/`PerformWithAudit`.
- **An escape hatch** (`ax.WithAudit(ctx, false)`) for call sites where the default
  is inappropriate.
- **No side effects under dry-run** — unchanged from before this feature
  (Constitution Principle IV).
- **Errors pass through** with their wrap chain intact, so your existing
  `ax.Error` mapping and exit codes are unaffected.
- **`trace_id`/`span_id`** on every audit line automatically, same as every other
  ax-go log line.

## Migrating existing code

This IS a breaking behavior change (Constitution Principle XI — a semantic
change, not a signature change). If your tests assert `Guard`/`Perform` produce
no `stderr` output on a real run, you have two options:

1. Update the assertion to expect the new audit lines (recommended — the
   visibility is the point).
2. Wrap the call site's context with `ax.WithAudit(ctx, false)` to keep the old
   silent behavior.

## Verify locally

```bash
go test -race ./...
go vet ./...
golangci-lint run
make doc-coverage      # ExampleGuardWithAudit / ExamplePerformWithAudit present and verified
make cover-check       # root ax package floor (85%) satisfied
```
