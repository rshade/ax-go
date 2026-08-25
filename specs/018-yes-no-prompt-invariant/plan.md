# Implementation Plan: --yes no-prompt invariant

**Branch**: `018-yes-no-prompt-invariant` | **Date**: 2026-08-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/018-yes-no-prompt-invariant/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Add `--yes` as the fourth auto-mounted agent-safety primitive and expose a
stateless public confirmation decision gate. `ax.Execute` and the direct MCP
dispatcher install and resolve the same persistent flag; the resolved approval
bit is carried in the per-run context. `ax.Confirm` returns one of three typed
outcomes: approved, blocked, or prompt-required. Blocked machine-mode calls
return the existing `ax.Error` envelope with `error_code`
`confirmation_required`, `actionable_fix` naming `--yes`, and exit code 2.
Human-mode prompt-required calls return normally so the adopting CLI can own
the actual prompt. MCP supplies approval as an isolated per-call boolean.

## Technical Context

**Language/Version**: Go 1.26.5, module `github.com/rshade/ax-go`, package `ax`

**Primary Dependencies**: Existing Cobra/pflag command tree and OTel context;
no new dependency. Existing `contract` package remains the isolated carrier
surface, while the root package owns the confirmation gate and API aliases.

**Storage**: N/A. Approval is per invocation and held only in
`context.Context`; no persistent or package-level state.

**Testing**: Go table-driven unit tests, golden/error-envelope assertions,
MCP dispatcher integration tests, a verified `ExampleConfirm`, schema tests,
stdin/stream-separation tests, and the required race/vet/lint/doc/surface
gates.

**Target Platform**: Go CLI consumers on the repository's existing supported
platform/build-tag matrix. No platform-specific implementation is introduced.

**Project Type**: Cross-cutting Go library and Cobra CLI runtime facade, with a
runnable integration example and an MCP server adapter.

**Performance Goals**: The gate and context lookup are constant-time and add no
I/O. Preserve existing command and MCP dispatch behavior; no numeric target is
claimed, so no new benchmark is required.

**Constraints**: stdout remains payload-only; refusal envelopes go to stderr;
the gate never reads stdin or performs prompt/pager/editor/spinner I/O; blocked
confirmation is exit 2; dry-run and approval remain orthogonal; contexts and
MCP calls must not share approval state; existing envelope fields and schema
reflection behavior remain stable except for the additive flag and error-code
value space.

**Scale/Scope**: Small cross-cutting change across the root execution wrapper,
shared internal flag helpers, isolated context contract, MCP dispatcher,
integration example, documentation, and tests. No new public package.

**Governing ADR(s)**: N/A. The feature is governed by Constitution Principles
I–IV, VI–VII, IX–XI; frozen ADRs contain no confirmation/prompt decision.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle / gate | Design evaluation |
|---|---|
| I. Stream Separation | PASS. `Confirm` performs no output on approved or prompt-required paths; the blocked error is returned for `Execute`/MCP to serialize on stderr, and the gate never writes stdout. |
| II. Deterministic Output & Exit Codes | PASS. `confirmation_required` always maps to `ExitValidation` (2); the error uses the existing struct envelope and deterministic fixed remediation text. |
| III. Machine Discoverability | PASS. `--yes` is installed as an ordinary persistent Cobra flag and is therefore reflected by the existing schema path without special cases; existing golden schema shape changes only by the intended additive flag. |
| IV. Agent-Safety Primitives | PASS. Approval joins the existing per-run context resolution, auto-mounted flags, dry-run composition, and MCP per-call forwarding. No implicit approval or environment fallback is added. |
| V. Asymmetric JSON I/O | PASS / N/A. No Hujson or config write path changes. |
| VI. ADR-Governed Scope | PASS. This is a cross-cutting agent-safety primitive specified in this feature; no domain command, persistence, second framework, or new package is added. |
| VII. Test-First Discipline | PASS. Tasks must add failing table-driven gate, context, Execute, MCP, stream, stdin, schema, golden, and Example tests before implementation; required repository gates remain in scope. |
| VIII. Observability & ID Discipline | PASS. The feature creates no new IDs or logger/backend; existing context trace metadata continues through returned errors. |
| IX. Security & Resource Safety | PASS. The gate is fail-closed for nil/state-free contexts, performs no blocking I/O, and returns errors rather than panicking. Piped stdin remains untouched. |
| X. Idiomatic Go & Dependency Minimalism | PASS. Uses existing context/Cobra/pflag/error abstractions, constant-time lookup, and no dependency or mutable global. |
| XI. Stability & SemVer | PASS. The new exported gate/context symbols and `--yes` flag are additive; the error envelope field set is unchanged and the new error-code value is additive-tolerant. |
| XII. Deprecation Lifecycle | PASS / N/A. No exported symbol is removed or deprecated. |

No gate failures require complexity justification. The ADR absorption gate is
satisfied vacuously because no governing ADR exists.

**ADR absorption gate (Constitution §Governance)**: Governing ADRs are `N/A`,
so no decision-record absorption or ADR-retirement task is required.

### Post-Design Re-check

PASS. Phase 1 keeps the implementation within the approved root `ax` facade,
the existing import-isolated `contract` carrier, and the private MCP
dispatcher. The design adds only the specified public gate/context symbols and
the `--yes` schema-visible flag, leaves the `ax.Error` field set and success
envelopes unchanged, preserves stderr/stdout separation, and introduces no
dependency, persistence, mutable global, ADR, or unresolved clarification.
The required tests cover the three outcomes, nil/fail-closed behavior,
dry-run orthogonality, collision handling, schema reflection, MCP type
validation/isolation, stdin preservation, and deterministic refusal output.

## Project Structure

### Documentation (this feature)

```text
specs/018-yes-no-prompt-invariant/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output; MUST hold "Decision Records Absorbed" for any governing ADR(s)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)
```text
context.go                         # root aliases for approval context accessors
confirm.go                         # public ConfirmationOutcome and Confirm gate
execute.go                         # --yes installation and pre-run resolution
internal/cli/cli.go                # shared FlagYes constant and bool installer
contract/context.go                # isolated approval context carrier
internal/mcpserver/dispatch.go     # per-call yes validation/threading
examples/integration/main.go       # canonical gated command example
example_test.go                     # verified primary-API example
confirm_test.go, context_test.go   # table-driven gate/carrier tests
execute_test.go, schema_test.go    # Execute/schema/stream contract tests
internal/mcpserver/dispatch_test.go # MCP approval and isolation tests
specs/018-yes-no-prompt-invariant/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── public-api.md
    ├── error-envelope.md
    └── mcp-confirmation.md
```

**Structure Decision**: Keep the implementation in the root `ax` facade,
the existing import-isolated `contract` carrier, and the existing private MCP
dispatcher. The shared `internal/cli` helper remains the single source of
truth for auto-mounted flag names and collision behavior. Tests stay beside
their package code; the integration command remains the user-facing example.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | The constitution check passes without an exception. |
