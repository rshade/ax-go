# Tasks: Certify and Internalize the Public Boundary Before v1.0

**Input**: Design documents from `/specs/015-internalize-helpers/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/ (audit-schema.md, baseline-schema.md, surfacecheck-output.md), quickstart.md

**Tests**: INCLUDED and tests-first. Constitution Principle VII and plan.md mandate
that tests land before implementation: write each test task, verify it fails for
the right reason, then implement.

**Organization**: Tasks are grouped by user story. Unlike a typical feature, the
stories here are strictly sequential: US2 enacts US1's reviewed audit, and US3's
CI wiring needs US1's committed artifacts to stay green. See Dependencies.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Every task names its exact file path(s)

**Governing ADR(s)**: none (plan.md records N/A; ADRs are frozen). Per the
template rule, there is intentionally NO ADR-retirement task in this plan.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Package scaffolding for the new internal gate; zero new module
dependencies (plan.md constraint).

- [X] T001 Create the `internal/cmd/surfacecheck/` package directory and `internal/cmd/surfacecheck/testdata/` fixture directory per the plan.md project structure; confirm `go.mod` gains no new dependency

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The complete `surfacecheck` tool (six-profile type-aware scanner,
strict artifact parsers, drift engine, deterministic CLI contract). It is the
blocking prerequisite for every story: US1 needs its `-list`/`-audit-seed`/check
modes to certify the surface, and US3's gate *is* this tool wired into Make/CI.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete. The
tool validates against `internal/cmd/surfacecheck/testdata/` fixtures here; the
committed `baseline.json` and audit arrive in US1.

### Tests for the Foundational tool (write FIRST, verify they FAIL)

- [X] T002 [P] Write table-driven scanner fixture tests in `internal/cmd/surfacecheck/inventory_test.go` with synthetic fixture packages under `internal/cmd/surfacecheck/testdata/` (materialize fixtures into a temp dir for `go list`, since the go tool skips `testdata`): exported package declarations, aliases, direct and promoted fields, embedded-interface method sets, value vs pointer-only method sets, promoted-member ambiguity, reachable hidden concrete types, build-tagged divergence, and `_test.go` exclusion per research D1/D2
- [X] T003 [P] Write strict-parser, schema-validation, and `FuzzXxx` fuzz tests in `internal/cmd/surfacecheck/main_test.go` for the baseline and audit JSON surfaces: 1 MiB read cap, unknown fields, trailing values, unsorted/duplicate IDs, classification/disposition pairing, lifecycle rules, and leak-required fields per `specs/015-internalize-helpers/contracts/audit-schema.md` and `specs/015-internalize-helpers/contracts/baseline-schema.md`
- [X] T004 Write golden, stream/exit-matrix, and repeated-run determinism tests in `internal/cmd/surfacecheck/main_test.go` with goldens under `internal/cmd/surfacecheck/testdata/`: minified pass JSON (exit 0, empty stderr), `-list` inventory JSON, `-audit-seed` shape, drift → exit 2 with one minified `ax.Error` (`surface_drift`), invalid artifact/flags → exit 2 (`invalid_surface_artifact`), permission → 4, internal → 1, and byte-identical repeated scans per `specs/015-internalize-helpers/contracts/surfacecheck-output.md`

### Implementation for the Foundational tool

- [X] T005 Implement the API-feature model in `internal/cmd/surfacecheck/inventory.go`: canonical IDs (`const:`/`var:`/`func:`/`type:`/`field:`/`interface-method:`/`method:` with the `*` pointer-owner marker), canonical signature strings, the six target profiles (linux/darwin/windows × amd64/arm64), and bytewise ID ordering per `specs/015-internalize-helpers/data-model.md` and research D2
- [X] T006 Implement the six-profile scanner in `internal/cmd/surfacecheck/inventory.go`: per profile run `go list -deps -export -json .` from a configurable directory with GOOS/GOARCH env (context-cancelable, bounded output), load the root package via stdlib `go/importer.ForCompiler(..., "gc", lookup)`, and traverse the resulting `*types.Package` for exports, direct/promoted fields, complete interface method sets, value/pointer method sets, alias-attributed members, and reachable hidden concrete types; fail closed on profile divergence per research D1/D2
- [X] T007 Implement strict size-capped decoders and cross-field validation for the baseline and audit documents in `internal/cmd/surfacecheck/main.go`: `DisallowUnknownFields`, trailing-value rejection, sortedness/uniqueness checks, and the classification/disposition/lifecycle cross-field rules from both schema contracts
- [X] T008 Implement the drift engine and audit cross-validation in `internal/cmd/surfacecheck/inventory.go`: `added`, `missing`, `signature-changed`, `profile-divergent`, `audit-missing`, `audit-state-invalid`, plus the `deprecation-missing` check that inspects root declaration doc comments via `go/ast` for a valid `Deprecated:` paragraph; drift items sort by (id, drift, expected, actual) per `specs/015-internalize-helpers/data-model.md`
- [X] T009 Implement the CLI contract in `internal/cmd/surfacecheck/main.go`: `flag.ContinueOnError` with its writer discarded; default check mode plus `-list`, `-audit-seed`, `-baseline <path>`, `-audit <path>`; one minified struct-backed JSON result on stdout for success modes; exactly one minified `ax.Error` envelope on stderr with codes `surface_drift`/`invalid_surface_artifact`/`surface_permission`/`surface_internal` and exit codes 0/1/2/4 per `specs/015-internalize-helpers/contracts/surfacecheck-output.md`
- [X] T010 Run `go test -race ./internal/cmd/surfacecheck/` and confirm the full tool suite passes, after having confirmed T002–T004 failed before implementation for the right reasons

**Checkpoint**: The tool is complete against fixtures — US1 can now generate
real artifacts, and no story is blocked.

---

## Phase 3: User Story 1 - Permanent certification of the current surface (Priority: P1) 🎯 MVP

**Goal**: A committed, reviewed `public-surface-audit.json` with exactly one
retained decision per compiler-visible root API feature, plus the initial live
`baseline.json`, both machine-validated by the gate.

**Independent Test**: `go run ./internal/cmd/surfacecheck` from the module root
exits 0 with the pass JSON; the pass proves the six profiles expose one
invariant canonical feature set and every feature maps to exactly one retained
audit decision (spec US1 acceptance).

- [X] T011 [P] [US1] Generate the live baseline with `go run ./internal/cmd/surfacecheck -list` and commit it as `internal/cmd/surfacecheck/baseline.json` per `specs/015-internalize-helpers/contracts/baseline-schema.md` (minified, bytewise-sorted features, one trailing newline, ≤1 MiB)
- [X] T012 [P] [US1] Generate the audit seed with `go run ./internal/cmd/surfacecheck -audit-seed` into a working copy of `specs/015-internalize-helpers/public-surface-audit.json`; the seed is intentionally invalid until reviewed and MUST NOT be committed unclassified
- [X] T013 [US1] Classify every seed record in `specs/015-internalize-helpers/public-surface-audit.json`: `supported` or `implementation-leak` with a one-line `rationale`, `disposition`, and lifecycle `live`; for leaks also fill `internal_target` (cohesive `internal/<role>`, never `internal/helpers`), `replacement`, and `compatibility_strategy`; facade aliases/wrappers stay `supported` absent explicit contract evidence, and ambiguity resolves to `supported` per spec US1 scenarios 2–4
- [X] T014 [US1] Record leak evidence in `specs/015-internalize-helpers/public-surface-audit.json`: in-repo call sites, indexed downstream-search results with the `downstream_checked_at` date, and earliest verified published presence (`git tag --contains`) in `first_published`; absence of hits is evidence only, never proof that removal is safe
- [X] T015 [US1] Commit the reviewed audit at `specs/015-internalize-helpers/public-surface-audit.json`: one record per canonical feature across all six profiles, records sorted bytewise by `id`, evidence arrays sorted, ≤1 MiB, one trailing newline, two-space indentation for review per `specs/015-internalize-helpers/data-model.md`
- [X] T016 [US1] Verify certification with `go run ./internal/cmd/surfacecheck`: exit 0, empty stderr, pass JSON whose `features_checked` equals `audit_records_checked` and whose `profiles_checked` is 6 — proving profile invariance and one retained decision per feature against `internal/cmd/surfacecheck/baseline.json` and `specs/015-internalize-helpers/public-surface-audit.json`

**Checkpoint**: The audit is the approval boundary — NO migration task (Phase 4)
starts until this artifact is reviewed and committed (plan.md checkpoint 2).

---

## Phase 4: User Story 2 - Non-public mechanics internalized without an early break (Priority: P2)

**Goal**: Every audit-approved leak moves behind a stable compatibility seam:
mechanics in cohesive `internal/` packages, root forwarders unchanged in
name/type/semantics with valid `Deprecated:` paragraphs, zero incompatible
`go-apidiff` findings, zero removed exports.

**Independent Test**: For every audit-approved leak, its mechanics live under
`internal/`, the root declaration retains the same name/type/semantics with a
Go-recognized `Deprecated:` paragraph, all in-repo call sites use the
replacement (SA1019 clean), and `go-apidiff` reports no incompatible change
(spec US2 acceptance).

If the audit approves zero leaks, this phase reduces to a recorded confirmation
in T022 and the remaining tasks are marked not-applicable.

- [X] T017 [US2] ~~For each `relocate-with-forwarder` audit record, write failing compatibility tests~~ **Not applicable: the committed audit approved zero leaks, so no compatibility tests exist to write (tasks.md zero-leak provision).**
- [X] T018 [US2] ~~Move each approved leak's mechanics into its audited cohesive `internal/<role>` package~~ **Not applicable: zero approved leaks; no mechanics moved and no forwarders created.**
- [X] T019 [US2] ~~Add a Go-recognized deprecation paragraph to every retirement candidate~~ **Not applicable: zero approved leaks means zero retirement candidates; no `Deprecated:` paragraphs added in feature 015.**
- [X] T020 [US2] ~~Migrate every in-repo call site to the supported replacement~~ **Not applicable: zero approved leaks; no replacements introduced, so no call sites migrated. SA1019 remains clean.**
- [X] T021 [US2] ~~Transition each migrated record's `lifecycle` to `deprecated`~~ **Not applicable: zero approved leaks; every audit row stays `live` and every baseline entry remains.**
- [X] T022 [US2] Verify US2 end-to-end: `go-apidiff` reports no incompatible change, `testdata/` error-envelope and `__schema` goldens pass unmodified, `make surface-check` (run as `go run ./internal/cmd/surfacecheck`) passes with the updated audit, SA1019 is clean, and `go test -race ./...` stays green

**Checkpoint**: Internalization is complete with the public boundary intact; the
feature carries notices only — removal is out of scope (FR-008/FR-009).

---

## Phase 5: User Story 3 - Boundary changes remain explicit and machine-checkable (Priority: P3)

**Goal**: The gate is wired into local and CI workflows so any new, removed, or
signature-changed API feature fails deterministically until the baseline and
audit carry a reviewed decision.

**Independent Test**: Add a scratch package declaration, exported field,
interface method, promoted selector, or platform-specific export and confirm
the gate returns exit 2, leaves stdout empty, and writes exactly one minified
`ax.Error` envelope to stderr (spec US3 acceptance).

- [X] T023 [P] [US3] Add a phony `surface-check` target to `Makefile` with the `@`-prefixed recipe `@go run ./internal/cmd/surfacecheck` (no Make echo on stdout), add it to the `ci` aggregate, and document it in the `help` target
- [X] T024 [P] [US3] Add an explicit surface-check step running `go run ./internal/cmd/surfacecheck` to the validate job in `.github/workflows/ci.yml`, placed next to the existing doc-coverage step
- [X] T025 [P] [US3] Document the gate in `AGENTS.md`: purpose, module-root invocation (`make surface-check` / `go run ./internal/cmd/surfacecheck` / nested `make -C`), the exit/error-code contract, the baseline+audit change protocol from `specs/015-internalize-helpers/contracts/`, and that `internal/cmd/surfacecheck` faces the 25% default per-package coverage floor
- [X] T026 [US3] Verify the guard end-to-end per `specs/015-internalize-helpers/contracts/surfacecheck-output.md`: `make surface-check` exits 0 with empty stderr; a scratch exported addition (reverted afterwards) fails with exit 2, empty stdout, and one minified `ax.Error`; and every `deprecated` audit row maps to a present source declaration carrying a valid `Deprecated:` paragraph

**Checkpoint**: The boundary is now self-defending in `make ci` and CI; future
surface drift cannot merge silently.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Whole-feature verification and the release handoff. No ADR
retirement task exists because no ADR governs this feature (plan.md).

- [X] T027 [P] Validate every command in `specs/015-internalize-helpers/quickstart.md` from the module root: `make surface-check`, direct `go run ./internal/cmd/surfacecheck`, the nested `make -C "$(git rev-parse --show-toplevel)" surface-check` form, both bootstrap modes, and the failure-reading examples
- [X] T028 Run the full verification suite from `specs/015-internalize-helpers/quickstart.md`: `gofmt -s -l .` prints nothing, `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `make doc-coverage`, `make cover-check`, `make surface-check`, and `make bench-check` all pass with no floors or budgets lowered (`internal/cmd/covercheck/main.go` and `internal/cmd/benchcheck/main.go` untouched). **Note: every gate passed except `make bench-check`, which is environmentally unreliable on the shared 2-core verification host at verification time: it failed twice against the real worktree with disjoint failing benchmark sets (+9%…+460% ns/op), then failed a control run with byte-identical source (60fa703) on both sides (+6.1%/+9.2%). The feature diff changes zero `.go` files outside `internal/cmd/surfacecheck` (no benchmarks), so the change is provably performance-neutral; CI on a dedicated runner is the authoritative bench-check verdict.**
- [X] T029 [P] Confirm governance invariants: zero files under `docs/adr/` created or edited, `CHANGELOG.md` untouched, the public-package allowlist in `internal/cmd/apidiff-verdict` unchanged, and `examples/integration/` importing only supported public API
- [ ] T030 Create the follow-up Spec Kit removal-feature tracking issue (scope: verify a real published `0.MINOR.0` carrying the notices, transition audit rows `deprecated`→`removable`→`removed`, delete baseline entries with the source exports, apply `breaking-change-approved` with a `feat!:`/`BREAKING CHANGE:` commit), reference it from `specs/015-internalize-helpers/quickstart.md`, and land feature 015 as a non-breaking `feat:` commit. **Partial: the quickstart.md follow-up reference and the `feat:` commit are done; the GitHub tracking issue was intentionally skipped per maintainer decision (also moot while the audit carries zero `deprecated` rows) — create it when a deprecation first exists.**

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories. The
  tool is the shared prerequisite: US1 cannot certify without it and US3's gate
  is this tool.
- **User Stories (Phases 3–5)**: STRICTLY SEQUENTIAL, unlike the usual parallel
  story layout:
  - **US2 depends on US1**: plan.md checkpoint 2 forbids any migration until the
    complete audit is reviewed and committed.
  - **US3 depends on US1**: wiring the gate into `make ci`/CI before
    `baseline.json` and the audit exist would fail every run
    (`invalid_surface_artifact`).
- **Polish (Phase 6)**: Depends on all three stories.

### User Story Dependencies

- **US1 (P1)**: Starts after Foundational. No dependency on other stories.
- **US2 (P2)**: Starts after US1's reviewed audit is committed. Consumes only
  the audit's `relocate-with-forwarder` / `deprecate-in-place` records.
- **US3 (P3)**: Starts after US1 (artifacts committed); may start while US2 is
  in flight only if coordinated so `make surface-check` stays green — safest
  after US2.

### Within Each Phase

- Foundational: tests T002–T004 FIRST (verify failure), then T005→T006→T008
  (same file `inventory.go`, sequential), T007→T009 (same file `main.go`,
  sequential), then T010.
- US1: T013→T014→T015 edit the same audit file sequentially; T016 verifies.
- US2: per audit record, T017→T018→T019→T020→T021; records targeting disjoint
  `internal/<role>` packages may be worked in parallel; T022 verifies last.
- US3: T026 verifies after the wiring tasks.

### Parallel Opportunities

- T002 and T003 (different test files) in parallel; T004 follows T003 (same file).
- T011 and T012 (different output files) in parallel.
- T023, T024, T025 (Makefile, ci.yml, AGENTS.md) in parallel.
- T027 and T029 (different concerns, read-only verification) in parallel.
- US2 per-record work in disjoint `internal/<role>` packages.

---

## Parallel Example: User Story 1

```bash
# Bootstrap both artifacts together (different files, tool is read-only):
Task: "Generate the live baseline into internal/cmd/surfacecheck/baseline.json"
Task: "Generate the audit seed working copy of specs/015-internalize-helpers/public-surface-audit.json"

# Then classify sequentially (same file): T013 → T014 → T015, verify with T016.
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational tool (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1 — the certified audit + committed baseline,
   gate passing locally
4. **STOP and VALIDATE**: `go run ./internal/cmd/surfacecheck` exits 0; the
   audit is reviewed before any code moves (plan.md checkpoint 2)

### Incremental Delivery

1. Setup + Foundational → tool proven against fixtures
2. US1 → permanent certification committed (MVP: the reviewable v1 candidate
   surface exists)
3. US2 → approved leaks internalized behind deprecated forwarders; apidiff and
   goldens unchanged
4. US3 → boundary enforced in `make ci` and CI
5. Polish → full gate suite green; follow-up removal feature handed off

### Sequencing Cautions

- Do NOT wire CI (Phase 5) before the artifacts exist (Phase 3): the gate fails
  closed on missing artifacts by design.
- Do NOT remove any export anywhere in this feature (FR-008); removal belongs
  to the follow-up feature tracked in T030.
- Keep `testdata/` goldens, coverage floors, and benchmark budgets byte/number
  identical throughout (FR-010/FR-011).

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks.
- [USn] labels map tasks to spec.md user stories for traceability.
- The audit's `classification` review (T013/T014) is the human judgment core of
  this feature; everything else is mechanical and gate-verified.
- The committed audit uses two-space indentation per data-model.md
  ("committed formatting is two-space indented for review"); contracts/
  audit-schema.md's "minified" encoding line describes the tool-emitted byte
  stream — flag any reconciliation to the reviewer rather than guessing.
- Commit after each task or logical group (Conventional Commits; feature 015
  lands as non-breaking `feat:`).
- Stop at any checkpoint to validate the story independently.
