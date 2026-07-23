---
description: "Task list for 016-optional-grpc-otlp"
---

# Tasks: Independently Opt-Out OTLP Export and gRPC Dial Adapter

**Input**: Design documents from `/specs/016-optional-grpc-otlp/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/build-configuration.md, quickstart.md

**Tests**: Test tasks ARE included — Constitution Principle VII (Test-First Discipline) and AGENTS.md make them mandatory, and FR-021/SC-006/SC-008/SC-009 are stated as test assertions.

**Organization**: Grouped by user story so each is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: `[US1]`–`[US4]` map to spec.md user stories
- Exact file paths included in every task

## Path Conventions

Go library at the module root (`github.com/rshade/ax-go`). Public package `ax`
lives at `/`; private mechanics under `internal/`. No new public package.

**Governing ADR**: none (plan.md § Technical Context). No ADR-retirement task.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Dependency and measurement groundwork shared by all stories.

- [X] T001 Promote `golang.org/x/tools` to a direct dependency in `go.mod` (`go get golang.org/x/tools@v0.47.0 && go mod tidy`); verify against research.md D6 that the diff is +1 direct / +2 first-party `golang.org/x/*` indirect requires and +4 `go.sum` lines, with **zero** new upstream modules and zero version bumps
- [X] T002 [P] Capture the pre-implementation size/package baseline by running the reproduction script in `specs/016-optional-grpc-otlp/quickstart.md` (§ Reproducing the size measurement) against the current tree and saving the output for the SC-001/SC-002 comparison in T020

**Checkpoint**: Toolchain dependency available; before/after measurement anchor recorded.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The two prerequisites that must land before any file gains a build constraint.

**⚠️ CRITICAL**: T003 is the sequencing hazard named in plan.md § Critical sequencing constraints #1 and research.md D8 — gating `telemetry_export_test.go` before the helper moves breaks the declined build's test compilation.

- [X] T003 Move the shared helper `executeTelemetryCommand` from `telemetry_export_test.go:217` into a new **untagged** file `telemetry_helpers_test.go`, leaving `execute_test.go`'s call sites unchanged; verify `go test -race ./...` still passes
- [X] T004 Add a variadic `tags ...string` tail to `ResolvePackageImports` and `AssertNoForbiddenImports` in `internal/testutil/imports.go`, splicing `"-tags", strings.Join(tags, ",")` into the `go list` argv only when non-empty; confirm all four existing call sites compile unchanged
- [X] T005 Add a `ForbiddenGRPCTreeImports()` rule set to `internal/testutil/imports.go` with the four patterns from data-model.md §4 (`google.golang.org/grpc`, `google.golang.org/protobuf`, `go.opentelemetry.io/proto/otlp`, `github.com/grpc-ecosystem/grpc-gateway/v2`) — a **distinct** `[]ForbiddenImport`, NOT a reuse of `ForbiddenRuntimeImports()` (which would wrongly catch `stdouttrace`, research.md D9)
- [X] T006 Add table-driven tests for T004 and T005 in `internal/testutil/imports_test.go` covering: tags spliced when supplied, argv unchanged when omitted, and each of the four patterns prefix-matching a subpackage (`google.golang.org/grpc/status`)

**Checkpoint**: Test helper is tag-safe and the dependency-boundary primitives exist — user story work can begin.

---

## Phase 3: User Story 1 - Ship a slim binary without giving up tracing (Priority: P1) 🎯 MVP

**Goal**: Two negative build constraints (`ax_no_otlp`, `ax_no_grpc`) that, applied together, drop a root-facade binary ~63% and link zero packages from the four forbidden trees — while tracing degrades to *no export*, never to *no tracing*.

**Independent Test**: Build the quickstart fixture consumer with `-tags=ax_no_grpc,ax_no_otlp`; assert zero gRPC/protobuf/OTLP-proto/gateway packages in `go list -deps`, a materially smaller stripped artifact, and that inbound W3C context is still extracted, a recording root span still wraps execution, and log lines still carry `trace_id`/`span_id`.

### Tests for User Story 1 ⚠️

> **Write these FIRST and confirm they fail for the right reason.** Before the tags exist, an unknown tag is silently ignored, so the dependency assertion fails with gRPC packages still present — the correct failure.

- [X] T007 [US1] Write the zero-forbidden-dependency assertion in a new `buildtags_deps_test.go` at the module root: call `testutil.AssertNoForbiddenImports` for `github.com/rshade/ax-go` with tags `ax_no_grpc,ax_no_otlp` against `testutil.ForbiddenGRPCTreeImports()`, asserting an exact count of zero (FR-015, FR-016, SC-002, SC-008)
- [X] T008 [P] [US1] Write the behaviour-parity assertions in a new **untagged** `telemetry_parity_test.go` at the module root — W3C `TRACEPARENT`/`TRACESTATE` extraction, a recording root span around `Execute`, and `trace_id`/`span_id` on 100% of log lines emitted inside that span — so they execute under every configuration, not only the default (FR-009, FR-010, FR-021, SC-003)
- [X] T009 [P] [US1] Write the declined-export diagnostic test in a new `internal/telemetry/otlp_disabled_test.go` with `//go:build ax_no_otlp`: with an export endpoint configured, `Start` returns a nil error and a recording `*sdktrace.TracerProvider`, emits **exactly one** `otel exporter disabled` diagnostic on `cfg.Stderr` per `Start` call, and two successive `Start` calls emit exactly one each (FR-008, FR-009, SC-006)
- [X] T010 [P] [US1] Write a `//go:build ax_no_otlp` test in `internal/telemetry/otlp_disabled_test.go` asserting `Shutdown` completes within `DefaultShutdownBudget` and does not block on an absent exporter (FR-014)

### Implementation for User Story 1

- [X] T011 [US1] Create `internal/telemetry/otlp.go` with `//go:build !ax_no_otlp`, holding `newOTLPExporter` moved **verbatim** from `internal/telemetry/telemetry.go:139-162` together with the `otlptracehttp` import
- [X] T012 [US1] Create `internal/telemetry/otlp_disabled.go` with `//go:build ax_no_otlp`: `newOTLPExporter(ctx, cfg) (sdktrace.SpanExporter, error)` returning `nil` plus a sentinel error, and a file-level doc comment naming `ax_no_otlp` and the restoration step; it MUST declare no exported symbol and reference no type from the forbidden trees (FR-024, contracts C5)
- [X] T013 [US1] Remove `newOTLPExporter` and the `otlptracehttp` import from `internal/telemetry/telemetry.go`, leaving `normalizeOTLPEndpoint`, `diagnosticExporter`, `writeDiagnostic`, `SanitizeDiagnostic`, `lockedWriter`, `Config`, `Start`, `telemetryResource`, `DefaultShutdownBudget`, and the whole `stdouttrace` branch unconditional (research.md D3); confirm `Start` still routes exporter failure through `writeDiagnostic(cfg.Stderr, "otel exporter disabled", err)` so the existing `ax: otel` prefix is preserved
- [X] T014 [US1] Create `grpc.go` at the module root with `//go:build !ax_no_grpc`, holding `GRPCDial` moved from `http.go:63-72` plus the `context`, `grpc`, and `otelgrpc` imports
- [X] T015 [US1] Narrow `http.go` to HTTP helpers only — imports reduced to `net/http`, `time`, and `otelhttp`; `DefaultHTTPTimeout`, `HTTPClientOption`, `httpClientConfig`, `WithHTTPTimeout`, `HTTPClient`, and `NewHTTPClient` stay unconditional (FR-013)
- [X] T016 [US1] Create `grpc_disabled.go` at the module root with `//go:build ax_no_grpc`: a **doc-comment-only** file naming `ax_no_grpc` as the responsible constraint, stating that `ax.GRPCDial` is absent from this build, and giving the restoration step (drop the tag); no exported symbol, no stub, no forbidden-tree type (FR-022, FR-023, FR-024, SC-011)
- [X] T017 [US1] Split `http_test.go`: move the three `GRPCDial` tests at `:136`, `:152`, and `:179` into a new `grpc_test.go` with `//go:build !ax_no_grpc`, leaving the four HTTP tests untagged in `http_test.go`
- [X] T018 [US1] Add `//go:build !ax_no_otlp` to `telemetry_export_test.go` (safe only after T003)
- [X] T019 [P] [US1] Move the OTLP-receiver test at `telemetry_debug_test.go:46` into a new `telemetry_debug_otlp_test.go` with `//go:build !ax_no_otlp`, leaving the other three tests untagged
- [X] T020 [P] [US1] Add `//go:build !ax_no_otlp` to `telemetry_security_test.go` (it asserts `"ax: otel export failed"`, unreachable under the decline)
- [X] T021 [US1] Move the two OTLP-dependent tests at `execute_test.go:179` and `execute_test.go:205` into a new `execute_otlp_test.go` with `//go:build !ax_no_otlp`; leave `execute_test.go:126` (`TestExecuteTelemetryFailOpen`) **unchanged** — it survives via the shared `ax: otel` prefix (research.md D4/D8)
- [X] T022 [US1] Run the measurement script from `specs/016-optional-grpc-otlp/quickstart.md` on the implemented tree for linux/amd64 and windows/amd64; confirm ≥55% stripped-size reduction and exactly zero packages in each forbidden tree under `ax_no_grpc,ax_no_otlp`, and replace the hand-applied reference output in quickstart.md with the real measurement (FR-028, SC-001, SC-002)

**Checkpoint**: User Story 1 fully functional — `go build -tags=ax_no_grpc,ax_no_otlp ./...` and `go test -race -tags=ax_no_grpc,ax_no_otlp ./...` both pass, the size win is measured, and tracing still correlates.

---

## Phase 4: User Story 2 - Existing consumers observe no change whatsoever (Priority: P2)

**Goal**: The default build's exported surface, behaviour, and machine payloads are byte-identical to pre-feature — purely additive, no breaking release.

**Independent Test**: With no tags, the full existing test suite, the public-API difference gate, and the golden-file comparisons all pass unchanged and without any breaking-change approval.

### Tests for User Story 2 ⚠️

- [X] T023 [P] [US2] Confirm the existing golden fixtures under `testdata/` are unmodified by this feature and that `golden_test.go` passes on the default build with byte-identical `__schema` and `ax.Error` output (FR-007, SC-005)

### Implementation for User Story 2

- [X] T024 [US2] Run the `API Diff` gate path locally (`go run ./internal/cmd/apidiff-verdict` against the base branch) and confirm zero incompatible changes on the six-package public surface, requiring no `breaking-change-approved` label and no `feat!:` commit — zero incompatible change is the evidence that no consumer must change source, adopt a different import path, or migrate away from an identifier they use today (FR-004, FR-005, SC-004)
- [X] T025 [US2] Run the default-configuration gate suite unchanged — `go test -race ./...`, `go vet ./...`, `golangci-lint run`, `make doc-coverage`, `make cover-check` — confirming every existing behaviour (export, debug spans, W3C extraction, root span, log correlation) passes and **no coverage floor is lowered** (FR-006, SC-012)
- [X] T026 [US2] Run `make bench-check` and confirm the tracked benchmarks stay within the 5% ns/op and +1 allocs/op budget (plan.md § Performance Goals)

**Checkpoint**: Default consumers provably unaffected; the change is additive under Principle XI.

---

## Phase 5: User Story 3 - Each decline is independently selectable (Priority: P3)

**Goal**: All four configurations — default, `ax_no_grpc`, `ax_no_otlp`, and both — compile, run, and behave identically at the payload and CLI-contract level.

**Independent Test**: Build and run the fixture consumer in all four configurations; each compiles and succeeds, with identical machine payloads.

### Tests for User Story 3 ⚠️

- [X] T027 [P] [US3] Add an **untagged** payload-parity test in `buildtags_parity_test.go` at the module root asserting `__schema` output and the `ax.Error` envelope are byte-identical to the committed `testdata/` goldens — so it runs and asserts under every one of the four configurations (FR-012, SC-005)
- [X] T028 [P] [US3] Add an **untagged** CLI-contract parity test in `buildtags_parity_test.go` covering stream separation (`stdout` payload only), exit-code mapping, `--dry-run` side-effect suppression, and `--idempotency-key` surfacing, so each is asserted under every configuration (FR-012)
- [X] T029 [P] [US3] Add a `//go:build ax_no_grpc,!ax_no_otlp` compile-and-run assertion in `buildtags_single_test.go` confirming the export path is still reachable with the gRPC helper declined, and a `//go:build !ax_no_grpc,ax_no_otlp` assertion confirming `GRPCDial` is still present with export declined (FR-002, US3 acceptance 1–2)
- [X] T030 [P] [US3] Add a `//go:build ax_no_otlp` test asserting `AX_OTEL_DEBUG` local span output still works with export declined (FR-011)

### Implementation for User Story 3

- [X] T031 [US3] Verify all four configurations build and their test suites pass: `go build ./...`, `go build -tags=ax_no_grpc ./...`, `go build -tags=ax_no_otlp ./...`, `go build -tags=ax_no_grpc,ax_no_otlp ./...`, each followed by the matching `go test -race -tags=… ./...` (FR-002, FR-003)

**Checkpoint**: Four independently valid configurations; no coupling between the two knobs.

---

## Phase 6: User Story 4 - The reduced configurations cannot silently rot (Priority: P3)

**Goal**: A surface-inventory gate plus tagged lint/vet/test/crosscompile coverage, so a declined configuration cannot regress unnoticed.

**Independent Test**: Reintroduce a forbidden dependency, or add/remove an exported identifier in one configuration, and confirm CI fails naming the offending dependency or the identifier and its configuration.

### Tests for User Story 4 ⚠️

> **⚠️ T032 must land before T037** — plan.md § Critical sequencing constraints #3. Generating `baseline.json` against a vacuously-passing gate bakes in a wrong baseline.

- [X] T032 [US4] Write the fail-closed regression test in `internal/cmd/surfacecheck/main_test.go`: a deliberately bogus `GOOS` profile MUST exit `2`, asserting the gate rejects `packages.Load` returning `err == nil` with an empty slice, and that every requested import path came back with `p.Types != nil` and `len(p.Errors) == 0` (contracts C6, research.md D6)
- [X] T033 [P] [US4] Write the package-list consistency test in `internal/cmd/surfacecheck/main_test.go` asserting surfacecheck's public-package list agrees exactly with `allowedPackages()` in `internal/cmd/apidiff-verdict/main.go:67-76` (research.md § Open Risks #5)
- [X] T034 [P] [US4] Write table-driven baseline tests in `internal/cmd/surfacecheck/main_test.go`: exhaustive configuration lists normalise to `"all"` on write, output is sorted at every level so a regenerated baseline on an unchanged tree is byte-identical, drift messages name the symbol **and** its configuration and profile, and exit codes are `0` OK / `1` drift / `2` bad input (SC-009, SC-010, Principle II)
- [X] T035 [P] [US4] Write a negative test proving the dependency boundary bites: a fixture import list containing `google.golang.org/grpc/status` under the minimal configuration produces a violation naming the offending dependency (SC-008)

### Implementation for User Story 4

- [X] T036 [US4] Implement `internal/cmd/surfacecheck/main.go`: house-style `run(...) int` pure entry point, `-baseline <path>` and `-update` flags, the 4 tag combinations and 6 `GOOS/GOARCH` profiles as hardcoded Go constants, `packages.Load` with `Mode: NeedName|NeedTypes` and per-load `GOOS`/`GOARCH`/`CGO_ENABLED=0` env, the fail-closed assertion from T032, violations to `stderr` and the `PASS`/`FAIL N …` verdict to `stdout`, and an `io.LimitedReader` size cap on baseline reads (FR-018, FR-020, contracts C6)
- [X] T037 [US4] Implement the symbol walk in `internal/cmd/surfacecheck/main.go`: package-scope objects via `types.Scope.Names()` **plus** method sets (`types.NewMethodSet(types.NewPointer(named))`) and struct fields, rendered with `types.ObjectString(obj, types.RelativeTo(pkg))` and prefixed by kind (`func:`, `type:`, `const:`, `var:`, `method:`, `field:`) per data-model.md §3
- [X] T038 [US4] Implement the deterministic baseline writer in `internal/cmd/surfacecheck/main.go` — sorted at every level, `"all"` normalisation for universal symbols, per-profile recording of architecture-dependent constants without normalising them away, and the `version: 1` envelope (data-model.md §3)
- [X] T039 [US4] Generate and commit `internal/cmd/surfacecheck/baseline.json` via `go run ./internal/cmd/surfacecheck -update`; review the diff and confirm `ax.GRPCDial` is the **only** symbol carrying a non-`"all"` `configurations` value (FR-019, data-model.md §3 invariants)
- [X] T040 [US4] Add a `surface-check` target to `Makefile` running `go run ./internal/cmd/surfacecheck`, and add it to the `ci` target's prerequisite list (FR-020, SC-010)
- [X] T041 [US4] Add a `surfacecheck` step to the `validate` job in `.github/workflows/ci.yml`, alongside the existing `go run ./internal/cmd/doccover` step; do **not** place it in `crosscompile.yml`, whose job-level `GOOS`/`GOARCH` would fight the tool's own per-combination env (research.md D6 § CI placement)
- [X] T042 [US4] Add a `github.com/rshade/ax-go/internal/cmd/surfacecheck` floor entry to the `perPackage` map in `defaultFloorConfig()` in `internal/cmd/covercheck/main.go`, calibrated to the measured coverage (SC-012)
- [X] T043 [P] [US4] Add a `run:` section with `build-tags` support to `.golangci.yml` and extend the `lint` target in `Makefile` to run `golangci-lint run --build-tags=…` once per tag combination (four invocations — golangci-lint accepts only one tag set per run, research.md D7)
- [X] T044 [US4] Extend the `test` and `validate` targets in `Makefile` with tagged passes — `go test -race -tags=…` and `go vet -tags=…` for each of the four combinations (FR-017, FR-021, research.md D7)
- [X] T045 [US4] Add the tagged test and lint matrix to `.github/workflows/ci.yml` so all four configurations run in CI, not only the default (FR-021, SC-007)
- [X] T046 [US4] Add the four tag combinations to the build matrix in `.github/workflows/crosscompile.yml`, yielding 4 configurations × 6 profiles (FR-017, SC-007)

**Checkpoint**: The declined configurations are built, tested, linted, vetted, dependency-checked, and surface-diffed on every PR.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T047 [P] Document both build constraints in `README.md`: what each declines, the usage form, the −0.03% / −15.1% / −63.3% ladder making clear the size benefit needs **both**, that `ax_no_grpc` removes `ax.GRPCDial` and how to restore it, and that the thin `contract`/`config`/`schema`/`id` packages carry **no live tracing** (`contract.TraceIDFromContext` reads a stored context value, it does not resolve an active span) (FR-025, FR-026, contracts C5)
- [X] T048 [P] Mirror the same tag documentation and thin-package tracing caveat in `CONTEXT.md`
- [X] T049 [P] Document the tagged toolchain commands and the new `surface-check` gate in `AGENTS.md`, and add the `internal/cmd/surfacecheck` row to its coverage-floor table
- [X] T050 [P] Add a verified declined-configuration build of `examples/integration/` — extend the `build-example` target in `Makefile` (or add a sibling target) to build it with `-tags=ax_no_grpc,ax_no_otlp` and keep the example current with the new behaviour (FR-027)
- [X] T051 Run `gofmt -s -w .`, then the full gate suite in every configuration: `make ci`, `make surface-check`, `make cover-check`, `make bench-check`, plus `go test -race -tags=…` for all four combinations
- [X] T052 Run the `specs/016-optional-grpc-otlp/quickstart.md` validation end to end — contributor commands, gate invocations, baseline regeneration, and the measurement script — and correct any drift between the document and the shipped behaviour

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Setup — **blocks all user stories**
- **US1 (Phase 3)**: depends on Foundational; T003 specifically gates T018/T021
- **US2 (Phase 4)**: depends on US1 (it verifies US1 did not disturb the default build)
- **US3 (Phase 5)**: depends on US1 (the tags must exist to be selected independently)
- **US4 (Phase 6)**: the surfacecheck workstream (T032–T042) depends only on Setup and may run **in parallel with US1**; the toolchain-coverage tasks (T043–T046) depend on US1
- **Polish (Phase 7)**: depends on US1 and US4
- **ADR retirement**: not applicable — no governing ADR

### Critical sequencing constraints (plan.md)

1. **T003 before T018 and T021.** `executeTelemetryCommand` must move to an untagged file before `telemetry_export_test.go` is gated, or the declined build's test compilation breaks.
2. **Both tags ship together.** `ax_no_grpc` alone measures −0.03%; splitting the release would present the first half as a failed change.
3. **T032 before T039.** The fail-closed assertion must exist before `baseline.json` is generated, or a vacuously-passing gate bakes in a wrong baseline.

### Within Each User Story

- Tests are written and confirmed failing before implementation
- Seam files (`otlp.go`/`otlp_disabled.go`, `grpc.go`/`grpc_disabled.go`) before the test re-partitioning that depends on them
- Test re-partitioning before the four-configuration verification run

### Parallel Opportunities

- T002 runs alongside T001
- Within US1: T008, T009, T010 (three distinct new files) in parallel; T019 and T020 in parallel
- Within US3: T027–T030 in parallel
- Within US4: T033, T034, T035 in parallel; T043 in parallel with the surfacecheck implementation
- Within Polish: T047–T050 in parallel
- **Cross-story**: US4's surfacecheck workstream (T032–T042) proceeds concurrently with all of US1

---

## Parallel Example: User Story 1

```bash
# Tests first, three distinct new files:
Task: "Behaviour-parity assertions in telemetry_parity_test.go"
Task: "Declined-export diagnostic test in internal/telemetry/otlp_disabled_test.go"
Task: "Shutdown-budget test in internal/telemetry/otlp_disabled_test.go"

# Test re-partitioning, distinct files:
Task: "Gate telemetry_debug_otlp_test.go"
Task: "Gate telemetry_security_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup
2. Phase 2: Foundational — **T003 is the hazard; do it first**
3. Phase 3: User Story 1
4. **STOP and VALIDATE**: build and test all four configurations, run the measurement script, confirm ≥55% reduction and zero forbidden packages
5. The size win — the feature's actual value — is deliverable here

### Incremental Delivery

1. Setup + Foundational → seams safe to cut
2. US1 → measured slim binary (MVP)
3. US2 → proof the default build is untouched, no breaking release
4. US3 → all four configurations independently valid
5. US4 → the configurations are gated and cannot rot
6. Polish → documentation, example, full-suite verification

### Parallel Team Strategy

Two genuinely independent workstreams after Foundational:

- **Developer A**: US1 → US2 → US3 (the opt-out mechanism)
- **Developer B**: US4's surfacecheck gate (T032–T042), which needs only Setup

They converge at T043–T046 (toolchain coverage, needs the tags) and Polish.

---

## Notes

- `[P]` = different files, no dependencies
- Both tags are **negative**: absence of the tag is the full-featured state
- The declined build must never gain an exported symbol — `grpc_disabled.go` and `otlp_disabled.go` declare none
- Never assume a green default `go test` / `go vet` / `golangci-lint` run covers the declined configurations; pass tags explicitly
- Commit after each task or logical group; Conventional Commits (`feat:` — non-breaking, minor bump)
