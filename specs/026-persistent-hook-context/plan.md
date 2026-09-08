# Implementation Plan: Agent-Safety Context Reaches Every Command in the Tree

**Branch**: `026-persistent-hook-context` | **Date**: 2026-09-08 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/026-persistent-hook-context/spec.md`

## Summary

`ax.Execute`'s `wrapPersistentPreRun` currently installs agent-safety context
setup (resolved mode, dry-run, approval, idempotency key) on `root`'s
persistent hook only. Cobra dispatches only the **nearest** ancestor's
persistent hook for an invoked command — never every ancestor's — unless the
adopter opts into `cobra.EnableTraverseRunHooks`. Any subcommand or group
that declares its own `PersistentPreRun`/`PersistentPreRunE` (ordinary Cobra
practice) therefore becomes the nearest hook for everything beneath it,
completely shadowing ax's setup for that subtree.

The fix walks the full command tree once during `prepareCommand` and wraps
**every** command that could become "the nearest ancestor with a hook" for
some invoked command: root always (the existing universal fallback), plus
any other command that already declares its own persistent hook at wrap
time. Each wrap reuses the exact same context-setup body already proven
correct for root, parameterized by the actually-invoked command Cobra passes
at call time (not the command the hook happens to be attached to — Cobra
always passes the originally-invoked command as the first argument,
regardless of which ancestor's hook fired, confirmed by reading
`cobra@v1.10.2`'s `Command.execute`). A new Cobra annotation marks a command
already wrapped, so a second `Execute` call against the same long-lived
command tree (the MCP server case, which already serves overlapping calls
against one tree) does not re-wrap or double-invoke anything.

## Technical Context

**Language/Version**: Go 1.27.1

**Primary Dependencies**: Existing dependencies only — `github.com/spf13/cobra`
(already imported by `execute.go`); no new module dependency. Confirmed
against the vendored `cobra@v1.10.2` source (`command.go`'s `execute()`
method) that Cobra passes the invoked command, not the hook-owning ancestor,
to whichever persistent hook it dispatches.

**Storage**: N/A — the only new state is a `string`-keyed entry in each
wrapped command's existing `Annotations map[string]string` field (already
used by `internal/schema` for an unrelated marker), scoped to that command
object's lifetime.

**Testing**: Test-first, table-driven over the required hook shapes: child
`PersistentPreRun`, child `PersistentPreRunE`, both on the same command,
grandchild-only (parent has no hook, its child does), and parent-and-child
both declaring one. Each shape is asserted against: (1) all four
agent-safety context values matching the no-hook baseline, (2)
`ax.Guard` suppressing a side effect under `--dry-run`, (3) `ax.Confirm`
returning approved under `--yes`, (4) the adopter's own hook still running
exactly once (counter-based), (5) the success envelope's `meta` still
carrying `dry_run`/`idempotency_key`. A repeated-`Execute`-call test proves
the idempotency annotation prevents re-wrapping. Required verification:
`go test -race ./...` across all four build-tag combinations, `go vet ./...`,
`golangci-lint run`, `make doc-coverage`, `make cover-check`,
`make surface-check`, `make size-check`. No golden `__schema` change is
expected — `internal/schema.NonDeterministicFields` reads only its own two
known annotation keys, so the new marker cannot leak into schema output
(verified by reading `internal/schema/nondeterministic.go` and
`schema/schema.go`'s `CommandSchema`, which has no raw `Annotations` field).

**Target Platform**: All supported ax-go consumer targets and all surface
gate profiles (`linux`, `darwin`, `windows` × `amd64`, `arm64`), under
default, `ax_no_grpc`, `ax_no_otlp`, and combined build configurations.
`execute.go` is untagged, so the fix is present identically in all four.

**Project Type**: Go library with a runnable Cobra integration example and
Starlight documentation site.

**Performance Goals**: `BuildCommand`'s `__schema` reflection path is a
CI-tracked benchmark, but this feature does not touch `BuildCommand` or
`schema.BuildSchema`. `prepareCommand`'s own tree walk is new work, but it
runs once per `Execute` call over a command tree that is already walked once
by `rebindCommandContexts` in the same function — proportional, one-time,
not a per-request hot path. No numeric performance claim is asserted; no new
benchmark is added, matching the existing untracked status of
`prepareCommand`.

**Constraints**:

- MUST NOT set `cobra.EnableTraverseRunHooks` — that changes the adopter's
  own multi-hook semantics process-wide, which is not this library's
  decision to make (explicit non-goal in the issue and spec).
- MUST NOT change `ax.Guard`, `ax.Confirm`, or `DryRunFromContext` — all
  three already behave correctly for a context carrying the right state;
  the defect is strictly upstream in how that state reaches the context.
- MUST preserve exact existing behavior for the previously-safe
  configuration (only root declares a hook, or no command declares one).
- MUST invoke each adopter-declared hook exactly once per command
  execution, preserving its existing error-propagation semantics.
- MUST wrap idempotently: a command already wrapped by a prior `Execute`
  call on the same tree object must not be wrapped or invoked again.
- Zero exported Go signature changes.

**Scale/Scope**: One new unexported constant (the annotation key), one new
unexported helper function (`wrapCommandPersistentPreRun`, the single-command
wrap-and-mark step factored out of the existing `wrapPersistentPreRun`), and
a tree-walk loop added to `wrapPersistentPreRun` itself. Table-driven tests
in `execute_test.go` covering five hook shapes. No new package, flag,
environment variable, command, payload field, dependency, or goroutine.

**Governing ADR(s)**: N/A. No ADR governs `Execute`'s persistent-hook
composition; ADR-0008 fixes Cobra as the framework and is untouched by this
fix (it does not revisit framework choice or Cobra's own semantics — it
corrects how ax-go composes with an existing, documented Cobra mechanism).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | PASS | No stream behavior touched; the fix is entirely context-propagation inside a pre-run hook. |
| II. Deterministic Output & Exit Codes | PASS | No payload/exit-code mapping change; the success envelope's `meta` fields become *correct* in the previously-broken scenario, not differently shaped. |
| III. Machine Discoverability via `__schema` | PASS | Verified the new Cobra annotation cannot leak into `__schema`/MCP output — `internal/schema.NonDeterministicFields` reads only its own two known keys, and the public `CommandSchema` carries no raw `Annotations` field. |
| IV. Agent-Safety Primitives | PASS | This feature directly restores the Principle IV guarantee (`--dry-run` MUST cause no side effects; idempotency key MUST auto-generate and surface) for a command-tree shape the guarantee silently failed for today. |
| V. Asymmetric JSON I/O | PASS | No input parsing or output encoding changes. |
| VI. ADR-Governed Scope — Library, Not Application | PASS | A composition fix inside the existing `Execute` lifecycle is foundation-library scope; no orchestration, persistence, or new ADR. Routed through this Spec Kit feature per Principle VI's own requirement for any runtime-behavior change. |
| VII. Test-First Discipline | PASS | Failing table-driven tests across all five required hook shapes precede implementation, per FR-008 and the issue's own testing-strategy section. |
| VIII. Observability & ID Discipline | PASS | No trace/resource ID scheme change; `trace.SpanFromContext(ctx).SetName(...)` behavior is preserved unchanged for every wrapped command. |
| IX. Security & Resource Safety | PASS | No I/O, no new panic path, no unbounded read. The annotation write is a bounded, in-memory map entry on an already-allocated struct. |
| X. Idiomatic Go & Dependency Minimalism | PASS | No new dependency; reuses the exact existing context-setup body via a factored-out helper rather than duplicating it. |
| XI. Stability & SemVer | PASS | Zero exported Go signature changes; `Metadata`/`Envelope`/`Error`/`Schema` shapes are unchanged. This is a runtime-behavior fix to a public entry point (`ax.Execute`) for a scenario no spec, ADR, or doc ever described as supported — ships as a non-breaking `fix:`, pre-v1.0 minor per Principle XI (a `0.x` release MAY fix behavior; relying on the shadowing defect was never a coherent, documented scenario). |
| XII. Deprecation Lifecycle | PASS | No symbol is deprecated, renamed, or removed. |

**ADR absorption gate (Constitution §Governance)**: PASS — Governing ADR(s) =
N/A. ADR-0008 remains untouched; no ADR-retirement task is required.

**Post-design re-check**: PASS. The research and data model confirm the fix
reuses Cobra's own documented invocation contract (the invoked command is
always the hook's first argument) rather than working around it, needs no
new public surface, and the annotation mechanism is proven not to leak into
`__schema`. No complexity exception is needed.

## Project Structure

### Documentation (this feature)

```text
specs/026-persistent-hook-context/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── behavior-contract.md
├── checklists/
│   └── requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
execute.go                 # REFACTOR wrapPersistentPreRun into a tree walk;
                            # ADD wrapCommandPersistentPreRun + the annotation constant
execute_test.go            # ADD table-driven hook-shape tests, Guard/Confirm-by-shape
                            # tests, envelope-meta assertion, repeat-Execute idempotency test
README.md                  # UPDATE the persistent-hook contract if adopter-facing
                            # guidance needs stating (only if plan/tasks find a gap)
```

**Structure Decision**: Extend the existing root `ax` execution facade in
place. The fix is a targeted refactor of one existing unexported function
(`wrapPersistentPreRun`) plus one new unexported helper in the same file —
no new file, package, or public surface. This mirrors how `execute.go`
already hosts `rebindCommandContexts`, a same-shaped tree-walk helper for a
different concern (context rebinding) in the same lifecycle stage.

## Complexity Tracking

*No violations — table intentionally empty.*
