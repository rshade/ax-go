# Tasks: Cut v0.1.0 — Make ax-go a Consumable, Pinnable Release

**Input**: Design documents from `/specs/003-cut-v010-release/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: REQUIRED — golden-file tests are the deliverable of Workstream B
(FR-004–FR-008) and the constitution mandates test-first (Principle VII).

**Organization**: Tasks grouped by user story. Execution order intentionally
differs from priority order: US2 (P2) and US3 (P3) MUST complete before US1's
(P1) release cut, because the tag is immutable once minted (spec US2 rationale).

**Commit policy (per maintainer instruction)**: No intermediate commits. All
work is committed at the end by the maintainer (tasks marked **USER**). The
agent never runs `git add`/`git commit`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1–US4 from spec.md
- **USER**: performed/approved by the maintainer, not the agent

---

## Phase 1: Setup — Repair the Working Tree

**Purpose**: The tree currently does not compile: stash-pop conflict markers
exist in two files (verified 2026-06-09; research D10). Nothing else can
proceed.

- [X] T001 Resolve stash-pop conflict markers in `example_test.go` (markers at
      lines 166/173/237: `<<<<<<< Updated upstream` vs `>>>>>>> Stashed
      changes`). Reconcile both sides — keep the union of intended examples;
      neither side may be dropped without inspecting what each adds.
- [X] T002 Resolve stash-pop conflict markers in `internal/cmd/doccover/main.go`
      (markers at lines 59/61/64). Same reconciliation rule as T001.
- [X] T003 Verify the tree compiles and vets cleanly: `go build ./...`,
      `go vet ./...`, `gofmt -l .` (expect no output from gofmt).

**Checkpoint**: Tree compiles — feature work can begin.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Baseline green before contract-freezing work starts (FR-014
becomes provable).

- [X] T004 Confirm the two staged patch fixtures remain tracked and exercised:
      `git ls-files testdata | wc -l` includes
      `testdata/config_patch_invalid.golden.json` and
      `testdata/config_patch_hujson_invalid.golden.json`, and
      `go test -race -run TestPatchConfig ./...` (or the owning test names in
      `config_test.go:979–987`) passes (research D6).
- [X] T005 Establish the CI baseline: `make ci` (test -race + vet +
      golangci-lint + doc-coverage) fully green. Use an extended timeout
      (golangci-lint can exceed 5 minutes).

**Checkpoint**: `make ci` green — user stories can begin.

---

## Phase 3: User Story 2 — Freeze public output contracts (Priority: P2, executes FIRST) 🎯

**Goal**: Golden fixtures for the success `Envelope[T]` shape and the NDJSON
line shape, locked before the immutable tag (FR-004–FR-008).

**Independent Test**: Both golden tests pass in `go test -race ./...` with no
tag existing; flipping any fixture byte fails the suite.

### Tests for User Story 2 (test-first — MUST fail before fixtures exist)

- [X] T006 [US2] Write `TestEnvelopeGolden` in `json_test.go` per
      `contracts/success-envelope.md`: pin trace/span via
      `trace.ContextWithSpanContext` + fixed `SpanContextConfig`
      (trace `0102030405060708090a0b0c0d0e0f10`, span `0102030405060708`),
      idempotency key `00000000-0000-4000-8000-000000000001` via
      `ax.WithIdempotencyKey`; `data` = the contract's fixed struct; serialize
      with `WriteJSON` into a `bytes.Buffer`; assert with
      `assertGolden(t, "testdata/success_envelope.golden.json", ...)`.
      Run it — MUST fail at `os.ReadFile` ("read golden ... no such file"),
      the correct TDD failure (research D7).
- [X] T007 [US2] Write `TestWriteJSONLineGolden` in `json_test.go` per
      `contracts/ndjson-line.md` (distinct payload struct, key
      `...000000000002`), asserting
      `testdata/ndjson_line.golden.json`. Run it — MUST fail at `os.ReadFile`.

### Implementation for User Story 2

- [X] T008 [P] [US2] Create `testdata/success_envelope.golden.json` with the
      exact bytes specified in `contracts/success-envelope.md` (one minified
      line + `\n`). `TestEnvelopeGolden` now passes.
- [X] T009 [P] [US2] Create `testdata/ndjson_line.golden.json` with the exact
      bytes specified in `contracts/ndjson-line.md`. `TestWriteJSONLineGolden`
      now passes.
- [X] T010 [US2] Drift self-check (FR-006 / spec US2 acceptance 3): mutate one
      byte in each new fixture, confirm `go test -race .` fails, restore via
      `git checkout -- testdata/...`. Both tests run inside the standard suite
      — no build tags, no skips.
- [X] T011 [US2] FR-007 / SC-003 audit close-out: confirm 11 fixtures exist in
      `testdata/`, every one referenced from a `*_test.go` (audit table in
      research D6), covering all six specs/001 `error_code`s, the error
      envelope, both `__schema` formats, and the two new success contracts.

**Checkpoint**: All v0.1.0 public output contracts are golden-locked.

---

## Phase 4: User Story 3 — Integration example reports its real version (Priority: P3)

**Goal**: Verify spec-002 deliverables end-to-end (research D3 says complete —
these tasks assert it; code changes only if a check fails, which is a defect in
scope).

- [X] T012 [P] [US3] Static verification (SC-002 / FR-001 / FR-003):
      `grep -rn 'const version' examples/` returns nothing;
      `examples/integration/main.go` uses `var version string` +
      `ax.ResolveVersion(version)`. Spec-002 deliverable checks (FR-003):
      the public helper exists (`grep -n 'func ResolveVersion' version.go`)
      with its tests (`TestResolveVersionFrom`, `FuzzResolveVersion`); the
      Makefile `build-example` target injects
      `-ldflags "-X main.version=..."`; the README documents the recipe
      (`grep -n 'build-example' README.md` returns matches).
- [X] T013 [US3] Behavioral verification (FR-002, SC-005 pre-tag form):
      `make build-example`, then compare three surfaces byte-identically:
      (a) `./bin/ax-integration __schema` — extract the non-empty,
      VCS-derived `version` field from stdout; (b)
      `./bin/ax-integration fail` — extract the `version` field of the
      `ax.Error` envelope from stderr; (c) `./bin/ax-integration` (bare root
      invocation) — extract the `version` label from the logger line on
      stderr. All three strings MUST be identical.

**Checkpoint**: "Never ship `dev`/`unknown`" safeguard demonstrated end-to-end.

---

## Phase 5: User Story 1 — Cut the pinnable v0.1.0 release (Priority: P1, executes after US2/US3)

**Goal**: Release-please pipeline fixed, release PR proposes exactly `0.1.0`,
tag minted, module resolvable.

### Pre-release changes

- [X] T014 [US1] ~~Edit `.github/workflows/release-please.yml` (FR-009, research
      D2): replace `token: ${{ secrets.RELEASE_PLEASE_TOKEN }}` with
      `token: ${{ secrets.GITHUB_TOKEN }}`; delete the stale PAT/GoReleaser
      comment block.~~ **SUPERSEDED by maintainer (2026-06-09):
      `secrets.RELEASE_PLEASE_TOKEN` is correct and stays — workflow file
      unchanged.** `release-please-config.json` and
      `.release-please-manifest.json` remain untouched (research D1).
- [X] T015 [P] [US1] Update `README.md` status blockquote (line 9; FR-012,
      research D8): remove "🚧 Implementation scaffold", state the v0.1.0
      release status, link to `CHANGELOG.md`. Run
      `npm run lint:md` (markdownlint) after editing.
- [X] T016 [US1] Full pre-commit validation gauntlet: `gofmt -l .` (empty),
      `make ci` green (FR-014 / SC-006), `npm run lint:md` clean, and
      dependency check (FR-015): `git diff go.mod go.sum` shows no new
      modules.

### Release execution (USER-gated)

- [ ] T017 [US1] **USER**: Commit all work (single commit or maintainer's
      preferred split) as Conventional Commits. One commit on `main` MUST
      include the one-shot footer (research D1 — without it release-please
      proposes `1.0.0`, per release-please issue #2087; this is independent
      of the token decision in T014):

  ```text
  feat(release): freeze public output contracts for v0.1.0

  Release-As: 0.1.0
  ```

  Validate with `cat PR_MESSAGE.md | npx commitlint` if using the PR flow.
  Push to `main` (directly or via PR per maintainer preference).
- [ ] T018 [US1] Verify the triggered release-please run: green (not the
      7–9-second auth failure), and the opened release PR proposes **exactly
      `0.1.0`** — not `1.0.0`, not `0.0.1`. `gh run list
      --workflow=release-please.yml --limit 3`; `gh pr list`. **STOP if the
      version is wrong — do not merge.**
- [ ] T019 [US1] Inspect the release PR's `CHANGELOG.md` (FR-011 / SC-007): a
      `0.1.0` section generated solely from conventional-commit history via the
      `changelog-sections` mapping; zero hand-authored lines.
- [ ] T020 [US1] **USER**: Merge the release PR. Confirm the tag `v0.1.0` is
      minted and the merge-triggered workflow run is green (SC-004, spec US1
      acceptance 3).

### Post-tag verification

- [ ] T021 [US1] From a scratch directory outside the repo:
      `go mod init scratch && go get github.com/rshade/ax-go@v0.1.0` resolves
      in under 30s with a valid `go.sum` entry (FR-013, SC-001).
- [ ] T022 [US1] Build the example from the tag (`git checkout v0.1.0 && make
      build-example`): `__schema.version`, the `ax.Error` envelope `version`,
      and the logger `version` label all report `v0.1.0`, byte-identical
      (SC-005). Return to the working branch afterward.

**Checkpoint**: v0.1.0 is live and pinnable.

---

## Phase 6: User Story 4 — Pipeline runs automatically on future merges (Priority: P4)

**Goal**: The fix survives first use; the next release needs no operator
intervention.

- [ ] T023 [US4] Configuration durability check: ~~no `RELEASE_PLEASE_TOKEN`
      reference remains anywhere~~ **per the maintainer decision in T014 the
      workflow keeps `secrets.RELEASE_PLEASE_TOKEN`** — verify the PAT secret
      authenticates (release-please run green, not the 7–9-second auth
      failure); workflow `permissions` block still grants `contents: write`,
      `issues: write`, `pull-requests: write`.
- [ ] T024 [US4] On the next conventional-commit push to `main` after v0.1.0
      (deferred observation — may complete with the next feature): confirm
      release-please runs green and, for a `feat:`/`fix:` push, opens a release
      PR consistent with the bump policy (`fix:` → `0.1.1`-style patch). Record
      the observed outcome here.

---

## Phase 7: Polish & Cross-Cutting

- [X] T025 [P] markdownlint all feature artifacts and modified docs:
      `markdownlint specs/003-cut-v010-release/**/*.md README.md` — clean.
- [ ] T026 Run the full `specs/003-cut-v010-release/quickstart.md` Definition
      of Done checklist; tick every box or file a defect.

> No ADR-retirement task: Governing ADR(s) = N/A (plan.md / research D9).
> ADR-0003 remains frozen in `docs/adr/`.

---

## Dependencies & Execution Order

### Phase Dependencies

```text
Phase 1 (Setup: fix conflicts) ─► Phase 2 (Foundational: make ci green)
        ─► Phase 3 (US2: golden fixtures)  ──┐
        ─► Phase 4 (US3: version verify) [P] ┼─► Phase 5 (US1: release cut)
        ─► T014/T015 (US1 pre-release edits) ┘        │
                                                      ▼
                                    Phase 6 (US4: durability) ─► Phase 7 (Polish)
```

- **Phases 3, 4, and US1's pre-release edits (T014–T015) are mutually
  parallel** after Phase 2 — different files, no shared state.
- **T017 (USER commit+push) is the hard serialization point**: everything
  before it must be complete and validated (T016).
- **T024 is deferrable** beyond this feature's close — it observes the *next*
  push to `main`.

### Within User Story 2 (test-first, Constitution VII)

- T006, T007 (failing tests) before T008, T009 (fixtures) — verify each test
  fails at `os.ReadFile` for the right reason first.
- T008 ∥ T009 (different fixture files).
- T010, T011 after both fixtures exist.

### Parallel Opportunities

```text
After Phase 2 completes, run concurrently:
  Agent A: T006 → T007 → T008 ∥ T009 → T010 → T011   (US2)
  Agent B: T012 ∥ T013                                (US3)
  Agent C: T014 ∥ T015                                (US1 pre-release edits)
Then converge on T016 → T017 (USER).
```

---

## Implementation Strategy

**Single-PR convergence**: Because the maintainer commits everything at the
end (T017), all of Phases 1–5-pre-release land as one changeset whose
workflow-fix commit carries `Release-As: 0.1.0`. The release PR that
release-please opens in response is the second and final merge. Stop-points:

1. After T005 — tree healthy; safe to pause.
2. After T011 — contracts frozen; the tag is now *safe to mint* whenever.
3. After T016 — everything staged for the USER commit; nothing further is
   agent work until the release PR appears.
4. T018's version check is the last gate before immutability — if the PR says
   `1.0.0`, the `Release-As` footer was lost; fix the commit message, do NOT
   merge.
