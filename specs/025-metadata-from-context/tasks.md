# Tasks: Live-Resolving Metadata Accessor for Root ax

**Input**: Design documents from `/specs/025-metadata-from-context/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`,
`contracts/public-api.md`, `quickstart.md`

**Tests**: Required by the feature specification and Constitution Principle
VII. Every behavior task begins with a failing test and records the expected
failure before implementation. The doc-comment-only change (User Story 2) has
no new behavior to test; its verification is the existing `contract` package
test suite plus `golangci-lint`'s `require-doc` gate staying green.

**Organization**: Tasks are grouped by independently testable user story.
Task IDs are execution ordered; `[P]` marks work in different files that can
proceed without an incomplete same-file dependency.

## Phase 1: Setup (Shared Verification)

**Purpose**: Confirm the feature pointer and pre-change behavior before
writing a test.

- [X] T001 Verify the 16/16 requirements checklist in `specs/025-metadata-from-context/checklists/requirements.md`, confirm `.specify/feature.json` and the managed `AGENTS.md` block point to feature 025, and run the existing focused tests for `trace_test.go` and `contract/context_test.go` to record the pre-change baseline

---

## Phase 2: Foundational (Design Contract)

**Purpose**: Lock the exact public contract every story uses.

- [X] T002 Cross-check `specs/025-metadata-from-context/contracts/public-api.md` against `trace.go`, `json.go`, `error.go`, and `contract/context.go`, resolving any signature, composition, or precedence mismatch in the feature design artifacts before tests begin

**Checkpoint**: The new function's signature, composition
(`contract.MetadataFromContext(withTraceMetadata(ctx))`), and precedence
rules (live span always supersedes explicit metadata) are unambiguous and
implementation-ready.

---

## Phase 3: User Story 1 — Read the Same Metadata a Command's Own Output Would Carry (Priority: P1) 🎯 MVP

**Goal**: `ax.MetadataFromContext(ctx)` returns live-resolved trace/span IDs
plus dry-run/idempotency-key state, equal field-for-field to
`ax.NewEnvelope(ctx, x).Meta` for the same context.

**Independent Test**: Running a command through `ax.Execute` with an active
root span, `--dry-run`, and an auto-generated idempotency key, calling
`ax.MetadataFromContext(cmd.Context())` returns non-zero trace/span IDs equal
to `ax.NewEnvelope(cmd.Context(), x).Meta`'s; with no active span it returns
`ZeroTraceID`/`ZeroSpanID`; a nil context does not panic; a live span's IDs
take precedence over any trace/span ID explicitly stored on the context.

### Tests for User Story 1

- [X] T003 [US1] Add `TestMetadataFromContextInsideExecuteMatchesNewEnvelope` (asserting full `Metadata` struct equality — `TraceID`, `SpanID`, `DryRun`, and `IdempotencyKey` — against `ax.NewEnvelope(cmd.Context(), x).Meta`, not just trace/span IDs), `TestMetadataFromContextWithNoSpanReturnsZeroIDs`, `TestMetadataFromContextNilContextFallsBackToBackground`, and `TestMetadataFromContextLiveSpanSupersedesExplicitMetadata` (stores explicit trace/span IDs via `contract.WithMetadata`, starts a live span on the same context, and asserts the returned `TraceID`/`SpanID` are the live span's, not the explicitly stored ones — the regression test for FR-006) to `trace_test.go` (mirroring the existing `TestTraceIDFromContextWithActiveSpanIsNonZero`, `TestTraceIDFromContextWithNoSpanReturnsZeroTraceID`, and `TestWithTraceMetadataNilContextFallsBackToBackground` patterns), then run the focused test pattern and verify it fails to compile because `MetadataFromContext` does not exist in the root package

### Implementation for User Story 1

- [X] T004 [US1] Add the fully documented `MetadataFromContext(ctx context.Context) Metadata` function to `trace.go`, implemented as `contract.MetadataFromContext(withTraceMetadata(ctx))`, with the doc comment from `contracts/public-api.md` stating live-span resolution and that explicit metadata is superseded by an active span
- [X] T005 [US1] Run the User Story 1 focused tests in `trace_test.go` under default and `ax_no_grpc,ax_no_otlp`, confirming the full-struct equality, zero-value, nil-context, and live-span-supersedes-explicit-metadata assertions all pass

**Checkpoint**: A caller inside root `ax` code gets live trace/span IDs from
the new accessor, guaranteed equal to what the envelope/error constructors
already emit.

---

## Phase 4: User Story 2 — Get Warned by the Isolated Accessor Instead of Silently Misled (Priority: P2)

**Goal**: `contract.MetadataFromContext`'s documentation discloses that it
cannot resolve an active span, mirroring the warning its sibling
`TraceIDFromContext`/`SpanIDFromContext` already carry.

**Independent Test**: Reading `contract.MetadataFromContext`'s doc comment
shows the same live-span limitation already documented on its sibling
accessors, with no change to its return value for any input.

### Implementation for User Story 2

- [X] T006 [US2] Extend `contract.MetadataFromContext`'s doc comment in `contract/context.go` with the no-live-span warning paragraph from `research.md` D3, without changing the function body
- [X] T007 [US2] Run `go test -race ./contract/...` and `golangci-lint run` scoped to `contract/`, confirming every existing `contract` test still passes unchanged and `require-doc` is satisfied for the updated comment

**Checkpoint**: The isolation boundary is now disclosed in the one place a
`contract`-only consumer would actually read it, with zero behavioral change.

---

## Phase 5: User Story 3 — Discover the New Accessor Where Envelopes Are Already Documented (Priority: P3)

**Goal**: Reference documentation and introductory tutorial material that
already describe envelope construction also describe the new accessor.

**Independent Test**: The README section introducing `ax.NewEnvelope` and
the `build-your-first-cli` tutorial's equivalent section both mention
`ax.MetadataFromContext`; a verified `ExampleMetadataFromContext` compiles
and runs.

### Tests and examples for User Story 3

- [X] T008 [P] [US3] Add a standalone verified `ExampleMetadataFromContext` to `example_test.go` demonstrating the function with a background context (deterministic `// Output:` showing zero-value IDs), then run the example test and `make doc-coverage` to confirm it does not regress the required-symbol baseline

### Documentation for User Story 3

- [X] T009 [P] [US3] Add a one- or two-sentence mention of `ax.MetadataFromContext` to the `README.md` section introducing `ax.NewEnvelope`
- [X] T010 [P] [US3] Add the same mention to `docs/src/content/docs/tutorials/build-your-first-cli.md` at the point `NewEnvelope` is first introduced

**Checkpoint**: A developer reading either canonical entry point for
envelope construction also finds the new accessor there.

---

## Phase 6: Public Surface and Cross-Cutting Verification

**Purpose**: Record the intentional export, review all generated artifacts,
and run the repository's required gates without lowering a policy budget.

- [X] T011 Add the sorted supported/live/keep-public `func:MetadataFromContext` decision (signature `func(context.Context) Metadata`) and advance `audited_at` in `specs/023-internalize-helpers/public-surface-audit.json`, then run `make surface-update` and review `internal/cmd/surfacecheck/baseline.json` to confirm it adds only `func:MetadataFromContext` with universal presence
- [X] T012 Run `gofmt -s` on changed Go files, execute the feature scenarios in `specs/025-metadata-from-context/quickstart.md`, and mark every completed task `[X]` in `specs/025-metadata-from-context/tasks.md`
- [X] T013 Run `make test` and `make validate`, fixing every race, build-tag, format, tidy, and vet failure without weakening tests or changing the feature contract
- [X] T014 Run `make lint` (or `golangci-lint run` directly per this repo's actionlint caveat) and `make doc-coverage`, fixing every Go/Markdown lint or verified-example failure without adding a doccover exemption
- [X] T015 Run `make cover-check`, `make surface-check`, and `make size-check`, fixing failures without lowering coverage floors, changing size budgets, or accepting unintended public-surface drift
- [X] T016 Review `git diff --check`, `git status --short`, and the full diff for scope: no `CHANGELOG.md` edit, no ADR edit/deletion, no payload/schema golden drift, no new dependency, and no unrelated user change; confirm all 16 tasks are complete in `specs/025-metadata-from-context/tasks.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on T001 and blocks behavior work.
- **User Story 1 (Phase 3)**: Depends on T002; it is the MVP and establishes
  the exported function every later story documents or verifies.
- **User Story 2 (Phase 4)**: Independent of US1's implementation details —
  it touches only `contract/context.go` — but is sequenced after US1 so the
  doc comment can reference the finished root function by name.
- **User Story 3 (Phase 5)**: Depends on T004 (the function must exist to
  document and exemplify it).
- **Public Surface and Verification (Phase 6)**: T011 depends on the final
  export from T004; T012–T016 are ordered validation checkpoints.

### User Story Dependencies

- **US1 (P1)**: Independent MVP after design lock; delivers the live-resolving
  accessor.
- **US2 (P2)**: Documentation-only change to a different file
  (`contract/context.go`); has no runtime dependency on US1 but is sequenced
  after it for narrative clarity in review.
- **US3 (P3)**: Documents and exemplifies the US1 contract; does not alter
  runtime semantics.

### Within Each User Story

- T003 must fail to compile for the missing export before T004 adds it.
- Same-file work remains serial: `trace_test.go` T003 before `trace.go` T004.
- T008, T009, and T010 touch distinct files and may proceed together once
  T004 lands.

## Parallel Opportunities

- T008, T009, and T010 are independent file groups after T004.
- T006/T007 (US2) can proceed concurrently with US3's documentation tasks
  once T004 has landed, since they touch entirely different files.
- No Phase 3 same-file task is marked `[P]`; the test-first dependency
  between `trace_test.go` and `trace.go` is intentionally serial.

## Parallel Example: User Story 3

```text
Task: "Add ExampleMetadataFromContext in example_test.go"
Task: "Update README.md envelope section"
Task: "Update build-your-first-cli.md tutorial"
```

## Implementation Strategy

### MVP First (User Story 1)

1. Complete T001–T002.
2. Write T003 and observe the expected compile failure.
3. Implement T004 and validate T005.
4. Stop: a caller inside root `ax` code can now read live-resolving envelope
   metadata without constructing an envelope or error.

### Incremental Delivery

1. US1 adds the live-resolving accessor.
2. US2 discloses the isolation boundary on the pre-existing accessor.
3. US3 makes the new accessor discoverable in documentation and adds a
   runnable example.
4. Phase 6 records the public surface and runs the complete policy suite.

## Notes

- No governing ADR means no final ADR-retirement task is generated.
- Do not edit or create `CHANGELOG.md`; release notes come from the eventual
  Conventional Commit.
- Do not change `contract.MetadataFromContext`'s return value for any input
  — T006 is a doc-comment-only change.
- Do not pre-populate trace metadata inside `Execute`'s stored context;
  resolution stays at read time (research.md D1).
- `make bench-check` is not required for this feature — the plan asserts no
  performance target for this trivial accessor.
