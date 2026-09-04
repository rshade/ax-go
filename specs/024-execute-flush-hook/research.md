# Research: Execute Shutdown Flush Hook

**Feature**: `024-execute-flush-hook` | **Date**: 2026-09-01

**Decision Records Absorbed**: **N/A.** Constitution Principles I, II, VI–XI
govern the lifecycle, stream, resource-safety, observability, and stability
contracts directly. ADR-0008 selects Cobra and notes that `ax.Execute` wraps it;
this feature leaves that choice and integration semantics intact, so the ADR is
touched context rather than a governing decision. No ADR is retired.

All Technical Context questions are resolved below; no `NEEDS CLARIFICATION`
markers remain.

## D1 — Public API shape

**Decision**: Add exactly one root-package function:

```go
func WithFlushFunc(flush func(context.Context) error) ExecuteOption
```

The callback type is not named and `Execute`'s signature is unchanged.

**Rationale**: The function shape matches issue #119, composes directly with a
closure around `ax.Flush(ctx, logger)`, and permits other bounded drains without
coupling `Execute` to `Logger` or Loki. A functional option is the established
configuration seam in `execute.go` and is additive under Constitution Principle
XI.

**Alternatives considered**:

- `WithFlush(logger Logger)` — rejected because the logger is normally created
  inside `RunE` from `cmd.Context()` and `cmd.ErrOrStderr()`, after `Execute` has
  installed trace and locked-writer state. Requiring early construction would
  weaken that integration and couple the lifecycle facade to logging.
- Change `Execute`'s positional signature — rejected as a needless breaking
  change.
- Add a named `FlushFunc` type — rejected; it adds surface without improving the
  one-callback contract.
- Add a general shutdown-hook registry — rejected as speculative scope. A caller
  needing multiple drains can aggregate them in its callback.

## D2 — Option resolution and nil behavior

**Decision**: The public option state in `executeConfig` gains one field:

```go
flushFunc func(context.Context) error
```

Each `WithFlushFunc` assignment replaces the field. The final option wins. A
nil callback therefore disables invocation and can clear an earlier callback.

The implementation also carries a private per-execution `shutdownTelemetry`
function field that defaults to `Telemetry.Shutdown`. It is not set by a public
option and does not change option resolution; it lets package tests observe the
second deadline directly without mutable package-level hooks.

**Rationale**: This is ordinary functional-option behavior and keeps the state
model deterministic: zero or one callback, invoked zero or one time. It avoids a
hidden slice whose ordering and partial-failure semantics the issue did not ask
to define.

**Alternatives considered**:

- Append every callback — rejected; it silently turns a singular option into a
  registry and raises ordering/error-aggregation questions.
- Ignore nil assignments while retaining an earlier callback — rejected; that
  violates last-option-wins and makes configuration harder to override.
- Return an error for nil — rejected; `ExecuteOption` has no error channel and
  nil is naturally equivalent to no registered callback.

## D3 — Lifecycle position and defer order

**Decision**: Register a dedicated flush defer after the existing telemetry
shutdown defer and before registering `span.End`. Runtime order is:

1. Cobra execution completes and the success/error exit code is determined.
2. The root command span ends through its later-registered defer.
3. If registered, the flush callback runs once.
4. Telemetry shutdown runs with its existing full timeout window.
5. `Execute` returns the previously determined exit code.

The callback runs on Cobra argument, pre-run, and `RunE` failures because every
normal return from `root.ExecuteContext` crosses the same defer stack. Keeping
flush separate also ensures the earlier telemetry defer still executes when
caller-supplied callback code panics during unwinding; no panic is recovered.

**Rationale**: `Execute` is the only boundary that observes all command outcomes
and already owns deterministic bounded shutdown. Flush-first drains application
logs before the final telemetry lifecycle closes. The explicit registration
order preserves current root-span duration semantics.

**Alternatives considered**:

- Keep a command-local `RunE` defer — rejected; it misses parse/pre-run failures,
  must be repeated, and was the original ergonomic defect.
- Put flush and telemetry statements in one defer — rejected because a panicking
  callback would skip later telemetry statements in that defer.
- Register the flush defer outside the telemetry defer — rejected because LIFO
  order would shut telemetry down before draining application logs.
- Start a goroutine — rejected; it would make drain completion nondeterministic
  and introduce race/leak risk at process exit.

## D4 — Context and timeout budget

**Decision**: For a registered callback, construct:

```go
flushCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
defer cancel()
flushErr := cfg.flushFunc(flushCtx)
```

The telemetry defer separately constructs its existing background-derived
timeout context of the same duration. `cfg.shutdownTimeout` remains configured
by `WithTelemetryShutdownTimeout` and defaults to the existing telemetry
timeout. Existing non-positive-duration behavior is left unchanged: the outer
Execute shutdown contexts are immediately expired; this feature does not
silently normalize or redefine that pre-existing option.

**Rationale**: A fresh context is not already canceled by command execution.
The deadline prevents a cooperative shutdown hook from waiting indefinitely.
Separate contexts preserve telemetry's current full budget even when flush
consumes its entire window, satisfying the additive/no-regression constraint
without adding a second public timeout knob.

**Alternatives considered**:

- Pass `root.Context()` or the caller's `ctx` — rejected because either may be
  canceled before shutdown, defeating the primary drain.
- Use one shared deadline for flush and telemetry — rejected because a slow
  flush could starve the existing telemetry lifecycle.
- Use unbounded `context.Background()` — rejected because a callback could hang
  cooperative process shutdown.
- Add `WithFlushTimeout` — rejected as unnecessary public surface; issue #119
  explicitly permits reuse of the existing budget duration.
- Use the internal Loki two-second ceiling as the only bound — rejected because
  `WithFlushFunc` is generic and must not assume every callback is `ax.Flush`.

## D5 — Error, diagnostic, and exit semantics

**Decision**: A non-nil callback error writes exactly:

```text
ax: flush failed: <sanitized error>\n
```

to the mutex-wrapped `cfg.stderr`. Error text passes through
`internal/telemetry.SanitizeDiagnostic`, which replaces ASCII control
characters and DEL with spaces. The error is not returned, wrapped into an
`ax.Error`, added to stdout, or used to change the command exit code.

**Rationale**: This mirrors the existing OTel shutdown branch and `ax.Flush`'s
fail-open contract. It preserves the actual command result as authoritative and
prevents newline/ANSI injection from forging additional diagnostics. The
sanitizer does not truncate or redact, so caller-supplied callback errors must
not contain credentials, secrets, or PII.

**Alternatives considered**:

- Return exit `1` on flush failure — rejected; an observability drain cannot
  rewrite whether the user's operation succeeded or its actual error category.
- Join the flush error with the command error — rejected; it would change the
  public error envelope and deterministic exit mapping.
- Suppress the error — rejected; silent loss is the problem the hook addresses.
- Emit JSON — rejected; this is an operational shutdown diagnostic matching the
  existing OTel diagnostic, not a second machine error envelope.

## D6 — Integration example ownership pattern

**Decision**: Change the private integration `newRootCommand` helper to return
both `*cobra.Command` and `func(context.Context) error`. It owns a closure-scoped
`ax.Logger` variable. Root `RunE` assigns the logger after `Execute` has decorated
the command context and writers; the returned callback later calls
`ax.Flush(ctx, logger)`. `runWithEntityID` registers that callback through
`WithFlushFunc`. Direct test/example call sites ignore the second result when
they use a different lifecycle. Because the same tree is also served by MCP and
tool calls may overlap, the most recent shutdown reference is protected by a
mutex while each invocation continues emitting through its own local logger.

**Rationale**: This removes the manual timeout/defer while preserving late logger
construction, trace correlation, locked stderr routing, label configuration,
and Loki option order without introducing a closure race. MCP call cancellation
retains the existing per-call flush backstop; the Execute callback is the primary
process drain. `ax.Flush(ctx, nil)` is already a documented no-op, so subcommands
that never construct the root logger remain safe.

**Alternatives considered**:

- Construct the logger before `Execute` — rejected because it would miss the
  decorated command context and locked diagnostic writer.
- Store the logger in a mutable package global — rejected by Constitution
  Principles VI/X and unsafe for tests/concurrency.
- Add a private logger-capture callback/wrapper builder — viable, but rejected in
  favor of returning the command's lifecycle companion directly; adapting the
  three private call sites is small and explicit.
- Store the logger in Cobra annotations or a context value — rejected as an
  opaque transport for one private closure.

## D7 — Documentation contract

**Decision**: Demonstrate `WithFlushFunc` inside the existing verified
`ExampleExecute`, not a standalone `ExampleWithFlushFunc`. Replace the manual
defer in the first-CLI tutorial and Loki quickstart; update root README lifecycle
guidance, the logging-surface guide, integration README/AUDIT, and
`Flush`/`WithLokiFromEnv`/`Execute` doc comments to recommend registration on
`Execute` while preserving direct `Flush` for non-Execute lifecycles.

**Rationale**: Constitution Principle VII explicitly places WithX options inside
a parent example. The issue asks for the recommended wiring, and the current
tutorial and Loki quickstart teach the manual `Flush` defer being replaced.

**Alternatives considered**:

- Add only an option doc comment — rejected; users copy the tutorial and
  integration example.
- Remove `ExampleFlush` — rejected; direct `Flush` remains valid for lifecycles
  not owned by `Execute` and for mixed logging surfaces.
- Mechanically rewrite historical feature-017 compatibility examples — rejected;
  they prove cross-surface/direct-Flush identity rather than recommend an
  Execute lifecycle.
- Add a new standalone option example — rejected by the repository's doccover
  convention and unnecessary duplication.

## D8 — Public surface and release classification

**Decision**: Classify `func:WithFlushFunc` as supported/live in the permanent
root surface audit with signature
`func(func(context.Context) error) ExecuteOption`; regenerate and review the
live baseline so the function is present in all configurations and profiles.
Advance the audit review date. No doccover baseline or API allowlist change is
needed. Land as a non-breaking `feat:` so release-please selects the next pre-v1
minor.

**Rationale**: The function is an intentional adopter-facing lifecycle contract,
not an implementation leak. Root `ax` is already an approved public package.
WithX options are demonstrated inside a parent example rather than separately
required by doccover. The source file is untagged, so presence must be universal.

**Alternatives considered**:

- Skip the audit/baseline update — rejected; `make surface-check` must fail on
  unreviewed additions.
- Put the option in `logging` — rejected; `Execute` and Loki direct push are
  deliberately root-only, and `logging` is import-isolated from Cobra/runtime
  dependencies.
- Treat the addition as a patch — rejected by Constitution Principle XI: a
  non-breaking feature is a pre-v1 minor, not a bug-fix-only patch.

## D9 — Test-first and verification matrix

**Decision**: Add failing tests before implementation, organized as:

- Table: success, classified command error, argument parsing failure, and
  persistent-pre-run failure each invoke the callback exactly once and retain
  their expected exit code.
- Table: option absent, option nil, repeated callbacks, and final nil establish
  zero/last-option-wins semantics.
- Table: callback error after success and after command failure preserves stdout
  and exit code while appending the exact sanitized diagnostic to stderr.
- Deadline test: callback observes a fresh live context even when the command
  parent was canceled, has a deadline, and a callback waiting for cancellation
  terminates under a short configured budget.
- Existing telemetry tests and all four build configurations verify parity.
- Integration/example tests compile and run the recommended closure wiring.

Then run the repository gate suite named in `plan.md`. No benchmark is added
because the feature makes no hot-path or numeric performance claim.

**Rationale**: These tests map directly to issue #119's acceptance criteria and
the spec's edge cases while keeping timing assertions bounded and generous under
the race detector.

**Alternatives considered**:

- Test only `ax.Flush` — rejected; its sink behavior is already covered and the
  new contract is `Execute` lifecycle invocation.
- Use a real Loki server for `Execute` unit tests — rejected; a callback seam is
  deterministic and isolates lifecycle behavior, while existing Loki tests
  cover actual draining.
- Add a performance benchmark — rejected; no performance target is asserted and
  shutdown is not a tracked hot path.
