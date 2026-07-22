---

description: "Task list for __schema Non-Deterministic Field Enumeration"
---

# Tasks: __schema Non-Deterministic Field Enumeration

**Input**: Design documents from `/specs/015-schema-nondeterministic-fields/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/schema-non-deterministic-fields.md](./contracts/schema-non-deterministic-fields.md), [quickstart.md](./quickstart.md)

**Tests**: Included and REQUIRED. AGENTS.md's Testing-First Discipline and plan.md's Constitution Check (Principle VII) both mandate that tests land before implementation for this feature — this is a project-wide, already-adopted convention, not an optional addition.

**Organization**: Tasks are grouped by user story (spec.md P1/P2/P3) to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- File paths are exact and relative to the repository root

## Governing ADR Status

None. research.md's "Governing ADR Status" confirms the GitHub issue's referenced ADR-0002/ADR-0003 do not exist as files in `docs/adr/`; the standing decisions already live as constitution principles II, III, and XI. No ADR-retirement task is included.

---

## Phase 1: Setup

**Purpose**: Confirm a clean starting baseline before touching shared schema code.

- [X] T001 Run `go test -race ./...`, `go vet ./...`, and `golangci-lint run` from the repository root to confirm the pre-feature baseline is green (AGENTS.md Development Workflow step 1; no new dependency is introduced by this feature per plan.md Technical Context, so no `go.mod` changes are expected)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shared reflection/registration mechanism and struct-shape additions that both User Story 1 and User Story 2 build on. Per research.md D1, one mechanism (struct tag + `WithNonDeterministicFields[T]`) serves both the built-in metadata fields and author-defined fields, so it must exist before either story can be wired into real `__schema` output.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Tests for Foundational (write first, confirm they fail)

- [X] T002 [P] Write failing unit tests for a new `internalschema.DataLocators(t reflect.Type) []string` function in `internal/schema/schema_test.go`: a directly tagged field yields `data.<json-name>`; a nested struct field descends to `data.<parent>.<child>`; a slice/array/map-value struct element descends without an index or key segment (`data.items.id`, never `data.items[0].id`, per research.md D3); an embedded (anonymous) struct's tagged fields are inlined at the parent's path; unexported fields are always skipped; a type with zero tagged fields returns an empty (non-nil) slice; a self-referential/deeply-nested struct does not panic or infinite-loop (bounded recursion, research.md D9); the result is sorted lexicographically and deduplicated (research.md D8)
- [X] T003 [P] Write failing unit tests for `internalschema.RegisterEnvelope(cmd *cobra.Command, dataLocators []string)` and `internalschema.NonDeterministicFields(annotations map[string]string) []string` in `internal/schema/schema_test.go`: after `RegisterEnvelope` on a real command, `NonDeterministicFields` returns the sorted, deduplicated union of the fixed built-in literal `{"meta.trace_id", "meta.span_id", "meta.idempotency_key"}` and the supplied `data.` locators; a command's annotations map that never went through `RegisterEnvelope` yields an explicit empty (non-nil) slice from `NonDeterministicFields`; calling `RegisterEnvelope` twice on the same command overwrites rather than accumulates (research.md D1, "Idempotency"); passing `cmd = nil` to `RegisterEnvelope` is a no-op and never panics (research.md D9)
- [X] T004 [P] Write failing test in `internal/schema/schema_test.go` asserting `BuildCommand` copies `cmd.Annotations` verbatim onto the returned `Command.Annotations` field, for both a command with annotations set and a command with a nil annotations map (data-model.md "`internal/schema.Command.Annotations`")
- [X] T005 [P] Write failing drift-detection test in `schema/schema_test.go`: reflect `contract.Metadata` once via `reflect.TypeFor[contract.Metadata]()`, collect every field tagged `ax:"nondeterministic"` as a `meta.`-prefixed locator, and assert the result equals the hardcoded built-in literal `{"meta.trace_id", "meta.span_id", "meta.idempotency_key"}` (research.md D2) — this test must fail today because `contract.Metadata` carries no such tags yet

### Implementation for Foundational

- [X] T006 [P] Add `ax:"nondeterministic"` struct tags to `contract.Metadata.TraceID`, `SpanID`, and `IdempotencyKey` in `contract/json.go` (documentation-only — do not touch the existing `json:` tags or `DryRun`, which stays untagged per research.md D2); this makes T005 pass
- [X] T007 Add `Annotations map[string]string` to the `internal/schema.Command` struct in `internal/schema/schema.go`, and update `BuildCommand` to copy `cmd.Annotations` onto the returned value for every node in the tree; makes T004 pass
- [X] T008 Implement `internalschema.DataLocators(t reflect.Type) []string` in `internal/schema/schema.go` (or a new `internal/schema/nondeterministic.go` in the same package): walks `t`'s fields (and nested struct/slice/array/map/pointer element types, embedded-struct promotion, bounded recursion depth) collecting `data.`-prefixed, dot-separated `json`-tag-name paths for every field carrying `ax:"nondeterministic"`; returns a sorted, deduplicated, non-nil slice; never panics on any input `reflect.Type` (research.md D3, D8, D9); makes T002 pass
- [X] T009 Implement `internalschema.RegisterEnvelope(cmd *cobra.Command, dataLocators []string)` and `internalschema.NonDeterministicFields(annotations map[string]string) []string` in the same file as T008: `RegisterEnvelope` stores a private envelope-marker annotation plus the encoded `dataLocators` on `cmd.Annotations` (nil `cmd` is a no-op), overwriting any prior registration; `NonDeterministicFields` returns the sorted, deduplicated union of the fixed built-in literal and the decoded `data.` locators when the envelope marker is present, or an explicit empty non-nil slice otherwise; makes T003 pass
- [X] T010 Add `NonDeterministicFields []string` to `schema.CommandSchema` (`json:"non_deterministic_fields"`, no `omitempty`) and to `schema.ErrorSchemaInfo` (`json:"non_deterministic_fields"`, no `omitempty`) in `schema/schema.go`, matching data-model.md's struct shapes; update any existing struct-literal-equality tests in `schema/schema_test.go` that construct `CommandSchema`/`ErrorSchemaInfo` values directly so they keep compiling
- [X] T011 Implement `func WithNonDeterministicFields[T any](cmd *cobra.Command)` in `schema/schema.go`: nil `cmd` is a no-op; otherwise calls `internalschema.RegisterEnvelope(cmd, internalschema.DataLocators(reflect.TypeFor[T]()))` (research.md D1); reflection runs exactly once, at call time, never on the `__schema` request path
- [X] T012 Forward `WithNonDeterministicFields[T any]` from the root `ax` package facade in `schema.go`: a thin generic forwarding function (cannot be a type alias, matching how `BuildSchema`/`NewSchemaCommand` are already forwarded)

**Checkpoint**: The reflection/registration mechanism exists and is unit-tested end-to-end (tag → `DataLocators` → `RegisterEnvelope`/`NonDeterministicFields` → `Command.Annotations` passthrough → drift-detection against `contract.Metadata`), but nothing is wired into real `__schema`/MCP output yet. User story work can now begin.

---

## Phase 3: User Story 1 - Agent trusts the diff, not just the promise (Priority: P1) 🎯 MVP

**Goal**: Every command node in both `__schema` (ax-native) and `__schema --as=mcp` output carries a `non_deterministic_fields`/`nonDeterministicFields` list — populated for commands registered via `WithNonDeterministicFields[T]`, explicitly empty otherwise — and the error envelope schema carries `non_deterministic_fields: ["trace_id"]`.

**Independent Test**: Call `__schema` for any command in the existing root test tree (`schema/schema_test.go`'s `newSchemaTestCommand`), read `non_deterministic_fields` on every node (populated or empty), and confirm the field is present, sorted, and deduplicated per FR-001/FR-002/FR-005/FR-006/FR-007.

### Tests for User Story 1 (write first, confirm they fail)

- [X] T013 [P] [US1] Write failing test in `schema/schema_test.go` asserting `BuildSchema(...).ErrorEnvelope.NonDeterministicFields == []string{"trace_id"}`
- [X] T014 [P] [US1] Write failing test in `schema/schema_test.go`: build a command registered via `WithNonDeterministicFields[T]` for a locally-defined `T` with one `ax:"nondeterministic"`-tagged field, assert its `CommandSchema.NonDeterministicFields` equals the sorted union of the built-in literal and `data.<field>`; assert a sibling unregistered command's `CommandSchema.NonDeterministicFields` is an explicit empty (non-nil) slice; assert calling `WithNonDeterministicFields[T](nil)` does not panic
- [X] T015 [P] [US1] Write failing test in `internal/mcp/mcp_test.go` asserting a registered command's walked `Tool.NonDeterministicFields` equals the same union computed for the equivalent `CommandSchema`, and an unregistered command's `Tool.NonDeterministicFields` is an explicit empty (non-nil) slice

### Implementation for User Story 1

- [X] T016 [US1] Wire `schema.BuildSchema`'s `ErrorSchemaInfo` construction in `schema/schema.go` to set `NonDeterministicFields: []string{"trace_id"}`; makes T013 pass
- [X] T017 [US1] Wire `schema.convertCommandSchema` in `schema/schema.go` to populate `CommandSchema.NonDeterministicFields` for every node via `internalschema.NonDeterministicFields(command.Annotations)`, using the `Annotations` passthrough from T007; makes T014 pass
- [X] T018 [US1] Add `NonDeterministicFields []string` to the internal `mcp.Tool` struct in `internal/mcp/mcp.go`, and populate it in `Build` via `internalschema.NonDeterministicFields(cmd.Annotations)` for each walked command; update any existing struct-literal-equality tests in `internal/mcp/mcp_test.go` that construct `Tool` values directly so they keep compiling; makes T015 pass
- [X] T019 [US1] Add `NonDeterministicFields []string` (`json:"nonDeterministicFields"`, no `omitempty`) to `schema.MCPTool` in `schema/schema.go`, and wire `BuildMCPSchema` to pass `tool.NonDeterministicFields` through from the internal `mcp.Tool`
- [X] T020 [P] [US1] Hand-edit `testdata/schema_ax.golden.json` and `testdata/schema_mcp.golden.json` at the repository root to add `"non_deterministic_fields": []` (ax-native, every command node) and `"nonDeterministicFields": []` (MCP, every tool) — the root test command tree (`newSchemaTestCommand`) registers no commands via `WithNonDeterministicFields`, so every entry is the explicit empty list per research.md D6 — plus `"non_deterministic_fields": ["trace_id"]` on `error_envelope`
- [X] T021 [US1] Run `go test ./... -run 'TestBuildSchema|TestBuildMCPSchema|TestRootSchemaOutputMatchesIsolatedPackage'` from the repository root and confirm all pass against the updated fixtures from T020

**Checkpoint**: `__schema`, `__schema --as=mcp`, and the error envelope schema all always emit `non_deterministic_fields`, populated for registered commands and explicitly empty otherwise. User Story 1 is independently functional and testable via the root golden fixtures.

---

## Phase 4: User Story 2 - Command author marks a field once (Priority: P2)

**Goal**: A command author tags one payload field and registers the command's type once; the field appears in `__schema` output automatically, and renaming the tagged field requires no change at the registration site.

**Independent Test**: Tag `examples/integration`'s `helloPayload.EntityID` field, register the payload type, regenerate `__schema` output, and confirm `data.entity_id` appears in the root command's `non_deterministic_fields` list with no second list edited (research.md D11).

### Tests for User Story 2 (write first, confirm they fail)

- [X] T022 [P] [US2] Write failing unit test in `schema/schema_test.go`: define two otherwise-identical local struct types, one with a field tagged `ax:"nondeterministic"` under name `Foo` (`json:"foo"`), the other with the same field renamed to `Bar` (`json:"bar"`) but the tag preserved; call `internalschema.DataLocators` (or `WithNonDeterministicFields`) on each and assert the resulting locator name changes from `data.foo` to `data.bar` automatically, with no other code path touched — demonstrates spec.md US2 Acceptance Scenario 2 (FR-003)
- [X] T023 [US2] Confirm (do not yet fix) that `examples/integration/golden_test.go`'s `TestGoldenSchema` and `TestGoldenSchemaMCP` fail once T024–T025 below tag `EntityID` and register the example's commands, because the checked-in `examples/integration/testdata/schema_{ax,mcp}.golden.json` fixtures do not yet reflect the new field — this failure is expected and confirms the golden harness is sensitive to the change (spec.md US2's independent test, FR-008 in miniature)

### Implementation for User Story 2

- [X] T024 [US2] Tag `helloPayload.EntityID` with `ax:"nondeterministic"` in `examples/integration/main.go` (research.md D11)
- [X] T025 [US2] In `examples/integration/main.go`, call `ax.WithNonDeterministicFields[helloPayload](root)` in `newRootCommand`, `ax.WithNonDeterministicFields[streamPayload](cmd)` in `newStreamCommand`, and `ax.WithNonDeterministicFields[patchConfigPayload](cmd)` in `newPatchConfigCommand` — do not add `ax:"nondeterministic"` tags to `streamPayload` or `patchConfigPayload` fields, only register the commands so their `meta.*` locators appear (research.md D11); leave `fail`, `fetch`, `authz`, `crash`, `__schema`, and `mcp-server` unregistered so they keep an explicit empty command-scoped list
- [X] T026 [US2] Regenerate `examples/integration/testdata/schema_ax.golden.json` and `schema_mcp.golden.json` via `go test ./examples/integration -run TestGolden -update`, per quickstart.md step 5
- [X] T027 [US2] Review the regenerated diff by hand: confirm `data.entity_id` plus the three built-in `meta.*` locators appear only on the root command, `meta.*`-only on `stream`/`patch-config`, and explicit empty lists on every other command (`fail`, `fetch`, `authz`, `crash`, `__schema`, `mcp-server`)
- [X] T028 [US2] Run `go test ./examples/integration/...` (without `-update`) to confirm `TestGoldenSchema`, `TestGoldenSchemaMCP`, and every other golden test in the package pass against the regenerated fixtures

**Checkpoint**: A real, author-tagged domain field (`data.entity_id`) flows end-to-end through `__schema` and the MCP adapter with no hand-maintained second list, and renaming a tagged field is proven to require zero registration-site changes. User Stories 1 and 2 are both independently functional.

---

## Phase 5: User Story 3 - Regressions are caught before release (Priority: P3)

**Goal**: An unintentional drop of a previously-documented non-deterministic field (or its marking) is caught by an automated check before merge.

**Independent Test**: Remove a tag or a registration from a command that previously had one, and confirm a test fails without any manual re-verification of every command (spec.md US3 Acceptance Scenario 1).

### Tests for User Story 3

- [X] T029 [P] [US3] Add a dedicated regression-detection unit test in `schema/schema_test.go` (independent of golden-file byte-diffing): register a payload type with two `ax:"nondeterministic"`-tagged fields, capture its computed `NonDeterministicFields` list, then register a second, otherwise-identical type with one tag removed, and assert the second list is missing exactly that one locator — demonstrates FR-008's regression-catching property directly at the unit level, in addition to the golden-file protection already established by T020/T026

### Implementation for User Story 3

- [X] T030 [US3] Add a short paragraph to AGENTS.md's "Core AX Mandates" section (near the existing `__schema` bullet) stating that `non_deterministic_fields` in `__schema` output is the authoritative source of truth for which fields an agent may safely ignore when diffing two runs (FR-009); run the markdownlint skill on the edited file
- [X] T031 [US3] Run `make cover-check` from the repository root and confirm `internal/schema` and `schema` stay at or above their per-package coverage floors (`internal/schema` has an explicit **95%** override in `internal/cmd/covercheck/main.go`'s `defaultFloorConfig` — the new `DataLocators`/`RegisterEnvelope`/`NonDeterministicFields` code must be tested thoroughly enough to clear it, not the 25% default; `schema` has no explicit override → confirm its coverage against the 25% default) after the new reflection/registration code lands
- [X] T032 [US3] Run `make bench-check` from the repository root and confirm `BenchmarkBuildCommand` stays within the CI-enforced budget (≤5% `ns/op` regression when statistically significant, ≤+1 `allocs/op`) — per plan.md's Performance Goals, the built-in metadata fields are a hardcoded literal and author fields are reflected once at registration time, not on the `__schema` request path, so no regression is expected

**Checkpoint**: All three user stories are independently functional, tested, and enforced — the enumeration exists (US1), authors can extend it with zero hand-maintained bookkeeping (US2), and an unintentional regression is caught automatically (US3).

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final repository-wide verification once all three user stories are complete.

- [X] T033 [P] Run `gofmt -l .` and fix any unformatted files touched by this feature
- [X] T034 Run `go test -race ./...` from the repository root and confirm the full suite passes
- [X] T035 Run `go vet ./...` and `golangci-lint run` from the repository root and confirm both are clean
- [X] T036 Run `make doc-coverage` and confirm `WithNonDeterministicFields[T]` does not regress the primary-API `ExampleXxx` gate (per AGENTS.md, it is demonstrated inside `examples/integration`'s existing example rather than requiring a new gated example)
- [X] T037 Manually walk through quickstart.md's five steps, substituting the actual registered `examples/integration` root command (`hello`, payload `helloPayload`, tagged field `data.entity_id`) for the walkthrough's illustrative `report`/`reportPayload` example, which is not itself added to `examples/integration` (research.md D11): tag → register → `jq` the `__schema` output → mask-and-diff two runs of the real root command → regenerate goldens; confirm each step produces output analogous to what quickstart.md documents
- [X] T038 Re-read `spec.md`'s Success Criteria (SC-001–SC-004) against the final state of `testdata/schema_{ax,mcp}.golden.json` and `examples/integration/testdata/schema_{ax,mcp}.golden.json` and confirm each is met, including SC-002 via T039's automated mask-and-diff test rather than T037's manual walkthrough alone
- [X] T039 [P] Add an automated test (e.g. `TestByteIdenticalModuloNonDeterministicFields` in `examples/integration/golden_test.go`) that runs the registered root command twice, reads the root `CommandSchema.NonDeterministicFields` from `__schema` output, deletes exactly those JSON paths from both runs' payloads, and asserts the remainder is byte-identical — the CI-gated equivalent of quickstart.md step 4, closing the gap left by `internal/testutil.MaskNonDeterministic`'s fixed three-field regex (research.md D10) and giving SC-002's universal claim automated, non-manual coverage

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — run first.
- **Foundational (Phase 2)**: Depends on Setup. BLOCKS all user stories — the reflection/registration mechanism (`DataLocators`, `RegisterEnvelope`/`NonDeterministicFields`, `Command.Annotations`, the new struct fields, `WithNonDeterministicFields[T]`) is shared by every story.
- **User Story 1 (Phase 3)**: Depends on Foundational. Wires the mechanism into real `__schema`/MCP/error-envelope output.
- **User Story 2 (Phase 4)**: Depends on Foundational and on User Story 1's struct-field/wiring work being in place (it exercises the same `CommandSchema`/`MCPTool` fields against a real example CLI). Independently testable once complete.
- **User Story 3 (Phase 5)**: Depends on Foundational, and benefits from US1/US2's golden fixtures already being pinned (T020, T026) to demonstrate regression-catching concretely, though its own unit test (T029) does not require them.
- **Polish (Phase 6)**: Depends on all three user stories being complete.

### Within Each Phase

- Tests (marked "write first, confirm they fail") MUST be written and MUST fail before their corresponding implementation task.
- Struct-shape additions (T010) before the generic registration function (T011) before the facade forward (T012).
- `internal/schema` changes (T007–T009) before `schema/schema.go` wiring (T016–T017, T019) and before `internal/mcp` wiring (T018), since both consumers call the shared helper.

### Parallel Opportunities

- T002, T003, T004, T005 (Foundational tests, different files/functions) can be written in parallel.
- T006 (contract tag) can run in parallel with T007 (Annotations passthrough) — different files.
- T013, T014, T015 (US1 tests) can be written in parallel — different files.
- T020 (golden-fixture hand-edit) can proceed in parallel with T021 once T016–T019 land, since T020 only touches `testdata/`.
- T033 (gofmt) can run in parallel with T031/T032 in Phase 5 once the story's code is complete.
- T039 (automated SC-002 test) can be written in parallel with T033–T036 (different files, no shared state) once Phase 4's example-command registrations are in place.

---

## Parallel Example: Foundational Tests

```bash
# Launch all Foundational test-writing tasks together (different files/functions, no shared state):
Task: "Write failing unit tests for internalschema.DataLocators in internal/schema/schema_test.go"
Task: "Write failing unit tests for internalschema.RegisterEnvelope/NonDeterministicFields in internal/schema/schema_test.go"
Task: "Write failing test asserting Command.Annotations passthrough in internal/schema/schema_test.go"
Task: "Write failing drift-detection test for contract.Metadata in schema/schema_test.go"
```

## Parallel Example: User Story 1 Tests

```bash
Task: "Write failing test asserting ErrorSchemaInfo.NonDeterministicFields == [\"trace_id\"] in schema/schema_test.go"
Task: "Write failing test asserting CommandSchema.NonDeterministicFields for registered vs. unregistered commands in schema/schema_test.go"
Task: "Write failing test asserting internal mcp.Tool.NonDeterministicFields matches the CommandSchema union in internal/mcp/mcp_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories).
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: run the root golden-fixture tests (T021) and confirm `non_deterministic_fields`/`nonDeterministicFields` are present and correct on every node.
5. This alone satisfies FR-001, FR-002, FR-005 (empty-list case), FR-006, and FR-007 for any ax-go-based CLI, even before any command author tags a domain field.

### Incremental Delivery

1. Setup + Foundational → mechanism ready, fully unit-tested, nothing wired yet.
2. Add User Story 1 → root `__schema`/MCP/error-envelope output always carries the field → validate via golden tests.
3. Add User Story 2 → a real author-tagged domain field (`data.entity_id`) flows through `examples/integration` end-to-end → validate via regenerated example goldens.
4. Add User Story 3 → regression-catching is demonstrated directly and documented → validate via the dedicated unit test plus `make cover-check`/`make bench-check`.
5. Polish → full-repo verification, quickstart.md walkthrough, success-criteria review.

---

## Notes

- [P] tasks touch different files or independent functions within the same file — no shared state conflicts.
- [Story] labels map every Phase 3+ task to spec.md's US1/US2/US3 for traceability.
- No new Go module dependency is introduced (plan.md Technical Context): all reflection uses stdlib `reflect` and `encoding/json`-compatible tag parsing.
- No `schema_version`/`ErrorSchemaVersion` bump is required or expected (research.md D7) — do not add one.
- Adding entries to a command's `non_deterministic_fields` list is non-breaking; removing a previously-shipped entry is breaking and requires the `breaking-change-approved` label plus a `feat!:`/`BREAKING CHANGE:` commit (FR-010) — not applicable to this initial implementation, but binding on future changes to the lists this feature establishes. The golden-file tests (T020/T021 at the root, T026/T028 in `examples/integration`) are the mechanical trip-wire that catches a silent list-content regression; the PR-label/commit-type rule is the human-review policy layered on top of that mechanical catch, not a separate enforcement path.
