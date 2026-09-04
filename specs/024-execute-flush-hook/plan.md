# Implementation Plan: Execute Shutdown Flush Hook

**Branch**: `024-execute-flush-hook` | **Date**: 2026-09-01 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/024-execute-flush-hook/spec.md`

## Summary

Add the optional root-package function
`WithFlushFunc(func(context.Context) error) ExecuteOption`. `Execute` stores the
last supplied callback and invokes it exactly once from a shutdown defer on every
normal command return path. The callback receives a fresh background-derived
context bounded by the duration configured through
`WithTelemetryShutdownTimeout`; it gets an independent window so it cannot
consume telemetry's existing shutdown budget. Defers are registered so runtime
order is root-span end, flush, then telemetry shutdown. A callback error is
control-character-sanitized and written to the configured `stderr` as
`ax: flush failed: ...`, but never changes stdout, the error envelope, or the
exit code.

The integration example will move logger ownership just far enough outside its
root `RunE` closure to return a late-bound flush closure alongside the command,
then pass that closure to `WithFlushFunc`. The command-local timeout/defer is
removed. The primary `ExampleExecute`, README, Loki quickstart, first-CLI
tutorial, logging guide, integration docs, and relevant logger doc comments will
show the same lifecycle-owned pattern. The additive export is recorded in both
public-surface artifacts.

## Technical Context

**Language/Version**: Go 1.26.7

**Primary Dependencies**: Existing dependencies only — stdlib `context` and
`fmt`; `github.com/spf13/cobra`; existing
`internal/telemetry.SanitizeDiagnostic`. The callback may call the existing
`ax.Flush`, but `Execute` does not import or depend on a logger implementation.
No new module dependency.

**Storage**: N/A — two per-call function fields in `executeConfig`: the selected
flush callback and a private telemetry-shutdown seam used to observe the second
deadline in tests. There is no persistent or package-level state.

**Testing**: Test-first, table-driven additions in `execute_test.go`, including
direct observation that telemetry receives a newly bounded shutdown context
after a deadline-consuming flush; adapt the runnable integration example's
direct private-constructor uses; update the verified `ExampleExecute`. Required
verification: `make test` (race detector across all four build configurations),
`make validate` (gofmt/tidy/vet across the matrix), `make lint`,
`make doc-coverage`, `make cover-check`, `make surface-check`, `make size-check`,
and `make bench-check`. No golden envelope or `__schema` shape changes are
expected.

**Target Platform**: All supported ax-go consumer targets and all surface gate
profiles (`linux`, `darwin`, `windows` × `amd64`, `arm64`), under default,
`ax_no_grpc`, `ax_no_otlp`, and combined build configurations.

**Project Type**: Go library with a runnable Cobra integration example and
Starlight documentation site.

**Performance Goals**: No allocation or latency claim. When the option is
omitted, the flush defer returns without constructing a shutdown context or
invoking caller code. Opted-in work occurs once during `Execute` shutdown and is
outside the tracked hot paths. No benchmark budget or threshold change.

**Constraints**:

- Keep `Execute`'s signature and every existing `ExecuteOption` unchanged.
- The callback is optional, synchronous, invoked once, and stored without global
  state; repeated options use ordinary last-option-wins semantics.
- A final nil callback clears an earlier callback and produces no invocation.
- Use a fresh `context.Background()`-derived timeout context; never pass the
  possibly canceled command context and never permit an unbounded cooperative
  shutdown wait.
- Give flush and telemetry separate contexts with the same configured duration;
  flush executes first but cannot spend telemetry's window.
- Keep flush in its own defer so telemetry's earlier-registered defer still runs
  if caller-supplied flush code panics while the stack unwinds. Do not add panic
  recovery.
- Route only the framework-generated failure diagnostic through the
  mutex-wrapped configured `stderr`; never write it to stdout.
- Sanitize error text through the existing telemetry diagnostic sanitizer. This
  prevents forged control-character lines but is not truncation or redaction;
  callback errors must not contain secrets or PII.
- Callback errors are fail-open and cannot replace the command result.
- Preserve default behavior byte-for-byte when no callback is registered.
- Keep the change root-package-only and build-tag-independent; no declined build
  may lose or re-type the export.
- Update the permanent audit and live baseline deliberately; do not regenerate
  either artifact without reviewing the exact diff.

**Scale/Scope**: One exported option and two unexported config fields (the
callback and its per-execution telemetry test seam); one shutdown defer;
table-driven lifecycle tests; small integration-example constructor/call-site
changes; documentation updates; one live-baseline row and one permanent-audit
row. No new package, flag, environment variable, command, payload field,
dependency, goroutine, logger implementation, or timeout option.

**Governing ADR(s)**: **N/A.** ADR-0008 fixes Cobra as the CLI framework and
mentions that `ax.Execute` wraps Cobra, but this feature neither revisits the
framework choice nor changes Cobra semantics. It is touched context, not a
governing decision. `research.md` records this finding; no ADR is absorbed or
retired.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | PASS | Callback failures write one sanitized diagnostic to configured `stderr`; stdout bytes are unchanged and tested on success and error paths. |
| II. Deterministic Output & Exit Codes | PASS | No payload/envelope change. The command's computed exit code remains authoritative even when flush fails or times out. |
| III. Machine Discoverability via `__schema` | PASS | No command, flag, or schema metadata changes; existing goldens must remain byte-identical. |
| IV. Agent-Safety Primitives | PASS | No idempotency, dry-run, confirmation, or mode behavior changes. The callback runs after all normal command paths, including guarded failures. |
| V. Asymmetric JSON I/O | PASS | No input parsing or machine output encoding changes. |
| VI. ADR-Governed Scope — Library, Not Application | PASS | Bounded lifecycle cleanup around an existing cross-cutting sink is foundation scope; no orchestration, persistence, or new ADR. |
| VII. Test-First Discipline | PASS | Failing table-driven tests precede implementation; verified `ExampleExecute` demonstrates the WithX option; race/vet/lint/doc gates run before handoff. |
| VIII. Observability & ID Discipline | PASS | Enables reliable draining of the existing opt-in Loki sink without adding a backend, logger, label, or telemetry dependency. |
| IX. Security & Resource Safety | PASS | Fresh bounded context prevents cooperative hangs; control characters are sanitized; error text is documented as non-redacted; no library-originated panic path is added. |
| X. Idiomatic Go & Dependency Minimalism | PASS | Context is the callback's first parameter; one functional option fits the existing configuration style; no new dependency or mutable package state. |
| XI. Stability & SemVer | PASS | One additive exported function, no semantic change for callers that omit it. This is a non-breaking `feat:` and therefore a pre-v1 minor release. |
| XII. Deprecation Lifecycle | PASS | No symbol is deprecated, renamed, or removed. |

**ADR absorption gate (Constitution §Governance)**: PASS — Governing ADR(s) =
N/A. ADR-0008 remains untouched and no ADR-retirement task is required.

**Post-design re-check**: PASS. The research, data model, public contract, and
quickstart preserve the existing stream/exit contracts; keep the callback
optional and bounded; make the separate timeout windows and defer order
explicit; and record the surface addition. No complexity exception is needed.

## Project Structure

### Documentation (this feature)

```text
specs/024-execute-flush-hook/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── public-api.md
├── checklists/
│   └── requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
execute.go                 # ADD option, config field, bounded shutdown invocation
execute_test.go            # ADD lifecycle/diagnostic/precedence/deadline tests
example_test.go            # UPDATE ExampleExecute and ExampleFlush guidance
logger.go                  # UPDATE Flush shutdown recommendation
loki.go                    # UPDATE WithLokiFromEnv shutdown recommendation
README.md                  # UPDATE lifecycle guidance and runnable wiring example

examples/integration/
├── main.go                # RETURN late-bound flush closure; remove RunE defer; register option
├── main_test.go           # ADAPT direct root constructor use
├── axtest_example_test.go # ADAPT direct root constructor use
├── README.md              # DOCUMENT Execute-owned Loki drain
└── AUDIT.md               # MAP lifecycle-owned flush evidence

docs/src/content/docs/
├── tutorials/build-your-first-cli.md       # REPLACE manual defer with option
└── guides/choose-a-logging-surface.md       # DOCUMENT root Execute wiring

specs/007-loki-direct-push/quickstart.md     # REPLACE legacy manual wiring
internal/cmd/surfacecheck/baseline.json      # ADD reviewed WithFlushFunc row
specs/023-internalize-helpers/
└── public-surface-audit.json                # ADD supported/live decision row
```

**Structure Decision**: Extend the existing root `ax` execution facade. The
callback is generic and late-bound, so the lifecycle layer does not gain a logger
dependency. Integration-example logger creation stays inside `RunE` so it uses
the decorated command context and mutex-wrapped diagnostic writer; the returned
closure captures the logger variable and is registered before execution.

## Complexity Tracking

*No violations — table intentionally empty.*
