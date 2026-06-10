---
description: "Task list for Build-time Version Injection via -ldflags"
---

# Tasks: Build-time Version Injection via -ldflags

**Input**: Design documents from `/specs/002-version-injection/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md,
contracts/version-api.md, quickstart.md

**Tests**: INCLUDED. The constitution's Test-First Discipline (Principle VII)
and FR-009 / FR-011 make tests a required deliverable for this feature, so test
tasks lead each behavioral change and must fail for the right reason before the
implementation lands.

**Organization**: Tasks are grouped by user story (P1 → P2 → P3) so each story
is an independently testable increment. The stories are intentionally sequential
(per spec: US2 depends on US1's plumbing; US3 builds on US1+US2's resolution
behavior).

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependency on an
  incomplete task)
- **[Story]**: Maps the task to a user story (US1/US2/US3); omitted for Setup,
  Foundational, and Polish tasks

## Path Conventions

Single Go library at the module root (`github.com/rshade/ax-go`, package `ax`)
plus the `examples/integration` reference binary. All paths below are relative
to the repository root.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish a clean attribution baseline and confirm the build-path
assumption before any change.

- [X] T001 Capture a green baseline before changes: run `go test -race ./...`,
  `go vet ./...`, and `make doc-coverage` from the repo root and confirm all pass,
  so any later failure is attributable to this feature.
- [X] T002 [P] Confirm the documented build path assumption (FR-003): run
  `git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-unknown` and
  verify it yields a non-empty value (e.g. `5bf9b77-dirty`) with the repo's
  current zero-tag state.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Create the shared `version.go` seam and its test scaffold that ALL
three user stories compile and build their tests against.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 Create `version_test.go` (package `ax`, repo root) test scaffold: a
  synthetic `*debug.BuildInfo` builder helper and a table-driven runner over
  `resolveVersionFrom(injected, info, ok)`, seeded with the sentinel-floor case
  (`injected==""`, `ok==false` → `0.0.0-unknown`). Run it and confirm it FAILS
  (function not yet implemented).
- [X] T004 Create `version.go` (package `ax`, repo root): the unexported
  `const versionUnknown = "0.0.0-unknown"` sentinel; the public
  `func ResolveVersion(injected string) string` that calls
  `runtime/debug.ReadBuildInfo()` exactly once and delegates to the pure seam;
  and the unexported `func resolveVersionFrom(injected string, info *debug.BuildInfo, ok bool) string`
  implementing only the sentinel floor for now (returns `versionUnknown`). Add
  the contract doc comment on `ResolveVersion` (precedence order + the
  never-empty / never-`dev` / never-`unknown` guarantee, per
  contracts/version-api.md). No `context.Context`, no `panic` (research D5).
  Make T003's floor case pass.

**Checkpoint**: `version.go` compiles, `ResolveVersion` returns the sentinel,
`go test -race ./...` and `golangci-lint run` (godoclint `require-doc`) are
green. Resolution branches are added per story below.

---

## Phase 3: User Story 1 - A built binary reports a real version in `__schema` (Priority: P1) 🎯 MVP

**Goal**: A binary built through the documented (injected) path reports a real,
VCS-derived `__schema.version` instead of an empty/placeholder field.

**Independent Test**: `make build-example && ./bin/ax-integration __schema | jq -e '.version != "" and .version != "v0.1.0"'`
succeeds, and the value equals the `git describe --tags --always --dirty` output
for the source commit (FR-001 / SC-001 / SC-002).

### Tests for User Story 1 ⚠️ (write first, confirm they FAIL)

- [X] T005 [P] [US1] In `version_test.go`, add the injected-wins table case
  (resolution row 1): a non-placeholder `injected != ""` returns `injected`
  verbatim regardless of build info (e.g. `v1.2.3`, `5bf9b77-dirty`), while bare
  placeholders fall through. Confirm it FAILS against the T004 sentinel-only
  stub.
- [X] T006 [P] [US1] In `examples/integration/main_test.go`, add a test that
  invokes the example's `__schema` command and asserts the `version` field is
  non-empty and is no longer the removed hardcoded `"v0.1.0"` placeholder
  (FR-009). Confirm it FAILS while `main.go` still uses the const.
- [X] T006a [US1] In `examples/integration/main_test.go`, add a test giving
  SC-006 a verified artifact for the THIRD version surface — the logger
  `version` label (today asserted nowhere). Compute
  `want := ax.ResolveVersion(version)`. Run the ROOT command (e.g.
  `[]string{"--name", "Ada"}`) with stdout and stderr captured separately: the
  root `RunE` emits one zerolog Info line to stderr (`logger.go` defaults to
  `InfoLevel`; the label renders as the JSON field `version` via `applyLabels`,
  and is emitted only when non-empty — which `ax.ResolveVersion` guarantees).
  Scan the stderr lines for the JSON object with a non-empty `version` field,
  decode that field, and assert it equals `want`. Then run `__schema` separately
  and assert `Schema.Version` equals `want`. Together with the T008-updated fail
  test (the `ax.Error` envelope `version` == `want`), this pins all three
  surfaces byte-identical to the one resolved value (FR-007 / SC-006). NOT `[P]`
  with T006 — both edit `examples/integration/main_test.go`, so run T006a after
  T006. Confirm it FAILS while `main.go` still feeds the `const` to its surfaces.

### Implementation for User Story 1

- [X] T007 [P] [US1] In `version.go`, implement precedence step 1 (usable
  injected values win) in `resolveVersionFrom`. Make T005 pass.
- [X] T008 [P] [US1] In `examples/integration/main.go`, change
  `const version = "v0.1.0"` (line 14) to `var version string`. In `run()`,
  resolve ONCE via `resolved := ax.ResolveVersion(version)` and pass that single
  value to `ax.WithVersion(resolved)` (replacing `ax.WithVersion(version)` at
  line 52). Thread the SAME `resolved` into the logger label: the logger is
  built in `newRootCommand`'s `RunE` closure (lines 72-75, a DIFFERENT scope from
  `run()`), so add a `resolved string` parameter to `newRootCommand`, call it as
  `newRootCommand(stdin, resolved)`, and use that parameter for
  `ax.Labels{Version: resolved}` (line 74) instead of the package-level
  `version`. One source of truth for all three surfaces (FR-007 / SC-006 /
  research D6). THEN, in `examples/integration/main_test.go`, update the
  pre-existing `TestRunFailCommandWritesErrorEnvelopeToStderr` assertion (lines
  192-193): it currently compares `got.Version` against the package `version`,
  which becomes the empty `var` after this change — replace it with
  `want := ax.ResolveVersion(version)` and assert `got.Version == want`. Without
  this the existing test breaks (the envelope carries the resolved fallback while
  `version` is now `""`), failing the green baseline (T001) and the T018
  gauntlet. Both sides call `ax.ResolveVersion(version)`, so the assertion holds
  deterministically across US1 and US2. Make T006 pass.
- [X] T009 [P] [US1] In `Makefile`, add
  `VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-unknown)`
  and a `build-example` target running
  `go build -ldflags "-X main.version=$(VERSION)" -o bin/ax-integration ./examples/integration`
  (real TAB indentation); register it in `.PHONY` and the `help` listing. Leave
  the existing library `build` target (`go build ./...`) unchanged — it is NOT an
  injection site (FR-010 / contracts build-target section).
- [X] T010 [P] [US1] Add `/bin/` to `.gitignore` so the `build-example` output
  (`bin/ax-integration`) is never committed and does not dirty the tree.

**Checkpoint**: `make build-example && ./bin/ax-integration __schema | jq -r '.version'`
prints a real `git describe` value; `make build-example VERSION=v1.2.3` overrides
it; `go test -race ./...` is green. MVP is independently demonstrable.

---

## Phase 4: User Story 2 - Even an un-injected build reports a meaningful version (Priority: P2)

**Goal**: A build with no link-time injection (`go run` / bare `go build` /
`go install ...@version`) still reports a real, non-empty version from embedded
build metadata, never empty and never the bare `dev`/`unknown`.

**Independent Test**: `go run ./examples/integration __schema | jq -r '.version'`
prints the commit revision (with a `-dirty` suffix when the tree is modified),
falling to `0.0.0-unknown` only when no VCS metadata exists at all (FR-004 /
SC-003).

### Tests for User Story 2 ⚠️ (write first, confirm they FAIL)

- [X] T011 [US2] In `version_test.go`, add the build-info fallback table cases
  (resolution rows 2–6 from data-model.md): (2) real `Main.Version` used;
  (3) `(devel)`/empty module + `vcs.revision` present + clean → `<revision>`;
  (4) same + `vcs.modified=="true"` → `<revision>-dirty`; (5) `(devel)` + no
  revision → `0.0.0-unknown`; (6) `ok==false` → `0.0.0-unknown`. Use the
  synthetic `*debug.BuildInfo` builder from T003. Confirm they FAIL against the
  injected-only resolver.

### Implementation for User Story 2

- [X] T012 [US2] In `version.go`, implement the build-info fallback in
  `resolveVersionFrom` (research D2): when `injected==""` and `ok`, return
  `info.Main.Version` if non-empty and not `"(devel)"`; else read the
  `vcs.revision` / `vcs.modified` entries from `info.Settings` and return
  `revision` (appending `-dirty` when `vcs.modified=="true"`); otherwise return
  `versionUnknown`. Make T011 pass.
- [X] T013 [US2] Verify the un-injected path: run
  `go run ./examples/integration __schema | jq -r '.version'` and confirm it is a
  non-empty commit revision (with `-dirty` when the tree is modified), never
  empty and never bare `dev`/`unknown` (SC-003).

**Checkpoint**: Both the injected (US1) and un-injected (US2) paths report a real
non-empty version; `go test -race ./...` covers all six resolution rows.

---

## Phase 5: User Story 3 - Adopters replicate the pattern with one helper call and one build flag (Priority: P3)

**Goal**: The resolution behavior is a reusable public contract — one
`ax.ResolveVersion` call plus one documented build flag — backed by verified
artifacts, so adopters write zero build-metadata parsing.

**Independent Test**: Following the README recipe (helper call + `-ldflags`
flag) yields a real `__schema.version` with no adopter-written parsing; the
helper carries a verified `ExampleResolveVersion`; `make doc-coverage` is green
(FR-006 / FR-011 / SC-004).

### Tests / verified artifacts for User Story 3 ⚠️

- [X] T014 [US3] Add a verified `ExampleResolveVersion` to the root-package
  `example_test.go` with an `// Output:` comment, demonstrating the injected-value
  path (`ax.ResolveVersion("v1.2.3")` → deterministic output, independent of
  build info per research D8). Confirm `go test ./...` runs and verifies it.
- [X] T015 [US3] Add `"ResolveVersion"` to `requiredSymbols()` (entry points
  group) in `internal/cmd/doccover/main.go`, then run `make doc-coverage` and
  confirm it is green — the T014 example satisfies the gate with NO `baseline.txt`
  entry (research D7). (Depends on T014 existing first.)

### Documentation for User Story 3

- [X] T016 [P] [US3] In `README.md`, add a "Build-time version injection"
  adopter section documenting both the helper call (`ax.ResolveVersion(version)`
  fed to `ax.WithVersion` and `ax.WithLoggerLabels`) and the build recipe
  (`make build-example` / `-ldflags "-X main.version=..."`), per FR-011 and
  quickstart.md.
- [X] T017 [P] [US3] In `examples/integration/README.md`, document the
  `make build-example` / `-ldflags` build path and the non-empty, VCS-derived
  `__schema.version` it produces.

**Checkpoint**: The pattern is reusable and documented; `make doc-coverage`
green; an adopter wires it with one helper call + one flag (SC-004).

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and hygiene across all stories.

- [X] T018 Run the quickstart.md §4 validation gauntlet end-to-end:
  `go test -race ./...`; `make doc-coverage`;
  `make build-example && ./bin/ax-integration __schema | jq -e '.version != ""'`;
  `go vet ./...`; `golangci-lint run`. All MUST be clean (SC-001/SC-003/SC-006).
- [X] T019 [P] Run `gofmt` on changed Go files (`version.go`, `version_test.go`,
  `example_test.go`, `examples/integration/main.go`,
  `examples/integration/main_test.go`) and `markdownlint` on changed Markdown
  (`README.md`, `examples/integration/README.md`); fix any findings.
- [X] T020 [P] (OPTIONAL — research D8, may be deferred) Add a
  `FuzzResolveVersion` over `injected` and synthetic revision strings asserting
  the result is never empty and never panics. Not a required deliverable —
  `ResolveVersion` is not a byte-level parser surface.

> **No ADR-retirement task**: per research D9, the governing decision is
> constitution Principle X (already ratified); the feature's governing ADR(s) =
> N/A. ADR-0003 is referenced for context only and stays frozen — there is
> nothing to absorb or delete.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup — BLOCKS all user stories
  (creates the `version.go` seam + test scaffold everything builds on).
- **User Stories (Phase 3–5)**: All depend on Foundational. They are sequential
  by design (P1 → P2 → P3): US2's fallback extends the same `resolveVersionFrom`
  US1 starts; US3 promotes/documents the behavior US1+US2 define.
- **Polish (Phase 6)**: Depends on all desired user stories being complete.

### User Story Dependencies

- **US1 (P1)**: Depends only on Foundational. Independently testable (the
  injected build path).
- **US2 (P2)**: Depends on US1's `version.go` resolver existing (extends
  `resolveVersionFrom`). Independently testable (the un-injected path).
- **US3 (P3)**: Depends on US1+US2 having defined resolution; pairs the verified
  example with the `requiredSymbols()` promotion. Independently testable (the
  adopter recipe + doc-coverage gate).

### Within Each User Story

- Tests are written and FAIL before implementation (TDD per Principle VII).
- `version.go` resolver branch → consumer wiring → build target.
- `ExampleResolveVersion` (T014) MUST precede the `requiredSymbols()` promotion
  (T015), or `make doc-coverage` fails the new required symbol.

### Parallel Opportunities

- T002 (Setup) is read-only and `[P]`.
- US1 tests: T005 (`version_test.go`) is `[P]` with the `main_test.go` tests
  (different file). T006 and T006a both edit
  `examples/integration/main_test.go`, so they are sequential with each other
  (T006a after T006) but together run in parallel with T005.
- US1 implementation T007/T008/T009/T010 touch distinct files
  (`version.go`; `examples/integration/main.go` + `main_test.go`; `Makefile`;
  `.gitignore`) → `[P]` after the US1 tests. T008 also edits
  `examples/integration/main_test.go` to fix the pre-existing version assertion;
  this is conflict-free because T006/T006a (the other writers of that file)
  complete in the test phase before any US1 implementation begins.
- US3 docs T016 (`README.md`) and T017 (`examples/integration/README.md`) → `[P]`.
- Polish T019 / T020 → `[P]`.
- US2 tasks share `version.go` / `version_test.go` and run sequentially (no `[P]`).

---

## Parallel Example: User Story 1

```bash
# After Foundational completes, launch the US1 tests together (they must fail first):
Task: "T005 injected-wins table case in version_test.go"
Task: "T006 non-empty __schema.version assertion in examples/integration/main_test.go"
# T006a (logger version label == __schema == resolved, SC-006) runs after T006 — same file, not parallel

# Then launch the US1 implementation across distinct files together:
Task: "T007 injected-wins branch in version.go"
Task: "T008 const->var + resolve-once wiring (main.go, resolved threaded into newRootCommand) + fix pre-existing version assertion (main_test.go)"
Task: "T009 build-example target in Makefile"
Task: "T010 /bin/ in .gitignore"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1 (Setup) and Phase 2 (Foundational — the `version.go` seam).
2. Complete Phase 3 (US1): injected branch + example wiring + `build-example`.
3. **STOP and VALIDATE**:
   `make build-example && ./bin/ax-integration __schema | jq -r '.version'`
   shows a real VCS-derived version. This alone closes the empty-version defect
   for the documented build path (SC-001).

### Incremental Delivery

1. Setup + Foundational → resolver seam ready.
2. US1 → injected build path reports a real version → **MVP**.
3. US2 → even un-injected builds report a meaningful version (hardening).
4. US3 → reusable public helper + documented recipe + doc-coverage gate.
5. Polish → full validation gauntlet + formatting.

### Notes

- `[P]` tasks = different files, no dependency on an incomplete task.
- Verify each test FAILS for the right reason before implementing.
- Keep the resolver pure (`resolveVersionFrom`) so every branch is table-tested
  with a synthetic `*debug.BuildInfo` — never mock the toolchain.
- One `resolved` value feeds `__schema.version`, the `ax.Error` envelope, and
  the logger `version` label (FR-007 / SC-006) — resolve once, pass to both
  options.
