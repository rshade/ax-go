# Data Model: Execute Shutdown Flush Hook

**Feature**: `024-execute-flush-hook` | **Date**: 2026-09-01

This feature persists no data. Its model is one per-`Execute` configuration
value and a deterministic shutdown state transition.

## Execute configuration delta

| Field | Type | Default | Set by | Contract |
|-------|------|---------|--------|----------|
| `flushFunc` | `func(context.Context) error` | `nil` | `WithFlushFunc` | Last option wins; nil means no invocation. |
| `shutdownTimeout` | `time.Duration` | existing telemetry default | `WithTelemetryShutdownTimeout` | Duration applied independently to flush and telemetry shutdown contexts. |

`executeConfig` also carries a private per-execution `shutdownTelemetry`
function that defaults to `Telemetry.Shutdown`. It is an implementation test
seam rather than public option state, so it does not participate in the option
resolution table below.

No callback slice, registry, logger value, result field, or package-level state
is introduced.

## Option resolution

| Options in call order | Final `flushFunc` | Invocations |
|-----------------------|-------------------|-------------|
| none | nil | 0 |
| `WithFlushFunc(nil)` | nil | 0 |
| `WithFlushFunc(A)` | A | A once |
| `WithFlushFunc(A), WithFlushFunc(B)` | B | B once; A never |
| `WithFlushFunc(A), WithFlushFunc(nil)` | nil | 0 |

## Lifecycle state machine

```text
configured
    |
    v
Cobra executing
    |
    +-- success --------------------+
    |                               |
    +-- parse/pre-run/RunE error ---+
                                    v
                            exit code determined
                                    |
                                    v
                              root span ended
                                    |
                    +---------------+---------------+
                    | flushFunc nil                 | flushFunc set
                    |                               v
                    |                    fresh bounded flush context
                    |                               |
                    |                    callback returns nil/error
                    |                               |
                    |                    error -> sanitized stderr only
                    +---------------+---------------+
                                    v
                         fresh bounded telemetry context
                                    |
                                    v
                           telemetry shutdown/diagnostic
                                    |
                                    v
                       return previously determined code
```

## Callback context

| Property | Value |
|----------|-------|
| Parent | `context.Background()` |
| Deadline | Invocation time + `shutdownTimeout` |
| Cancellation | Deferred cancel in the flush defer; deadline also cancels |
| Command values | Not inherited |
| Command cancellation | Not inherited |
| Relationship to telemetry context | Same duration, separate context and deadline |

The callback is synchronous. `Execute` does not start a goroutine and does not
recover a panic originating in caller-supplied callback code, matching Cobra's
existing treatment of a panicking `RunE`. Because flush and telemetry are
separate defers, telemetry shutdown still runs during panic unwinding. Library
code itself adds no panic.

## Result and diagnostic matrix

| Command result | Callback result | stdout | stderr addition | Returned code |
|----------------|-----------------|--------|-----------------|---------------|
| success | nil | unchanged success payload | none | `0` |
| classified error | nil | unchanged (normally empty) | existing error envelope only | original `1`/`2`/`3`/`4` |
| success | error | unchanged success payload | `ax: flush failed: <sanitized>\n` | `0` |
| classified error | error | unchanged | existing error envelope, then flush diagnostic | original `1`/`2`/`3`/`4` |
| any | context deadline error | unchanged | sanitized flush diagnostic | original code |

## Diagnostic transformation

Input error text:

```text
push failed\nforged: line\t\x1b[31mred\x7f
```

Emitted framework line:

```text
ax: flush failed: push failed forged: line  [31mred 
```

Every ASCII code point below `0x20` and DEL (`0x7f`) becomes a space through the
existing sanitizer. Non-control Unicode text is preserved. Sanitization is not
redaction or length truncation.

## Integration-example ownership

| Value | Created | Used | Lifetime |
|-------|---------|------|----------|
| `logger` closure variable | root-command constructor | mutex-protected assignment in root `RunE`; mutex-protected read by returned flush closure | one constructed command |
| invocation logger | each root `RunE` | local emission for that invocation | one command invocation |
| `root` | root-command constructor | passed to `Execute` | one execution |
| `flush` closure | root-command constructor | passed to `WithFlushFunc` | one execution shutdown |

If a selected subcommand never assigns the root logger, the closure passes nil
to `ax.Flush`; the established nil-safe contract returns nil immediately.
Protecting the shared shutdown reference avoids a race when the same tree serves
overlapping MCP calls; each call still emits through its own local logger.

## Invariants

- Callback count is always 0 or 1 per `Execute` invocation.
- Callback failure never changes stdout bytes, an `ax.Error` value, or exit code.
- Telemetry always receives its own complete configured timeout window.
- Omitting the option produces the pre-feature lifecycle.
- Public callback presence and signature are identical across all build tags and
  supported target profiles.
