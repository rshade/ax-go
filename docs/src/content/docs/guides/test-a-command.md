---
title: Test a command built on ax-go
description: Run a Cobra command tree through the real ax.Execute lifecycle in a test, and decode its result without a hand-declared wrapper struct.
sidebar:
  order: 3
---

You have a command built on `ax-go` and you want to test its `--dry-run` and
confirmation-gated behavior, without hand-rolling the `ax.Execute` +
`ax.WithStdout` pattern or a wrapper struct to unwrap the envelope.

## The problem

A bare `*cobra.Command`'s own `Execute()` has never seen `--dry-run`, `--yes`,
`--format`, or `--idempotency-key` — those flags are mounted by
`ax.Execute`'s internal setup, not by Cobra itself:

```go
func TestReconcileDryRun(t *testing.T) {
    root := newRootCmd()
    root.SetArgs([]string{"reconcile", "--dry-run"})
    if err := root.Execute(); err != nil {
        t.Fatal(err) // fails here: "unknown flag: --dry-run"
    }
}
```

`github.com/rshade/ax-go/axtest` runs the tree through the same lifecycle a
production binary uses instead.

## Run a command and inspect its result

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

## Test a confirmation-gated command's blocked and approved outcomes

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
`blocked.Stdout` — stream separation holds inside the test exactly as it
holds in production.

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

A caller expecting a non-zero exit code should use `Run` directly:
`RunAndDecode` fails the test immediately on a shape mismatch, which would
obscure the exit code you actually wanted to assert on.

:::note[Test-only by convention and by check]
`axtest` is designed to be imported only from `_test.go` files. It depends on
the full root `ax` package and Cobra without restriction — unlike `logging`,
`config`, `contract`, `id`, and `schema`, it isolates discoverability and a
stable home for test tooling, not binary size. A test in `axtest` itself
asserts that no non-test source file anywhere in the module imports it, under
every supported build configuration and GOOS/GOARCH profile.
:::

## Related

- [Expose your command tree with `__schema`](/ax-go/guides/expose-schema/) —
  the flags `axtest.Run` exercises are the same ones `__schema` documents.
- [Choose a logging surface](/ax-go/guides/choose-a-logging-surface/) — the
  other public packages follow a size-isolation rationale that `axtest`
  deliberately does not.
