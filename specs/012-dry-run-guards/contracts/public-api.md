# Public API Contract: Agent-safety dry-run helpers

**Feature**: `012-dry-run-guards` | **Date**: 2026-06-28

The only public interface this feature exposes is two new functions in the root `ax`
package. There are no new packages, types, flags, env vars, or envelope fields.

## Surface delta (package `ax`)

```go
// ADDED
func Guard(ctx context.Context, effect func(context.Context) error) (bool, error) // (executed, error)
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error
```

Nothing is removed, renamed, or changed. The `contract`, `config`, `schema`, and `id`
packages are untouched (FR-008: the helpers must NOT be added to `contract`).

## Doc-comment contract (Principle VII / godoclint require-doc)

Each function carries a contract-style doc comment stating: the dry-run-gated behavior, the
stderr suppression line, the defensive nil handling, that the caller's error is returned with
its wrap chain intact, and that the helper itself maps no exit code. See `data-model.md` for
the normative behavior the comments must describe.

## Stability & SemVer (Constitution Principle XI)

- **Classification**: additive change to the existing public package `ax`.
- **apidiff**: root `ax` is already on the public allowlist in
  `internal/cmd/apidiff-verdict`. Adding exported functions is API-compatible (additive), so
  `go-apidiff` reports no incompatible change; **no `breaking-change-approved` label** is
  required and the `check-packages` guard stays satisfied (no new public package).
- **Release**: ride a Conventional `feat:` commit → pre-v1.0 `0.MINOR.0` bump via
  release-please. Not a breaking change; no `feat!:` / `BREAKING CHANGE:`.

## Documentation gates

- **doc-coverage** (`make doc-coverage` / `internal/cmd/doccover`): satisfied by verified
  `ExampleGuard` and `ExamplePerform` in `example_test.go`. No `baseline.txt` edit (we add
  examples rather than exempt symbols).
- **godoclint require-doc**: both new exported functions carry doc comments (presence gated
  at 100%).

## Coverage gate

`internal/cmd/covercheck` floor for package `github.com/rshade/ax-go` is **80%**. The
truth-table tests for `guard.go` (all rows of both tables, plus the stderr-capture and
determinism tests) keep the new file near 100% and the package floor satisfied.

## Non-goals (explicit)

- No `contract.Guard` / `contract.Perform` (would break `contract` import-isolation).
- No new flag, env var, or envelope field.
- No change to how `--dry-run` resolves into context.
- No value-returning generic variant.
