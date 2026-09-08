# Research: Agent-Safety Context Reaches Every Command in the Tree

**Feature**: `026-persistent-hook-context` | **Date**: 2026-09-08

**Decision Records Absorbed**: **N/A.** No ADR governs `Execute`'s
persistent-hook composition. ADR-0008 fixes Cobra as the CLI framework and
is touched context, not a governing decision — this feature corrects how
`ax.Execute` composes with an existing, already-documented Cobra mechanism;
it does not revisit the framework choice. No ADR is absorbed or retired.

All Technical Context questions are resolved below; no `NEEDS CLARIFICATION`
markers remain.

## D1 — Confirming Cobra's actual dispatch contract

**Decision**: Read `cobra@v1.10.2`'s `Command.execute()` directly rather than
assume behavior. Confirmed:

```go
parents := make([]*Command, 0, 5)
for p := c; p != nil; p = p.Parent() {
    // ... without EnableTraverseRunHooks: append(parents, p)
}
for _, p := range parents {
    if p.PersistentPreRunE != nil {
        if err := p.PersistentPreRunE(c, argWoFlags); err != nil { return err }
        if !EnableTraverseRunHooks { break }
    } else if p.PersistentPreRun != nil {
        p.PersistentPreRun(c, argWoFlags)
        if !EnableTraverseRunHooks { break }
    }
}
```

Two facts this settles:

1. `parents` starts at `c` itself (the invoked command) and walks upward via
   `p.Parent()`. Without `EnableTraverseRunHooks`, Cobra stops at the first
   `p` with a non-nil hook — which may be `c` itself, not only an ancestor.
2. Whichever `p.PersistentPreRunE`/`PersistentPreRun` is found, it is always
   called with `c` (the originally invoked command) as its first argument —
   never `p`. A hook attached to a group command still receives the actual
   leaf command being run.

**Rationale**: Fact 2 is what makes reusing the exact same context-setup
closure body at every wrapped level correct without parameterizing it by
"which node owns this hook" — the closure already receives the right `cmd`
to call `cli.LookupFlagString`/`SetContext` on, because that argument is
always the invoked command, supplied by Cobra itself.

**Alternatives considered**:

- Assume Cobra passes the hook-owning command and thread the invoked command
  through some other channel — rejected once source-reading showed it is
  unnecessary; Cobra already does the right thing here.

## D2 — Which commands need wrapping

**Decision**: Wrap two categories of command, discovered by one tree walk
during `prepareCommand`:

1. **`root`, unconditionally** — the existing universal fallback for any
   invoked command whose ancestor chain has no closer hook.
2. **Any other command that already declares its own `PersistentPreRun` or
   `PersistentPreRunE` at wrap time** — because Cobra's `parents` walk (D1)
   stops at the *first* node with a hook starting from the invoked command
   itself, only nodes that already have a hook can ever become that
   stopping point instead of root.

A command with no hook of its own is never wrapped, because Cobra's own walk
skips straight past it looking for the nearest node that does have one —
touching it would add an unused annotation for no behavioral effect.

**Rationale**: This is the minimal set that restores the invariant
(agent-safety context available for every invoked command) without
guessing at adopters' tree shapes or adding wrapping overhead to commands
that were never at risk.

**Alternatives considered**:

- Wrap every command in the tree unconditionally — rejected; harmless but
  wasteful (mutates every command's `Annotations` map even when it can
  never become Cobra's dispatch target), and the issue's own proposed
  direction already names "wrap every command that declares a persistent
  hook."
- Set `cobra.EnableTraverseRunHooks` instead of wrapping conditionally —
  rejected per the issue and spec: that flag changes the *adopter's own*
  hook semantics tree-wide (all ancestors' hooks would now run, not just the
  nearest), which is a decision this library has no business making for an
  adopter who never asked for it.

## D3 — Idempotent wrapping (the annotation)

**Decision**: Mark each wrapped command with a new Cobra annotation key,
`github.com/rshade/ax-go/execute/persistent-hook-wrapped`, set to `"true"`
immediately after wrapping. `wrapCommandPersistentPreRun` checks this
annotation first and returns immediately if already set.

**Rationale**: `Execute` can run more than once against the same long-lived
command tree object — the codebase's own MCP server path already documents
"the same tree is also served by MCP and tool calls may overlap"
(`trace.go`'s existing comments on a related concern). Without a marker, a
second `Execute` call would capture the *already-wrapped* function as
`previousE`/`previous` and wrap it again, growing an `O(n)`-deep closure
chain per repeated call — functionally tolerable today (each layer still
calls the next until the original hook fires once), but wasteful, and the
issue explicitly flags this as sharing the same annotation mechanism as a
separate, out-of-scope repeat-`Execute` idempotence defect (tracked
elsewhere). Guarding uniformly — including root — with one marker avoids an
inconsistent asymmetry (root exempt from the guard, everything else not) and
costs nothing extra to implement once the per-command helper exists.

Verified this cannot leak into `__schema` output: `schema.CommandSchema` (the
JSON-serialized shape) has no raw `Annotations` field, and
`internal/schema.NonDeterministicFields` reads only its own two named keys
(`envelope`, `non-deterministic-fields`), never the full annotation map.

**Alternatives considered**:

- A package-level `map[*cobra.Command]bool` registry — rejected by
  Constitution Principle X (no mutable package-level state).
- A sentinel wrapper type / function-pointer comparison to detect "already
  ours" — rejected as more fragile and less idiomatic in this codebase than
  the existing `Annotations`-based marker pattern `internal/schema` already
  established for an unrelated purpose.
- Skip idempotency entirely, leaving the existing nested-closure tolerance —
  rejected; FR-006 requires it explicitly, and the fix already needs a
  per-command wrap step where the check costs one map lookup.

## D4 — Preserving exact hook-invocation semantics

**Decision**: `wrapCommandPersistentPreRun` captures `previousE :=
cmd.PersistentPreRunE` and `previous := cmd.PersistentPreRun` for the
command being wrapped, installs a new `PersistentPreRunE` that performs ax's
context setup first and then calls `previousE` (if non-nil, propagating its
error) followed by `previous` (if non-nil) — byte-for-byte the same
call-and-propagate structure the existing root-only implementation already
uses. `cmd.PersistentPreRun` is left in place, unread by Cobra once
`PersistentPreRunE` is non-nil (Cobra's own `if ... else if` in D1 already
prefers `PersistentPreRunE`), exactly matching today's root behavior, which
never clears `root.PersistentPreRun` either.

**Rationale**: Byte-for-byte reuse of the proven call-and-propagate pattern
means the only new failure surface is the tree walk and the idempotency
check — not a reimplementation of hook semantics.

**Alternatives considered**:

- Explicitly nil out `cmd.PersistentPreRun` after capturing it — rejected;
  unnecessary (Cobra never reads it once `PersistentPreRunE` is set) and
  inconsistent with the existing root implementation, which does not do
  this either.

## D5 — Test strategy

**Decision**: Add a table-driven test in `execute_test.go` over five hook
shapes (child `PersistentPreRun`, child `PersistentPreRunE`, both on one
command, grandchild-only, parent-and-child both), each asserting all four
context values match a no-hook baseline captured in the same table. Add
focused tests for: a `Guard`-wrapped side-effect counter staying at zero
under `--dry-run` for every shape; a `Confirm` outcome test approved under
`--yes` for every shape; a success-envelope assertion that `meta` carries
`dry_run`/`idempotency_key` correctly; and a repeated-`Execute`-call test
proving the second call does not re-wrap (adopter hook still fires exactly
once total, not twice).

**Rationale**: These map directly to the spec's acceptance scenarios, the
issue's own testing-strategy section, and FR-008. Table-driven fits the five
hook shapes cleanly since they share the same assertion shape with only the
tree construction differing per case.

FR-007 (the success envelope's `meta` staying correct in every hook shape)
is deliberately **not** re-verified end-to-end for all five shapes. Reading
`contract.MetadataFromContext`'s body shows `Envelope.Meta.DryRun` and
`Envelope.Meta.IdempotencyKey` are populated from exactly the same
`DryRunFromContext`/`IdempotencyKeyFromContext` reads the per-shape table
already asserts, with no intermediate transformation between them. Per-shape
context-value correctness (D5's table) therefore guarantees per-shape
envelope correctness by construction; the one explicit end-to-end envelope
assertion in the combined `--dry-run --yes` scenario (User Story 3) is a
spot check on the composition itself, not a second independent proof
obligation per shape.

FR-004 and FR-005 (must-not-change `Guard`/`Confirm`/`DryRunFromContext`;
must never set `cobra.EnableTraverseRunHooks`) are constraints on what the
implementation does *not* do, not new behavior — verified by the scope
review in the final cross-cutting task (an explicit `grep` for
`EnableTraverseRunHooks`, plus confirming the diff touches no file for the
three named helpers) rather than a dedicated behavioral test.

**Alternatives considered**:

- One test function per hook shape instead of a table — rejected; the five
  shapes share identical assertions with only tree topology varying, the
  canonical table-driven case per this repository's testing conventions.
- Add a benchmark for the tree walk — rejected; `prepareCommand` is not a
  tracked hot path and this feature asserts no performance target.
