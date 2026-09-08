# Quickstart: Agent-Safety Context Reaches Every Command in the Tree

**Feature**: `026-persistent-hook-context` | **Date**: 2026-09-08

Nothing to adopt. This is a transparent runtime fix inside `ax.Execute` —
adopters do not change any code to get it.

## What was broken

```go
group := &cobra.Command{
    Use: "profiles",
    PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
        return loadSharedConfig(cmd.Context())
    },
}
group.AddCommand(deleteCmd)
root.AddCommand(group)
```

Before this fix, running `mytool profiles delete --dry-run` silently lost
every piece of agent-safety context: `ax.DryRunFromContext(cmd.Context())`
returned `false` inside `deleteCmd`'s `RunE`, so an `ax.Guard`-wrapped
delete executed for real. `--yes` fared no better: `ax.Confirm` returned
`confirmation_required` even with explicit approval, because the approval
flag never reached the context either. Both are documented behaviors of
`ax.Guard` and `ax.Confirm` for a context that lacks a resolved mode — the
defect was entirely in `ax.Execute` never propagating that state past the
group's own hook.

## What this fix does

`ax.Execute` now walks the whole command tree once during setup and applies
its agent-safety context wiring to **every** command that could become
Cobra's dispatch target for a persistent hook — not only the root command.
`group`'s own `PersistentPreRunE` above still runs exactly once, doing
exactly what it always did (`loadSharedConfig`), but ax's context setup now
runs immediately before it, every time, at every depth.

```text
mytool profiles delete --dry-run
    -> ax's wrapped hook on "profiles" resolves mode/dry-run/approval/key
    -> then calls the adopter's original loadSharedConfig hook
    -> deleteCmd.RunE now sees DryRunFromContext == true, as expected
```

No adopter code changes. No new flag, option, or exported symbol.

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
make size-check
```

Expected behavior:

- for each of the five required hook shapes (child `PersistentPreRun`,
  child `PersistentPreRunE`, both on one command, grandchild-only,
  parent-and-child both), all four `*FromContext` accessors match the
  no-hook baseline;
- a `Guard`-wrapped side-effect counter stays at zero under `--dry-run` in
  every shape;
- `Confirm` returns approved under `--yes` in every shape;
- the adopter's own hook fires exactly once per command execution, in every
  shape, including across a repeated `Execute` call against the same tree;
- the success envelope's `meta.dry_run`/`meta.idempotency_key` are correct
  in every shape;
- `__schema` and `__schema --as=mcp` output is byte-identical to before this
  change — the new internal marker never appears in either.
