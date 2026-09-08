# Data Model: Agent-Safety Context Reaches Every Command in the Tree

**Feature**: `026-persistent-hook-context` | **Date**: 2026-09-08

This feature persists no data and adds no new type. Its model is a
command-tree walk and one new marker value stored on an existing Cobra
field.

## New state: the wrapped-command marker

| Property | Value |
|----------|-------|
| Storage | `cobra.Command.Annotations["github.com/rshade/ax-go/execute/persistent-hook-wrapped"]` |
| Type | `string`, value always `"true"` when present |
| Lifetime | Scoped to the in-memory `*cobra.Command` object; never serialized, never persisted across process restarts |
| Written by | `wrapCommandPersistentPreRun`, once per command, the first time it is wrapped |
| Read by | `wrapCommandPersistentPreRun`, to skip re-wrapping a command already marked |
| Visible to `__schema` / MCP output? | No — verified `internal/schema.NonDeterministicFields` reads only its own two named keys, and `schema.CommandSchema` carries no raw `Annotations` field |

## Tree-walk classification

For a given command tree rooted at `root`, every command `cmd` falls into
exactly one of three classes at wrap time:

| Class | Condition | Wrapped? |
|-------|-----------|----------|
| Root | `cmd == root` | Always (existing behavior, now also idempotency-guarded) |
| Self-hooked | `cmd != root` and (`cmd.PersistentPreRun != nil` or `cmd.PersistentPreRunE != nil`) at wrap time | Yes |
| Hookless | `cmd != root` and neither field is set | No — Cobra's own nearest-ancestor search will never stop here |

## Invocation data flow (per command execution)

```text
Cobra selects the invoked command `c`
    |
    v
Cobra walks p := c, c.Parent(), c.Parent().Parent(), ... up to root
    |
    v
Cobra finds the first p with p.PersistentPreRunE != nil (or PersistentPreRun)
    |
    v
Cobra calls p.PersistentPreRunE(c, args)   <-- note: c, not p
    |
    v
ax's wrapped hook (installed on p) executes, using `c` (the invoked command)
for every flag lookup and context mutation:
    mode        := ResolveMode(LookupFlagString(c, format), env, ttyState)
    dryRun      := LookupFlagBool(c, dry-run)
    approval    := LookupFlagBool(c, yes)
    idempotency := LookupFlagString(c, idempotency-key) or NewIdempotencyKey()
    ctx := WithMode/WithDryRun/WithApproval/WithIdempotencyKey(c.Context(), ...)
    c.SetContext(ctx)
    |
    v
ax's wrapped hook then calls p's own originally-declared hook (if any),
still passing `c`, propagating any error
    |
    v
Cobra proceeds to c.PreRunE / c.RunE with c.Context() now carrying
correct agent-safety state, regardless of which ancestor `p` fired
```

## Invariants

- Every invoked command's context carries the same four agent-safety values
  (mode, dry-run, approval, idempotency key) regardless of which ancestor in
  its chain — including itself — happens to be the one Cobra dispatches.
- A command's own originally-declared persistent hook (of either form) runs
  exactly once per command execution, with its existing error-propagation
  behavior unchanged.
- A command already wrapped by ax is never wrapped a second time by a later
  `Execute` call against the same tree object.
- No command's `Annotations` map gains a new visible entry in `__schema` or
  `__schema --as=mcp` output as a result of this feature.
