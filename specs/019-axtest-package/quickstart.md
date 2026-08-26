# Quickstart: Testing a command built on ax-go with `axtest`

**Feature**: [spec.md](./spec.md) | **Contract**: [contracts/axtest-package.md](./contracts/axtest-package.md)

This walks through the scenario from GitHub issue #178: a command tree with a
`--dry-run`-aware, confirmation-gated action, tested without hand-rolling the
`ax.Execute` + `ax.WithStdout` pattern or a wrapper struct to unwrap the
envelope.

## Before (the friction this feature removes)

```go
func TestReconcileDryRun(t *testing.T) {
    root := newRootCmd()
    root.SetArgs([]string{"reconcile", "--dry-run"})
    if err := root.Execute(); err != nil {
        t.Fatal(err) // fails here: "unknown flag: --dry-run"
    }
}
```

`--dry-run` does not exist on the bare tree — it is mounted by
`ax.Execute`'s internal `prepareCommand`, never by `cobra.Command.Execute`
called directly.

## After

```go
package reconcile_test

import (
    "context"
    "testing"

    "github.com/rshade/ax-go/axtest"
)

type reconcileResult struct {
    Reconciled int `json:"reconciled"`
}

func TestReconcileDryRun(t *testing.T) {
    root := newRootCmd()

    result := axtest.Run(context.Background(), t, root, []string{"reconcile", "--format=json", "--dry-run"})
    if result.ExitCode != 0 {
        t.Fatalf("exit code = %d, want 0; stderr:\n%s", result.ExitCode, result.Stderr)
    }

    data := axtest.Decode[reconcileResult](t, result.Stdout)
    if data.Reconciled != 0 {
        t.Errorf("dry run reconciled %d resources, want 0", data.Reconciled)
    }
}
```

No wrapper struct was declared to reach `reconcileResult` out of the
envelope's `data` key, and `--dry-run` is recognized because `Run` executes
the tree through the same lifecycle a production binary uses. The test pins
`--format=json` because `Decode` consumes JSON and must not depend on the
process's `AGENT_MODE` value or TTY detection.

## Testing a confirmation-gated command's three outcomes

This is the scenario User Story 1's Acceptance Scenarios 2 and 3 describe —
distinguishing a blocked run from an approved one:

```go
func TestDeleteRequiresApproval(t *testing.T) {
    ctx := context.Background()
    root := newRootCmd()

    blocked := axtest.Run(ctx, t, root, []string{"delete", "widget-1"})
    if blocked.ExitCode != 2 {
        t.Errorf("without --yes: exit code = %d, want 2 (validation)", blocked.ExitCode)
    }

    root = newRootCmd() // fresh tree: flags on a reused root retain prior values
    approved := axtest.Run(ctx, t, root, []string{"delete", "widget-1", "--yes"})
    if approved.ExitCode != 0 {
        t.Errorf("with --yes: exit code = %d, want 0; stderr:\n%s", approved.ExitCode, approved.Stderr)
    }
}
```

The blocked run's `ax.Error` envelope is on `blocked.Stderr`, never
`blocked.Stdout` — Principle I holds inside the test exactly as it holds in
production.

## The happy-path shortcut

When a test only cares about a successful outcome, `RunAndDecode` composes
`Run` and `Decode` in one call:

```go
func TestStatusReportsVersion(t *testing.T) {
    root := newRootCmd()

    data, exitCode := axtest.RunAndDecode[statusResult](context.Background(), t, root, []string{"status", "--format=json"})
    if exitCode != 0 {
        t.Fatalf("exit code = %d, want 0", exitCode)
    }
    if data.Version == "" {
        t.Error("status result has empty version")
    }
}
```

## What this replaces in a real project

The production evidence in the spec's Problem Statement — `pulumi-curfew`'s
Task 14 — hand-declared this once per test file:

```go
type reconcileEnvelope struct {
    Data *reconcileResult `json:"data"`
}
```

...four of those seven declarations collided by name with an existing
production type in `package main`. `axtest.Decode[reconcileResult]` replaces
every one of them with a single generic call.
