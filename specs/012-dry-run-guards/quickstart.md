# Quickstart: dry-run side-effect guards

**Feature**: `012-dry-run-guards` | **Date**: 2026-06-28

`ax.Guard` and `ax.Perform` let a command author make `--dry-run` safe and faithful without
hand-rolling a conditional. Dry-run state is already in `context.Context` and the envelope
already carries `dry_run: true`, so you only wrap the side effect.

## Before (hand-rolled)

```go
RunE: func(cmd *cobra.Command, _ []string) error {
    ctx := cmd.Context()
    if ax.DryRunFromContext(ctx) {
        // easy to forget; easy to let a side effect leak
    } else {
        if err := writeReport(ctx, path); err != nil {
            return err
        }
    }
    return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(ctx, payload{Path: path}))
}
```

## After — `Guard` (skip-only)

Use `Guard` when dry-run should simply *not* perform the side effect.

```go
RunE: func(cmd *cobra.Command, _ []string) error {
    ctx := cmd.Context()

    wrote, err := ax.Guard(ctx, func(ctx context.Context) error {
        return writeReport(ctx, path) // skipped under --dry-run
    })
    if err != nil {
        return err
    }

    // wrote == false under --dry-run; shape the payload accordingly.
    return ax.WriteJSON(cmd.OutOrStdout(),
        ax.NewEnvelope(ctx, payload{Path: path, Written: wrote}))
}
```

Under `--dry-run`: `writeReport` is never called, `wrote` is `false`, one suppression line
is written to `stderr`, and the envelope still carries `dry_run: true`.

## After — `Perform` (faithful preview)

Use `Perform` when dry-run should still surface the *same* validation errors a real run
would — without mutating. The `rehearse` callback does the read-only checks; `commit` does
the real mutation.

```go
RunE: func(cmd *cobra.Command, _ []string) error {
    ctx := cmd.Context()

    err := ax.Perform(ctx,
        func(ctx context.Context) error { // rehearse: validate, do not mutate
            return validatePatch(ctx, configPath, patchDoc)
        },
        func(ctx context.Context) error { // commit: the real mutation
            return ax.PatchConfigFile(ctx, configPath, []byte(patchDoc))
        },
    )
    if err != nil {
        return err // a bad patch fails identically in dry-run and real mode
    }

    return ax.WriteJSON(cmd.OutOrStdout(),
        ax.NewEnvelope(ctx, payload{Path: configPath, Patched: true}))
}
```

- **Real run**: `commit` runs, `rehearse` is ignored.
- **Dry-run**: `rehearse` runs (surfacing the same errors), `commit` never runs, one
  suppression line is written to `stderr`, and the success envelope is byte-identical to a
  real run apart from `dry_run: true`.
- Pass `nil` for `rehearse` to make dry-run a pure skip (same effect as `Guard`).

## What you get for free

- **No side effects under dry-run** — the `effect`/`commit` callback is never invoked
  (Constitution Principle IV).
- **`dry_run: true`** continues to flow into the envelope automatically; the helpers do not
  touch your envelope.
- **A suppression line on `stderr`** (`"dry-run: side effect suppressed"`, with `dry_run`,
  `ax_helper`, and `trace_id`/`span_id` fields) so an agent can see what a dry-run withheld —
  on `stderr`, never `stdout`.
- **Errors pass through** with their wrap chain intact, so your existing `ax.Error` mapping
  and exit codes are unaffected.

## Verify locally

```bash
go test -race ./...
go vet ./...
golangci-lint run
make doc-coverage      # ExampleGuard / ExamplePerform present and verified
make cover-check       # root ax package floor (80%) satisfied
```
