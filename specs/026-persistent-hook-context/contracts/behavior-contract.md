# Behavior Contract: Agent-Safety Context Reaches Every Command in the Tree

**Feature**: `026-persistent-hook-context` | **Date**: 2026-09-08

This feature changes no exported Go signature. The contract that changes is
`ax.Execute`'s **runtime behavior** for command trees where a subcommand
declares its own persistent hook — a contract not previously documented
anywhere, because the shadowing defect this feature fixes was never a
supported or described scenario.

## Surface delta (root package `ax`)

None. `Execute`, `ExecuteOption`, and every other exported signature are
unchanged. `wrapPersistentPreRun` and the new `wrapCommandPersistentPreRun`
are both unexported implementation details of `execute.go`.

## Behavioral contract

| Contract ID | Requirement |
|-------------|-------------|
| PH-01 | For any command in a tree passed to `ax.Execute`, `ModeFromContext`, `DryRunFromContext`, `ApprovalFromContext`, and `IdempotencyKeyFromContext` return the same values whether or not that command or any ancestor declares its own persistent hook. |
| PH-02 | `ax.Guard`-wrapped side effects are suppressed under `--dry-run` for a command reached through any subcommand hook shape (child hook, grandchild-only hook, or both parent and child). |
| PH-03 | `ax.Confirm` returns approved under `--yes` for a command reached through any subcommand hook shape. |
| PH-04 | An adopter's own persistent hook (either form, or both declared on the same command) runs exactly once per command execution, with its existing error-propagation behavior unchanged. |
| PH-05 | The success envelope's `meta.dry_run` and `meta.idempotency_key` fields are correct for every hook shape. |
| PH-06 | A command tree already prepared by one `Execute` call is not re-wrapped or double-invoked by a subsequent `Execute` call against the same tree object. |
| PH-07 | `cobra.EnableTraverseRunHooks` is never set by this library. |
| PH-08 | The export and behavior are identical in all four supported build configurations and six surface profiles (the fix lives in untagged `execute.go`). |

## Behavior change from today

| Scenario | Before this fix | After this fix |
|----------|------------------|-----------------|
| Root-only hook (or no hook anywhere) | Correct | Unchanged — no behavior difference |
| A subcommand group declares its own persistent hook | Agent-safety context silently empty/false for that subtree; `--dry-run` fails open, `--yes` fails closed | Agent-safety context correct for that subtree, matching the no-hook baseline |
| A subcommand group's own hook returns an error | Runs, with a broken context, but the error still propagates | Runs identically — error still propagates — but now with a correct context available to it too |

## Stability and release classification

- **Go API**: no change; both touched functions are unexported.
- **Machine payloads**: unchanged shape; `meta.dry_run`/`meta.idempotency_key`
  become *correct* in the previously-broken scenario, not differently
  structured.
- **SemVer**: non-breaking `fix:`; while pre-v1, this rides the minor digit
  per Constitution Principle XI (a `0.x` release MAY fix behavior). Relying
  on the shadowing defect was never a documented, coherent scenario.
- **Deprecation**: none.

## Non-goals

- No change to `ax.Guard`, `ax.Confirm`, or `DryRunFromContext` semantics.
- No new exported type, constant, flag, or `ExecuteOption`.
- Setting `cobra.EnableTraverseRunHooks` — explicitly rejected (see
  research.md D2).
- The separate repeat-`Execute` idempotence defect the issue references
  (tracked elsewhere) — this feature's annotation mechanism happens to also
  guard against re-wrapping on a repeated `Execute` call (PH-06), but no
  additional scope beyond what FR-006 already requires is taken on here.
