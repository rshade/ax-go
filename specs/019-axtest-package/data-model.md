# Phase 1 Data Model: axtest — Full-Lifecycle Command Test Helper

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) |
**Research**: [research.md](./research.md)

`axtest` has no persisted state and no domain data of its own — it wraps an
existing lifecycle and reshapes its output for a test. The two entities named
in spec.md's Key Entities section are both transient values scoped to one
test invocation.

## Entity: Execution Result

What one full-lifecycle run of a command tree produces, considered together
because a test may need any combination of the three fields to make its
assertion (spec FR-002).

| Field | Type | Description | Validation / Invariants |
|---|---|---|---|
| `Stdout` | `[]byte` | The command's captured machine-payload stream — exactly what `ax.Execute` wrote via `ax.WithStdout` during this run. | May be empty (a run that failed before producing a payload, per the spec's Edge Cases). Never `nil` vs. empty-slice is not a meaningful distinction a caller should branch on. |
| `Stderr` | `[]byte` | The command's captured diagnostic stream — logs, an `ax.Error` envelope on failure, or nothing on a quiet success. | May be empty. This is the only place a blocked-confirmation or validation-error envelope is observable (Principle I: errors go to `stderr`, never `stdout`). |
| `ExitCode` | `int` | The deterministic exit code `ax.Execute` returned. | One of the five documented codes (`0` success, `1` internal, `2` validation, `3` network/timeout, `4` auth/permission) — `axtest` does not itself constrain this value; it passes through whatever `ax.Execute` returned, unchanged. |

**Relationships**: Produced exclusively by the `Run` operation. Consumed by a
test's own assertions directly, or handed to the **Typed Result** decode step
via its `Stdout` field.

**Lifecycle**: Constructed fresh on every `Run` call; carries no identity and
is never mutated after construction. Two `Run` calls against the same command
tree produce two independent `Execution Result` values (see research.md's
flag re-mounting decision — the underlying tree's flag *values* may carry
state between calls, per ordinary command-framework behavior, but the
returned `Execution Result` values themselves are independent).

## Entity: Typed Result

A caller's own result type (`T`, a type parameter — this is generic over
whatever payload shape the command under test actually produces), populated
by decoding the data an Execution Result's `Stdout` field carries, with the
enclosing envelope already removed (spec FR-005).

| Aspect | Description |
|---|---|
| Source | The `Stdout` field of an `Execution Result`, or any `[]byte` a caller already has in the same shape (the decode step does not require its input to have come from `Run`). |
| Shape it unwraps | The envelope's `data` field — the same shape `ax.NewEnvelope[T]` produces on the production side, so a type a command's handler already uses as its payload is the same type a test decodes into; no separate "test DTO" is needed. |
| Failure mode | If `Stdout` does not parse as that envelope shape, no `Typed Result` is produced — the calling test fails immediately and with a clear cause (FR-006) rather than receiving a zero-valued `T` that could be silently misread as a real (if empty) result. |

**Relationships**: Depends on an `Execution Result` (or an equivalent
`[]byte`) as its input; has no relationship to anything else. It is not
persisted, cached, or reused across calls.

**State transitions**: None — a `Typed Result` is a one-shot decode, not a
value with a lifecycle of its own.

## Non-entities (explicitly out of scope for this model)

- **No envelope metadata type is introduced.** `ax.Envelope[T]`
  (`contract.Envelope[T]`) already exists and is reused as-is; axtest does not
  wrap or extend it.
- **No configuration/options type beyond what `ax.Execute` already accepts**
  (`ax.ExecuteOption`) is introduced. `axtest.Run` and `axtest.RunAndDecode`
  forward any supplied options straight through.
