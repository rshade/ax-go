# Tasks: --yes no-prompt invariant

**Input**: Design documents from `/specs/018-yes-no-prompt-invariant/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/`, and `quickstart.md` are complete.

**Tests**: Required by the feature specification and Constitution Principle VII.
Tests must be authored and observed failing for the intended reason before the
implementation task that makes them pass.

**Organization**: Tasks are grouped by user story. Shared flag/context seams are
implemented first; each story then has its own tests and implementation/checkpoint.

## Phase 1: Setup (Shared Test Scaffolding)

**Purpose**: Establish failing tests for the two shared primitives used by all
story phases.

- [X] T001 [P] Add table-driven tests for the shared `yes` flag name, default, and author-flag collision preservation in `internal/cli/cli_test.go`.
- [X] T002 [P] Add context-carrier tests for absent, false, and true approval values through `contract/context_test.go` and `context_test.go`.

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Implement the shared flag and context seams that direct execution,
the confirmation gate, schema reflection, and MCP dispatch all depend on.

**Checkpoint**: `FlagYes` and the approval context carrier exist with no mutable
state, no new dependency, and collision behavior matching the existing safety
flags.

- [X] T003 Implement `FlagYes = "yes"` and reuse the existing persistent-bool installer in `internal/cli/cli.go` so an existing author flag is never overwritten; make the tests from `internal/cli/cli_test.go` pass.
- [X] T004 Implement `contract.WithApproval`/`contract.ApprovalFromContext` in `contract/context.go` and the root `ax.WithApproval`/`ax.ApprovalFromContext` aliases in `context.go`; make the tests from `contract/context_test.go` and `context_test.go` pass.

## Phase 3: User Story 1 — A non-interactive run never hangs (Priority: P1) 🎯 MVP

**Goal**: A confirmation point in a direct Cobra CLI either fails immediately
with a structured exit-2 refusal or proceeds under explicit `--yes`, without
touching stdin/stdout or performing any prompt-like I/O.

**Independent Test**: Execute a confirmation-gated command in machine mode with
and without `--yes`; the blocked run exits 2, emits exactly one
`confirmation_required` envelope to stderr, emits no stdout, performs no effect,
and does not read stdin; the approved run exits 0 and matches the ungated
success payload apart from documented non-deterministic metadata.

### Tests for User Story 1

- [X] T005 [US1] Add a table-driven confirmation decision test in `confirm_test.go` covering approved, blocked, prompt-required, nil/state-free context, repeated calls, dry-run orthogonality, and the guarantee that `Confirm` performs no stdin/stdout/pager/editor/spinner I/O.
- [X] T006 [US1] Add Execute integration tests in `execute_test.go` for auto-mounted `--yes`, machine-mode refusal/exit 2, stderr-only envelope output, side-effect suppression, approved-vs-ungated payload equivalence, all four approval-by-dry-run combinations, piped stdin preservation, and author-declared `yes` collision handling.
- [X] T007 [US1] Add a deterministic confirmation error-envelope regression test and fixture in `error_test.go` and `testdata/confirmation_required.golden.json`, asserting `error_code=confirmation_required`, fixed `actionable_fix`, unchanged envelope fields, and exit code 2. Also assert FR-019 directly in `json_test.go`: the success envelope must be byte-identical with and without approval in context, and its `meta` key set must stay exactly `trace_id`/`span_id`/`idempotency_key`. The existing envelope golden cannot catch a field that appears only when approval is granted, because it renders from a context that never carries one.

### Implementation for User Story 1

- [X] T008 [US1] Implement `ConfirmationOutcome`, the three exported outcome constants, and `Confirm(ctx, subject)` in `confirm.go`; normalize nil contexts, fail closed when mode is absent, return the existing `ax.Error` with `confirmation_required`/`ExitValidation`/`--yes` remediation, and perform no I/O.
- [X] T009 [US1] Install and resolve the persistent `--yes` flag in `execute.go` alongside mode, dry-run, and idempotency-key resolution, carrying the result with `WithApproval` before the adopting command's pre-run callback executes.

**Checkpoint**: User Story 1 is independently functional and can be validated
with the tests in `confirm_test.go`, `execute_test.go`, and `error_test.go`.

## Phase 4: User Story 2 — An MCP client approves a call it initiated (Priority: P2)

**Goal**: MCP clients can approve each confirmation-gated call explicitly with a
boolean `yes` argument, observe the same structured refusal when omitted, and
never leak approval between calls.

**Independent Test**: Call a gated MCP tool without `yes`, with `yes: true`,
with `yes: false`, with a non-boolean `yes`, and in two sequential calls; assert
refusal, success, validation error, and no cross-call approval leakage as
specified by `contracts/mcp-confirmation.md`.

### Tests for User Story 2

- [X] T010 [US2] Extend `internal/mcpserver/dispatch_test.go` with a confirmation-gated test command and failing table-driven tests for absent/true/false/non-boolean `yes`, exit-2 validation, refusal envelope mapping, approved side effects, and sequential-call state isolation.

### Implementation for User Story 2

- [X] T011 [US2] Extend `callConfig`, argument validation, required-flag accounting, isolated argv construction, and per-call context application in `internal/mcpserver/dispatch.go` to thread boolean `yes` exactly like dry-run without storing state on the dispatcher.
- [X] T012 [US2] Update `internal/mcpserver/dispatch.go` persistent-flag setup to install `FlagYes` for directly embedded MCP servers and preserve author-declared `yes` flags; make the tests in `internal/mcpserver/dispatch_test.go` pass.

**Checkpoint**: User Stories 1 and 2 both work independently; a direct CLI and
an MCP `tools/call` produce the same confirmation decision for the same approval
state.

## Phase 5: User Story 3 — A human-operated CLI keeps its confirmation (Priority: P3)

**Goal**: Human mode remains distinguishable from both approval and machine-mode
blocking, while the gate itself still never prompts or performs interactive I/O.

**Independent Test**: Resolve human mode without approval and assert
`ConfirmationPromptRequired, nil`; resolve human mode with approval and assert
`ConfirmationApproved, nil`; verify neither path reads stdin or writes output.

- [X] T013 [US3] Add a dedicated human-mode regression test in `confirm_test.go` that asserts the prompt-required outcome is distinct from approved and blocked, returns no `confirmation_required` error, and leaves the actual prompt responsibility with the caller.
- [X] T014 [US3] Document the caller-owned prompt branch and approved/no-prompt branch in `README.md` and `specs/018-yes-no-prompt-invariant/quickstart.md`, including the fail-closed behavior for a context with no resolved mode.

**Checkpoint**: The gate preserves a human CLI's confirmation decision point
without making ax-go itself a prompt framework.

## Phase 6: User Story 4 — The primitive is discoverable and learnable (Priority: P3)

**Goal**: Agents discover `--yes` through the existing schema, and developers
have runnable root-package and integration examples of a confirmation-gated
command.

**Independent Test**: Run `__schema` on a CLI with no author-defined safety flags
and find the boolean `yes` flag with default false; run `ExampleConfirm`; run the
integration example's gated command with and without `--yes`.

### Tests for User Story 4

- [X] T015 [P] [US4] Extend schema reflection assertions in `schema_test.go`, `schema/schema_test.go`, and `examples/integration/golden_test.go` so the tests fail until the ordinary flag path exposes boolean `yes` with default false; defer fixture rewrites to T017.
- [X] T016 [P] [US4] Add the verified primary-API `ExampleConfirm` to `example_test.go`, exercising approved, blocked, and prompt-required contexts with deterministic output and no interactive input.

### Implementation and Documentation for User Story 4

- [X] T017 [US4] Add a confirmation-gated canonical command and recorded side-effect seam to `examples/integration/main.go`, wire it into the command tree, cover blocked/approved behavior in `examples/integration/main_test.go` and `examples/integration/golden_test.go`, and update the schema fixtures generated by the real integration command.
- [X] T018 [US4] Update the agent-safety and confirmation usage sections in `README.md` with `--yes`, `ax.Confirm`, the three outcomes, exit-2 remediation, and the MCP per-call argument.

**Checkpoint**: Schema, godoc, README, and the real-Cobra integration command
all teach the same public contract.

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Reconcile public-surface artifacts and run the repository's
required quality gates after all story implementations are complete.

- [X] T019 [P] Review exported doc comments for `ConfirmationOutcome`, all three outcome constants, `Confirm`, `WithApproval`, and `ApprovalFromContext` in `confirm.go` and `context.go` against the `godoclint` contract.
- [X] T020 Regenerate and review the intentional public-surface additions in `internal/cmd/surfacecheck/baseline.json` and append retained supported audit records in `specs/023-internalize-helpers/public-surface-audit.json` with `make surface-update`, confirming only the planned root/contract declarations and schema-visible behavior changed.
- [X] T021 Run `gofmt` on all changed Go files: `confirm.go`, `context.go`, `execute.go`, `internal/cli/cli.go`, `contract/context.go`, `internal/mcpserver/dispatch.go`, `examples/integration/main.go`, and their changed test files.
- [X] T022 Run the default-configuration gates from the repository root: `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `make doc-coverage`, `make cover-check` (root package `ax` carries an 85% per-package floor and gains `confirm.go`), `go mod tidy -diff` (`go.sum` was tidied during implementation), and `npm run lint:md` for the changed `README.md` and `AGENTS.md`. Record and fix any failure in the changed files. Prefer `npm run lint:md` over `make lint` locally: `make lint` reliably stalls at its actionlint step in snap-based environments.
- [X] T023 Run the declined-dependency verification matrix from the repository root across **every** non-default configuration — `ax_no_grpc`, `ax_no_otlp`, and `ax_no_grpc,ax_no_otlp` — with `go test -race -tags=<tags> ./...`, `go vet -tags=<tags> ./...`, and `golangci-lint run --build-tags=<tags>` for each, confirming the new root/context behavior has parity under tagged builds. `BUILD_TAG_MATRIX` declares all four configurations exhaustive and `make test`/`make lint` iterate them; a green default run does NOT cover a declined configuration, and testing only the both-tags case leaves each single-tag build unverified.
- [X] T024 Run `make surface-check` from the repository root and verify `git diff --check` plus the final `git status --short` in `internal/cmd/surfacecheck/baseline.json`, `AGENTS.md`, and `specs/018-yes-no-prompt-invariant/`.
- [X] T025 Reconcile the derived agent documentation: add `--yes` as the fourth entry in the `AGENTS.md` "Core AX Mandates" agent-safety primitives list, covering the three `ax.Confirm` outcomes, the exit-2 `confirmation_required` envelope and its `actionable_fix`, dry-run orthogonality, and the MCP per-call boolean `yes` argument. `AGENTS.md` is the canonical agent-instruction file and a derived doc the constitution expects reconciled in the same change; leaving its three-item primitives list stale would teach every future agent the wrong contract.

### Verification record — 2026-08-24

All Phase 7 gates executed from the repository root. Every gate relevant to this
changeset is green:

| Gate | Configurations | Result |
|---|---|---|
| `gofmt -s -l .` | default | clean |
| `go vet ./...` | all 4 | clean |
| `golangci-lint run` | all 4 | `0 issues.` each |
| `go test -race ./...` | all 4 | green except one pre-existing failure (below) |
| `make doc-coverage` | default | `23/23 required symbols have an example` |
| `go run ./internal/cmd/covercheck` | default | exit 0; root `ax` **90.3% >= 85.0%**, repo-wide **84.0% >= 78.0%** |
| `make surface-check` | 4 configs x 6 profiles | `pass`, 312 features, 167 audit records, 7 packages |
| `go mod tidy -diff` | default | clean (confirms the incidental `go.sum` tidy) |
| `npm run lint:md` | n/a | clean |

**Pre-existing failure, outside this changeset**:
`internal/telemetry TestTelemetryResource` fails on both subtests
(`telemetry.sdk.name missing/empty ... SDK default resource was dropped`) in all
four build configurations. It was confirmed to fail identically on a pristine
detached worktree at `HEAD` with none of this feature's changes present, so it is
base-branch breakage, not a regression from this work. `internal/telemetry` is not
in this changeset, and its coverage (65.8%) still clears its 60% floor. Most likely
cause is the OpenTelemetry monorepo bump to v1.45.0 (commit `1bbfce3`) changing
resource-merge semantics. It needs its own issue and fix; deliberately not addressed
here, since repairing an unrelated package inside this feature branch would violate
the changeset-scoping rule in `AGENTS.md`.

`make cover-check` and `make test` cannot run to completion until that failure is
fixed, because both abort at the shared test step; `covercheck` was therefore run
directly against the coverage profile that step still produced.

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 — Setup**: T001–T002 establish the failing shared tests and can run in parallel.
- **Phase 2 — Foundational**: T003–T004 depend on Phase 1 and block every user story.
- **Phase 3 — US1**: T005–T007 are failing tests; T008–T009 implement the direct CLI path.
- **Phase 4 — US2**: T010 must fail before T011–T012; it depends on the shared flag/context foundation and the direct gate contract.
- **Phase 5 — US3**: T013–T014 depend on the gate from US1; no new runtime dependency is introduced.
- **Phase 6 — US4**: T015–T018 depend on the auto-mounted flag and gate; T015/T016 can run in parallel before T017/T018.
- **Phase 7 — Polish**: T019–T025 depend on all desired stories and must be completed before handoff.

### User Story Dependencies

- **US1 (P1)**: Starts after Phase 2; no dependency on another story. This is the MVP.
- **US2 (P2)**: Starts after Phase 2 and requires the public gate contract from US1 for its end-to-end gated-command test.
- **US3 (P3)**: Uses the US1 gate and is independently verified by its human-mode outcome test.
- **US4 (P3)**: Uses the shared flag and gate from Phases 2–3; schema reflection itself has no special-case dependency.

### Parallel Opportunities

- T001 and T002 can run in parallel because they touch separate test packages.
- Within US1, T005 and T006 can run in parallel; T007 can also proceed in parallel if the shared golden helper is not modified concurrently.
- Within US4, T015 and T016 can run in parallel; T017 and T018 touch separate documentation/example concerns after their tests are authored.
- T019 can run in parallel with review of the schema fixture changes before T020; T022 and T023 are independent build configurations but should not run concurrently on a shared module cache if the environment is constrained.

## Parallel Example: User Story 1

```text
Task: T005 — table-driven gate outcome and no-I/O tests in confirm_test.go
Task: T006 — Execute stream/exit/side-effect tests in execute_test.go
Task: T007 — confirmation_required golden regression in error_test.go and testdata/confirmation_required.golden.json
```

## Parallel Example: User Story 2

```text
Task: T010 — MCP confirmation argument and isolation tests in internal/mcpserver/dispatch_test.go
```

The implementation tasks T011 and T012 must follow T010 because they modify the
same dispatcher behavior being asserted.

## Parallel Example: User Story 3

```text
Task: T013 — human-mode outcome regression in confirm_test.go
Task: T014 — caller-owned prompt documentation in README.md and quickstart.md
```

## Parallel Example: User Story 4

```text
Task: T015 — schema assertions and golden fixtures
Task: T016 — ExampleConfirm in example_test.go
```

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete T001–T004 for the shared flag/context foundation.
2. Complete T005–T009 for the direct `Confirm`/`--yes` path.
3. Run the US1 independent test criteria and stop for an MVP review.

### Incremental Delivery

1. Add US2 MCP threading and isolation (T010–T012).
2. Add US3 human-mode preservation and caller guidance (T013–T014).
3. Add US4 schema, example, integration command, and README discoverability (T015–T018).
4. Complete the public-surface and full verification polish phase (T019–T025).

### Test-First Rule

For each story, execute its test tasks first and confirm they fail for the
missing behavior before starting the corresponding implementation tasks. Never
weaken an assertion to make an implementation pass.
