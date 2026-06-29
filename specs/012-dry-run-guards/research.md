# Research: Agent-safety helpers for --dry-run side-effect suppression

**Feature**: `012-dry-run-guards` | **Date**: 2026-06-28

**Decision Records Absorbed**: **N/A.** Agent-safety primitives are governed directly by
Constitution Principle IV (Agent-Safety Primitives) — "`--dry-run` MUST emit the same
envelope with `dry_run: true` and cause no side effects." No ADR governs this feature, so
there is no ADR to absorb or retire.

All Technical-Context unknowns are resolved below; no `NEEDS CLARIFICATION` remain.

---

## D1 — Exact helper signatures

**Decision**: Match the user-approved shapes verbatim, in package `ax`:

```go
func Guard(ctx context.Context, effect func(context.Context) error) (executed bool, err error)
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error
```

**Rationale**: These were presented and approved in the planning question previews.
`Guard` returns `executed` so the caller can shape its output (e.g. set a `created: false`
field) without re-reading the dry-run flag. `Perform` returns only `error` because the
caller already knows which branch semantics it asked for; "which branch ran" is recoverable
via `DryRunFromContext` if ever needed. Both take their callbacks as
`func(context.Context) error` so the supplied work receives the (possibly cancelled)
context — consistent with `context.Context`-first discipline (Principle X).

**Alternatives considered**:

- Generic value-returning variants (`Perform[T]`) so a commit can return a result. Rejected:
  contradicts the approved `error`-only signature, and callers can capture results through a
  closure variable. Keeps the surface minimal (Principle X).
- A single combined helper with an optional rehearse. Rejected: the user explicitly chose
  **both** a skip-only `Guard` and a rehearse/commit `Perform`; two named entry points read
  more clearly at call sites than one overloaded one.

## D2 — Source of the logger for the suppression line

**Decision**: Construct the canonical logger inline on the skip path with
`ax.NewLogger(ctx)` and emit at **Info** level. There is no logger-in-context lookup.

**Rationale**: AGENTS.md / ADR-0009 explicitly forbid an `ax.WithLogger(...)`-style runtime
logger-selection API and a second logger backend, so there is no `LoggerFromContext` to pull
from — `ax.NewLogger(ctx)` is *the* canonical constructor. It defaults to `os.Stderr` at
`InfoLevel` and installs the `tracingHook`, so the suppression line automatically carries
`trace_id`/`span_id`. Info (not Debug) is required because the default logger filters Debug,
and FR-013/SC-007 require the line to actually appear.

**Alternatives considered**:

- Pull a pre-configured logger from context. Rejected: no such API exists and adding one is
  explicitly out of scope (ADR-0009 migration-seam clause).
- A package-level default logger var the helpers reuse. Rejected: AGENTS.md bans mutable
  package-level state (Principle X).

**Accepted cost**: A fresh `NewLogger` allocation per skip. Skips happen at most once per
guarded side effect per command invocation — not a hot loop — so no benchmark/`testing.B`
claim is made (Principle VII/X). If a future hot-path caller emerges, that is a separate,
benchmark-backed change.

## D3 — Suppression log-line shape

**Decision**: A static message with structured fields, never composed from user input:

```go
ax.NewLogger(ctx).
    Info(ctx).
    Bool("dry_run", true).
    Str("ax_helper", "Guard"). // or "Perform"
    Msg("dry-run: side effect suppressed")
```

**Rationale**: Satisfies the guardrail "never compose log messages from un-sanitized
user-controlled strings" — the message is a constant and all variable data goes through
ZeroLog field methods (`.Bool`, `.Str`), which escape correctly (Principle IX, no log
forging / no PII). The `dry_run` field makes the line trivially greppable by agents; the
`ax_helper` field distinguishes which primitive skipped. `trace_id`/`span_id` are added by
the existing hook, so the line correlates with the active span (Principle VIII).

**Alternatives considered**: Golden-testing the exact line. Rejected: the line carries
non-deterministic `trace_id`/`span_id`, so it is asserted by substring/field presence, not a
golden fixture.

## D4 — Testing the stderr suppression line under a fixed signature

**Decision**: Capture `os.Stderr` via an `os.Pipe` in a **non-parallel** test, run the
helper, close the writer, and read the captured bytes; assert the JSON line contains
`"dry_run":true`, the `ax_helper` field, and the static message — and that the real-run path
emits **nothing**. Pure behavior tests (the truth tables in D6) do not capture stderr.

**Rationale**: `NewLogger(ctx)` writes to `os.Stderr` evaluated at call time, and the
approved signature has no writer parameter, so redirecting the process `os.Stderr` is the
idiomatic capture path (the same approach the stdlib and this repo's own tests use for
process-stream assertions). Keeping the test non-`t.Parallel()` makes the global swap
`-race`-clean because no other goroutine logs during it. SC-007 (one line on stderr, zero
bytes added to stdout) is asserted by capturing *both* streams.

**Alternatives considered**:

- Add a variadic `...Option` to inject a writer. Rejected: changes the approved signature;
  the user locked the shape.
- A test-only seam (build tag / exported test hook). Rejected: unnecessary complexity for a
  well-understood `os.Pipe` capture; would add surface area for no production benefit.

## D5 — File and symbol placement

**Decision**: New file `guard.go` in package `ax` holds `Guard`, `Perform`, and an
unexported `logDryRunSkip(ctx context.Context, helper string)` used by both. Tests in
`guard_test.go`; the two verified examples (`ExampleGuard`, `ExamplePerform`) are appended to
the existing `example_test.go` (package `ax_test`), matching the repo convention of housing
examples there.

**Rationale**: Co-locating the two related primitives and their shared skip-logger in one
file keeps the feature legible. The unexported helper removes duplication between `Guard` and
`Perform` and centralizes the FR-013 log contract. Examples in `example_test.go` keep
`make doc-coverage` discovery in its established location.

## D6 — Branch semantics (truth tables)

**Decision**: The suppression log fires only when dry-run is active **and** a non-nil
effect/commit was therefore skipped (there is nothing to "suppress" otherwise). Full truth
tables live in `data-model.md`; summary:

- `Guard`: real run → execute `effect`, return `(true, err)`, no log. Dry-run → skip,
  return `(false, nil)`; log iff `effect != nil`.
- `Perform`: real run → execute `commit` (ignore `rehearse`), return its error, no log;
  `nil commit` → no-op, `nil`. Dry-run → run `rehearse` if non-nil; a failed rehearsal
  returns its error with **no** log; otherwise never run `commit` and log iff
  `commit != nil`.

**Rationale**: Makes the "no side effects under dry-run" guarantee total (FR-007) while
keeping the log honest — no "suppressed" line when nothing would have run, and no
"suppressed" line when the preview itself failed (the command already surfaces that error). Defensive nil
handling (FR-011) avoids panics on a missing callback (Principle IX).

**Alternatives considered**: Always log whenever dry-run is active. Rejected: emits a
misleading "side effect suppressed" line when the caller passed no effect/commit.

## D7 — Nil / absent context

**Decision**: Read dry-run via `ax.DryRunFromContext(ctx)`. Because that delegates to
`ctx.Value(...)`, which panics on a literally-nil `context.Context`, both helpers FIRST
normalize a nil context with `if ctx == nil { ctx = context.Background() }` (mirroring
`contract.MetadataFromContext`). A nil/absent context then resolves dry-run as inactive and
runs the real path without panicking.

**Rationale**: Satisfies the spec Edge Case and FR-011 ("MUST NOT panic"). An earlier draft
of this decision assumed the logger was the only nil-`ctx` hazard and skipped the guard;
the adversarial review reproduced a panic, showing `DryRunFromContext` itself dereferences
the nil interface *before* any branch — so an explicit nil guard is required. Verified by
`TestGuardPerformNilContextNoPanic`.

## D8 — Integration-example refactor (canonical demonstration)

**Decision**: Refactor `examples/integration/main.go`'s `newPatchConfigCommand` to call
`ax.Perform(ctx, rehearse, commit)`, where `rehearse` is the existing in-memory
`dryRunPatchConfig` logic (read + apply patch in memory, discard) and `commit` is
`ax.PatchConfigFile(...)`. This replaces the hand-rolled `if ax.DryRunFromContext(...)`
conditional and becomes the canonical FR-012 demonstration.

**Rationale**: Proves the helper on the repo's own real, mutating command and keeps the
example honest (the rehearsal already surfaces the same missing-file / invalid-Hujson /
invalid-patch errors a real run would). Note the integration command's dry-run path will now
also emit the FR-013 suppression line on stderr; any existing integration test that asserts
*exact* stderr must accommodate the added line (its stdout envelope is unchanged).

## D9 — MCP dispatch path

**Decision**: No change to `internal/mcpserver/dispatch.go`. It already seeds dry-run into
the per-call context (`contract.WithDryRun`, dispatch.go:487); served commands run their
normal `RunE`, which may call `Guard`/`Perform`, and the dry-run state flows through. The
suppression line lands on the server's `stderr`, never on the tool result (which is the
command's `stdout`), so stream separation holds across the MCP boundary.

**Rationale**: The helpers are for command authors; the dispatcher composes with them for
free. No new coupling between `internal/mcpserver` and the new symbols is required.

## D10 — Stability, apidiff, and doc-coverage gates

**Decision**: Adding `Guard`/`Perform` to root `ax` is an **additive** public-API change.
Root `ax` is already on the apidiff allowlist (`internal/cmd/apidiff-verdict`), so no
allowlist edit and no `breaking-change-approved` label are needed; the change rides a `feat:`
commit and a pre-v1.0 `0.MINOR.0` bump (Principle XI). The two `ExampleXxx` satisfy FR-012
and keep `make doc-coverage` green without touching `baseline.txt` (we add examples rather
than exempt symbols). Root-package coverage floor is 80% (`internal/cmd/covercheck`); the
truth-table tests put `guard.go` near 100%.

**Rationale**: Keeps every CI gate green with zero governance friction — the feature is
purely additive within an already-sanctioned surface.

## Open questions

None. All Technical-Context items are resolved; Constitution Check passes pre- and
post-design.
