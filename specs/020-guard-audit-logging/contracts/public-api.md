# Public API Contract: Default-On Audited Guard/Perform

**Feature**: `020-guard-audit-logging` | **Date**: 2026-08-26

The public interface this feature exposes is three new functions and a
**behavior change** to two existing functions, all in the root `ax` package.
There are no new packages, types, flags, env vars, or envelope fields.

## Surface delta (package `ax`)

```go
// UNCHANGED SIGNATURE, CHANGED BEHAVIOR — see "Breaking-change classification" below.
func Guard(ctx context.Context, effect func(context.Context) error) (bool, error)
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error

// ADDED
func GuardWithAudit(ctx context.Context, description string, effect func(context.Context) error) (bool, error)
func PerformWithAudit(ctx context.Context, description string, rehearse, commit func(context.Context) error) error
func WithAudit(ctx context.Context, enabled bool) context.Context
```

Nothing is removed, renamed, or re-typed. The `contract`, `config`, `schema`,
`id`, `logging`, and `mcp` packages are untouched — the opt-out context key stays
root-`ax`-only (`research.md` D3).

## Breaking-change classification (Constitution Principle XI)

This is **not** a purely additive change, despite `go-apidiff` reporting it as
one.

- **What `go-apidiff` will see**: three new exported functions, zero removed,
  renamed, or re-typed symbols. Structurally indistinguishable from a plain
  additive `feat:` like feature 012's.
- **What actually changed**: `Guard`'s and `Perform`'s real-run behavior. Before
  this feature, a real-run call was silent on `stderr`. After, it unconditionally
  writes two structured lines (about-to-run + succeeded/failed) unless the call
  site opts out via `WithAudit(ctx, false)`. This is a **semantic change**, which
  Principle XI's breaking-change definition explicitly classifies as breaking for
  the Go API surface, independent of signature stability.
- **Concrete existing casualties**: `guard_test.go`'s `TestGuardSuppressionLogged`
  and `TestPerformSuppressionLogged` each currently assert real-run `stderr` is
  empty. Both assertions are corrected by this feature's own task list — proof
  the break is real, not hypothetical, and caught inside this repository before
  it could surprise a downstream consumer.
- **Required process** (MUST, per Principle XI and the spec's Clarifications):
  - Commit as `feat!:` with a `BREAKING CHANGE:` trailer.
  - PR carries the `breaking-change-approved` label — applied **manually**, since
    the automated apidiff gate will not request it for a behavior-only change.
  - Migration note (for the commit trailer and README): "`Guard` and `Perform`
    now emit two structured stderr log lines (`ax: about to run effect`, then
    `ax: effect succeeded`/`ax: effect failed`) around every real (non-dry-run)
    invocation by default. Dry-run behavior is unchanged. To restore the
    previous silent behavior for a specific call site, wrap its context with
    `ax.WithAudit(ctx, false)`."
- **Release**: rides a pre-v1.0 `0.MINOR.0` bump via release-please
  (`bump-minor-pre-major: true` already handles this; no `bump-patch` path is
  eligible for a breaking change).

## Doc-comment contract (Principle VII / godoclint require-doc)

Each of the three new exported functions and `Guard`/`Perform`'s updated doc
comments MUST state: the audit-logging behavior (default-on for `Guard`/
`Perform`; description-carrying for the `WithAudit` variants), the dry-run
suppression-line behavior (unchanged), the `WithAudit(ctx, false)` opt-out, the
defensive nil handling, that the caller's error is returned with its wrap chain
intact, and that no exit code is mapped. See `data-model.md` for the normative
behavior the comments must describe.

## Stability & SemVer (Constitution Principle XI)

- **Classification**: additive-looking, breaking-in-substance change to the
  existing public package `ax` (see above).
- **apidiff**: root `ax` is already on the public allowlist in
  `internal/cmd/apidiff-verdict`; adding three exported symbols reports as
  API-compatible from `go-apidiff`'s perspective. The `breaking-change-approved`
  label is applied deliberately, NOT because the automated check requests it —
  this is the one gate a reviewer must apply human judgment to, not defer to CI.
- **Release**: `feat!:` / `BREAKING CHANGE:` commit → pre-v1.0 `0.MINOR.0` bump.

## Documentation gates

- **doc-coverage** (`make doc-coverage` / `internal/cmd/doccover`): satisfied by
  verified `ExampleGuardWithAudit` and `ExamplePerformWithAudit` in
  `example_test.go` (both are primary-API top-level entry points, same tier as
  `ExampleGuard`/`ExamplePerform`). `WithAudit` is demonstrated **inside** one of
  those two examples, per Principle VII's "`WithX` options inside a parent
  example" rule — no separate `ExampleWithAudit` is required or added. No
  `baseline.txt` edit (already empty; examples are added, not exempted).
- **godoclint require-doc**: all three new exported functions carry doc comments
  (presence gated at 100%); `Guard`/`Perform`'s existing doc comments are updated
  to describe the new default behavior rather than left describing pre-feature
  behavior.

## Coverage gate

`internal/cmd/covercheck` floor for package `github.com/rshade/ax-go` is **85%**
(current table in `AGENTS.md`/`CLAUDE.md`). The extended truth-table tests for
`guard.go` (all rows of both tables across the new audit-enabled axis, plus the
stderr-capture tests for the new default lines and the corrected pre-existing
tests) keep the package at its established near-100% coverage for this file.

## Non-goals (explicit)

- No `contract.Guard`/`contract.Perform`/`contract.WithAudit` (would break
  `contract` import-isolation and add public surface with no consumer —
  `research.md` D3).
- No new flag, env var, or envelope field.
- No change to how `--dry-run` resolves into context, and no change to
  `logDryRunSkip`'s behavior or shape.
- No confirmation/approval gate — this feature is observability-only and never
  changes whether an effect runs (spec Out of Scope, carried from issue #179).
- No staged rollout across releases — default-on, the opt-out, and the named
  variants all ship together (spec Assumptions).
