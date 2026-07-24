---
description: "Task list for 017-import-isolated-logging"
---

# Tasks: Import-Isolated Logging Package

**Input**: Design documents from `/specs/017-import-isolated-logging/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/logging-package.md, contracts/logcore-package.md, quickstart.md

**Tests**: Test tasks ARE included and are mandatory — plan.md's Constitution Check marks Principle VII (Test-First Discipline) **PASS (binding)**, and FR-006/FR-007/FR-010/FR-014/FR-015 plus SC-001/SC-004/SC-006/SC-007 are stated as test assertions. Every test task lands before the implementation task it constrains and must fail for the right reason first.

**Organization**: Grouped by user story so each is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: `[US1]`–`[US3]` map to spec.md user stories
- Exact file paths included in every task

## Path Conventions

Go library at the module root (`github.com/rshade/ax-go`). Public package `ax` lives at `/`; the new public package `logging` and private mechanics under `internal/logcore` follow the `mcp` over `internal/mcpserver` precedent. No `pkg/` or `src/`.

**Governing ADR**: none (plan.md § ADR governance finding — ADR-0004 and ADR-0008 are touched but not governing). **No ADR-retirement task is generated.**

**Build configurations**: per AGENTS.md, a green default run covers none of the declined configurations. Every verification task that says "all four configurations" means `""`, `ax_no_grpc`, `ax_no_otlp`, and `ax_no_grpc,ax_no_otlp`.

**Task numbering**: T001–T059 are the original generated set. T060–T062 were added by the `/speckit-analyze` remediation pass and are placed in the phase where they execute, not at the end — the ID is an identifier, the phase and the Dependencies section are the order. No original ID was renumbered, so references elsewhere stay valid.

---

## Phase 0: Governance (executes before Phase 1)

**Purpose**: Reconcile the supreme governance document before writing code against it. Constitution conflicts are resolved by amendment, never by proceeding and reinterpreting.

- [x] T060 Amend `.specify/memory/constitution.md` Principle VIII so its named-constructor clause admits an identity-preserving alias exposed by an import-isolated surface, while making explicit that a second implementation or backend remains forbidden by Principle VI. Follow the constitution's own amendment procedure: edit the principle, prepend a Sync Impact Report block, apply the correct semantic bump (`1.2.0 → 1.2.1`, PATCH — a clarification, since no principle is added, removed, or redefined), review the plan/spec/tasks templates (no change required — none encodes a logging-constructor contract), and record why AGENTS.md's "canonical constructor" sentence stays true and unchanged. **Completed during the `/speckit-analyze` remediation pass**; verify the diff before merge

**Checkpoint**: The constitution sanctions the surface this feature is about to build. Plan.md's Constitution Check § Principle VIII named-constructor clause records the reasoning.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the two new package roots so test files have something to compile against.

- [x] T001 Create `internal/logcore/doc.go` declaring `package logcore` with a package doc comment stating it is the zerolog-backed logger implementation, that it is not a public surface, and that the `internal/` path restriction is what carries Principle VI's no-pluggable-backend guardrail once `Sink` must be exported (plan.md § Guardrail note, research.md R4)
- [x] T002 [P] Create `logging/doc.go` declaring `package logging` with a package doc comment stating it is the import-isolated public logging surface, naming what its dependency graph excludes (root `ax`, `internal/telemetry`, OTel SDK/exporters, gRPC, protobuf, Cobra, `net/http`, `crypto/tls`), and directing readers needing Loki direct push or `Execute` to root `ax` (contracts/logging-package.md § Import isolation contract, quickstart.md § Choosing a surface)

**Checkpoint**: Both packages exist and compile empty; `go build ./...` is green.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Extract the logger into `internal/logcore`, decouple Loki from the core logger (FR-010), and re-point root `ax` onto the extracted types via aliases. This phase is blocking for every story: the public `logging` package has nothing to wrap until `logcore` exists, and `logcore` cannot satisfy C-13 ("no Loki-specific identifier") until `lokiWriter` implements the exported `Drain`/`SanctionLabels` methods.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Tests for Foundational (write first, verify they fail for the right reason)

- [x] T003 [P] Write `internal/logcore/logcore_test.go` — table-driven tests for C-01 (every `Option` applied before any sink is sanctioned, so option order is irrelevant), C-02 (each `AdditionalSinks` entry satisfying `LabelSanctioner` is sanctioned; an entry that does **not** implement it is left alone and never rejected — this is the FR-010 generic-seam regression guard, using one fake sink implementing only `Sink` and a second implementing both), C-03 (`io.MultiWriter` fan-out when sinks are present), C-07 (`applyLabels` omits empty fields entirely rather than emitting empty strings), and C-08 (`WithLabels` re-sanctions on carried sinks and returns a derived logger)
- [x] T004 [P] Write `internal/logcore/sink_test.go` — tests for C-09 (`Flush` drains each sink in order and joins errors with `errors.Join`, asserted with `errors.Is` against each sink's error), C-10 (`Flush` returns `nil` for a nil logger and for a logger not satisfying `flusher`), and C-11 (no `panic` on any drain path including a sink returning a non-nil error and a cancelled context)
- [x] T005 [P] Write `internal/logcore/tracing_test.go` — tests for C-04 (`tracingHook` stamps correct `trace_id`/`span_id` on every enabled event under an active span context), C-05 (with no active span both fields carry `contract.ZeroTraceID`/`contract.ZeroSpanID`, the valid zero-value hex strings ADR-0004 requires so consumer parsers never branch on absence — research.md R8), and C-06 (a level-filtered event never constructs, so the hook never runs)
- [x] T006 [P] Write `internal/logcore/loki_free_test.go` — a source-scanning test asserting C-13: no file in `internal/logcore` contains a Loki-specific identifier (`loki`, `Loki`, `lokiWriter`, `labelPair`, `AX_LOKI_URL`), which is the standing assertion that FR-010's decoupling is not silently reintroduced
- [x] T007 [P] Write `internal/logcore/logcore_bench_test.go` — `testing.B` benchmarks with `-benchmem` mirroring the shapes of the root package's existing `BenchmarkLogger*` set and asserting the C-05 allocation contract that the no-active-span emission path allocates nothing. Two naming rules, both load-bearing: (a) do **not** rename, move, or delete the existing root `logger_bench_test.go` benchmarks — a benchmark present in `BENCH_BASE_REF` but absent from the current run is a hard `benchcheck` failure (AGENTS.md § Performance Regression Budget); (b) give the new benchmarks **distinct names** (`BenchmarkLogcoreEmit`, `BenchmarkLogcoreTracingHook`, …), never a duplicate of a root name, because `benchcheck`'s missing-benchmark detection keys on the benchmark name alone and ignores the package group (`internal/cmd/benchcheck/main.go`, `missingBenchmarks`) — two same-named rows in the current run against one in the baseline is an ambiguity whose failure mode hides a regression. SC-006's tracked hot path stays covered by root's unchanged `BenchmarkLogger*`, which after the extraction exercises the relocated code through the delegation (research.md R13)
- [x] T008 [P] Write `internal/logcore/concurrency_test.go` — a `-race` test running multiple goroutines emitting through one logger while another goroutine calls `Flush`, covering L-09/C-09 concurrency safety
- [x] T009 [P] Write the new cases in `internal/testutil/imports_test.go` for a logging-surface rule set: assert that `github.com/rs/zerolog` and `go.opentelemetry.io/otel/trace` are **not** flagged (they are required by the logging surface, unlike the four contract packages), and that root `ax`, `internal/telemetry`, `internal/mcpserver`, `go.opentelemetry.io/otel/sdk`, the OTel exporters prefix, the OTel contrib instrumentation paths, `google.golang.org/grpc`, `google.golang.org/protobuf`, `github.com/spf13/cobra`, `net/http`, and `crypto/tls` all are (contracts/logging-package.md)

### Implementation for Foundational

- [x] T010 Implement `internal/logcore/logcore.go` — move `Labels` and the `labelField*` name constants, the `Logger` interface, `Config` (renamed from `loggerConfig`, with exported `Ctx`/`Writer`/`Level`/`Labels`/`AdditionalSinks` fields and the `//nolint:containedctx` suppression carried onto `Ctx`), `Option` (renamed from `LoggerOption`), `WithWriter`/`WithLevel`/`WithLabels`, `New`, and the unexported `zerologLogger` with its `Debug`/`Info`/`Warn`/`Error`/`WithLabels`/`Zerolog` methods, preserving the data-model.md § Construction sequence order exactly (apply options → sanction labels → `io.MultiWriter` → build zerolog at level → apply labels → attach hook)
- [x] T011 Implement `internal/logcore/sink.go` — the exported `Sink` (`io.Writer` + `Drain(ctx) error`) and `LabelSanctioner` (`SanctionLabels(Labels)`) interfaces with doc comments explaining why each is exported (research.md R3: an unexported method name is qualified by its defining package and is therefore unsatisfiable across a package boundary), the unexported `flusher` interface, `zerologLogger.flush`, and the package-level `Flush(ctx, Logger) error`
- [x] T012 Implement `internal/logcore/tracing.go` — move `tracingHook` and `applyLabels`, plus an unexported `traceIDs` helper reading `trace.SpanContextFromContext` and falling back to `contract.ZeroTraceID`/`contract.ZeroSpanID`; import only the OTel trace **API**, never the SDK, and export no symbol beyond the closed set in contracts/logcore-package.md. Relocate `tracingHook`'s allocation-contract doc comment with **one** correction: drop the citation to "ADR-0009", which names a file that does not exist in `docs/adr/` (a stale reference left by an earlier retirement, `logger.go:213`). Attribute the allocation contract to this feature's `research.md` R8 instead. Copying a known-false reference verbatim into a new file is more expensive than deleting it, and no ADR decision is being altered — only a citation that was already wrong
- [x] T013 Extend `internal/testutil/imports.go` with a per-surface rule set — add `ForbiddenLoggingImports()` returning the contracts/logging-package.md table (including the `net/http` and `crypto/tls` entries absent from the contract-package list) and an `AssertLoggingSurfaceIsolated` helper, leaving `ForbiddenRuntimeImports()` and `ForbiddenGRPCTreeImports()` unchanged so the four existing contract packages keep forbidding `zerolog` outright
- [x] T014 Rewrite `logger.go` in root `ax` to delegate **directly onto `internal/logcore`** — `type Logger = logcore.Logger`, `type Labels = logcore.Labels`, `type LoggerOption = logcore.Option` as identity-preserving **alias** declarations (`=`, never redeclarations), and `NewLogger`/`WithLoggerWriter`/`WithLoggerLevel`/`WithLoggerLabels`/`Flush` as thin **functions** calling their `logcore` counterparts (never `var`, which `go-apidiff` classifies as breaking — research.md R7), preserving every existing doc comment's documented behavior verbatim; no `lokiWriter` type assertion may remain in this file. **Root `ax` MUST NOT import the public `logging` package**: the two public surfaces are siblings over `logcore`, never a chain. Identity holds either way, but root importing `logging` makes T036's parity test — which imports `ax` — an import cycle (research.md R7, data-model.md § Alias graph)
- [x] T015 Update `loki.go` in root `ax` — rename `drain` → `Drain` and `sanctionLabels` → `SanctionLabels` so `*lokiWriter` satisfies `logcore.Sink` and `logcore.LabelSanctioner`, and re-point the `WithLokiFromEnv` option body at `logcore.Config`'s exported fields, **preserving the `errorWriter: &cfg.Writer` address capture** that makes a later `WithLoggerWriter` observable to the Loki error path and keeps option order irrelevant (research.md R5). Nothing else moves: the push URL, auth token, goroutine lifecycle, retry/fail-open diagnostics, `labelPair`, and the five-label cardinality allowlist all stay
- [x] T016 Reconcile `trace.go` in root `ax` — keep `TraceIDFromContext`, `SpanIDFromContext`, the unexported `traceIDs`, and `withTraceMetadata` in place for `contract.Metadata` population; do **not** export a `logcore.TraceIDs` to deduplicate, because contracts/logcore-package.md defines a closed export set. Record in a comment that root's `traceIDs` and `logcore`'s must stay behaviorally identical, which the T036 parity test enforces
- [x] T017 Sweep the root-package test files for references to the moved unexported identifiers, and keep the sweep **evidence-based**. Establish the real set first — `grep -n 'loggerConfig\|logSink\|flusher\|applyLabels\|tracingHook\|sanctionLabels\|\.drain(' *_test.go` — because the set is much smaller than it looks: at the time of writing, `logger_test.go` and `loki_test.go` contain **zero** such references, and `logger_bench_test.go`'s four `tracingHook` hits are all in comments. The genuine production call sites are `logger.go:115,162,184` (T014) and `loki.go:296` (T015). Make **only** mechanical renames in whatever the grep actually returns, and update stale comment references so they name the new home; a substantive assertion change here would forfeit SC-005
- [x] T018 Run `make surface-update` and review every line of `git diff internal/cmd/surfacecheck/baseline.json` for the root-package alias drift (`ax.Logger`, `ax.Labels`, and `ax.LoggerOption` now render as aliases, and `LoggerOption` resolves to `func(*logcore.Config)` — the known cosmetic consequence in data-model.md § Option). Confirm no root feature is `added` or `removed` and that `specs/015-internalize-helpers/public-surface-audit.json` needs no new record, since the audit joins on bare feature IDs and no ID changed
- [x] T019 Verify the checkpoint across all four build configurations: `gofmt -s -l .`, `go build`, `go vet`, `go test -race`, `golangci-lint run --build-tags=...` for each of `""`, `ax_no_grpc`, `ax_no_otlp`, `ax_no_grpc,ax_no_otlp` (or `make test`, `make validate`, `make lint`, which iterate `BUILD_TAG_MATRIX`), plus `make cover-check`, `make doc-coverage`, and `make surface-check`

**Checkpoint**: One logger implementation exists, `logcore` is Loki-free (FR-010 satisfied, plan.md's Principle VIII violation remediated), root `ax` compiles and behaves identically, and every existing gate is green. User story work can now begin.

---

## Phase 3: User Story 1 - Small binary for a distributed CLI (Priority: P1) 🎯 MVP

**Goal**: A consumer depending only on `github.com/rshade/ax-go/logging` gets a fully trace-correlated logger in a stripped 64-bit Linux binary under 3 MB, against the measured 12,017,929-byte root-facade baseline.

**Independent Test**: Build `examples/logging` (which imports only the logging surface) with `-trimpath -ldflags="-s -w"`, confirm `go list -deps` contains none of the forbidden trees, confirm the emitted line carries the correct `trace_id`/`span_id` on stderr with stdout untouched, and measure the binary under 3 MB.

### Tests for User Story 1 (write first, verify they fail for the right reason)

- [x] T020 [P] [US1] Write `logging/import_isolation_test.go` — assert `AssertLoggingSurfaceIsolated` against `github.com/rshade/ax-go/logging` **and** `github.com/rshade/ax-go/examples/logging` under all four build configurations by passing each tag set to `testutil.ResolvePackageImports`, satisfying FR-014's requirement that the isolation guarantee be verified rather than assumed; add a positive assertion that `github.com/rs/zerolog` and `go.opentelemetry.io/otel/trace` **are** present, so a future refactor cannot pass the test by accidentally dropping trace correlation
- [x] T021 [P] [US1] Write `logging/logging_test.go` — table-driven tests for the contracts/logging-package.md behavioral IDs reachable from the isolated surface: L-01 (`NewLogger` never returns nil), L-02 (default writer stderr, default level info), L-03 (all output on the diagnostic stream, payload stream never written), L-04/L-05 (correct IDs under an active span; zero-value hex constants and no allocation without one), L-06 (option order independence), L-07 (empty `Labels` fields omitted), L-08 (`WithLabels` returns a derived logger), and L-11 (`Flush(ctx, nil)` returns `nil`). Add the L-12 **documentation** assertion FR-012 now requires: read `logging/logging.go` and assert `Flush`'s doc comment states plainly that it performs no work for consumers of `logging` alone. `godoclint`'s `require-doc` gates that a comment exists, never that it says the true thing, so a promise carried only by prose is unverified; root's `documentation_test.go` is the in-repo precedent for asserting documentation content by reading the file
- [x] T022 [P] [US1] Write `logging/example_test.go` with `ExampleNewLogger` — a verified example declaring `// Output:` that constructs a logger with `WithLoggerWriter` (to `os.Stdout` so the output is assertable), `WithLoggerLevel`, and `WithLoggerLabels`, demonstrating the `WithX` options inside the parent example rather than as separately gated examples (contracts/logging-package.md § Documentation contract)
- [x] T023 [P] [US1] Write `internal/cmd/sizecheck/main_test.go` — table-driven tests distinguishing **three** outcomes, not two: a build failure, an absolute **ceiling breach** (SC-001), and a **reduction-ratio breach** (SC-002), each with its own message (research.md R10). Cover the pass path, both failure paths, invalid flags, and the stream/exit contract used by the sibling gates: a pass writes one minified JSON object to stdout — including both measured sizes and the computed reduction — and nothing to stderr with exit `0`; a failure writes nothing to stdout and exactly one minified `ax.Error` envelope to stderr
- [x] T024 [P] [US1] Extend `internal/cmd/doccover/main_test.go` for **package-qualified** required symbols. The decisive case, which is the whole point of the change: with root's `ExampleNewLogger` present and `logging`'s absent, the gate must **fail**. Today `requiredSymbols()` holds bare names (`"NewLogger"`, `"Logger"`, `"Flush"`) checked against one flat scanned set, so merely scanning a second directory and unioning the results lets root's example satisfy the new surface's requirement — the contract would be stated but unenforced, and the gate would be green, which is worse than red (research.md R12). Also cover: qualified `baseline.txt` entries ratcheting as before, an unqualified legacy line being rejected or migrated deterministically, and a symbol required in two packages needing an example in each
- [x] T025 [P] [US1] Write `examples/logging/main_test.go` — assert the example command writes its log line to stderr with stdout empty and carries the expected labels, satisfying the 25% default per-package coverage floor the new package faces

### Implementation for User Story 1

- [x] T026 [US1] Implement `logging/logging.go` — the exact exported set from contracts/logging-package.md and nothing more: `Logger`/`Labels`/`LoggerOption` as identity-preserving aliases over `logcore`, and `NewLogger`/`WithLoggerWriter`/`WithLoggerLevel`/`WithLoggerLabels`/`Flush` as thin functions. `Sink`, `LabelSanctioner`, and `Config` are **not** re-exported — that omission is what makes external backend registration impossible (FR-011). Every symbol carries a contract-style doc comment, and `Flush`'s states L-12 plainly: for a consumer of `logging` alone it performs no work and returns `nil`, because the only buffering destination lives in root `ax` and is unreachable here (FR-012)
- [x] T027 [US1] Implement `examples/logging/main.go` — a minimal command importing **only** `github.com/rshade/ax-go/logging` (plus stdlib) that constructs a logger and emits one correlated line, doubling as the second example FR-017 requires and as the artifact `sizecheck` measures
- [x] T061 [US1] Implement `examples/rootlogging/main.go` and `examples/rootlogging/main_test.go` — the root-facade counterpart: **byte-for-byte the same program** as `examples/logging` with `logging.NewLogger` swapped for `ax.NewLogger`, so the only variable between the two measured binaries is the surface imported. This is the denominator of the SC-002 reduction ratio, committed rather than synthesised into a temp module because an in-module program builds against the repository's own `go.mod` — no network, no `replace` stanza, and the comparison stays reviewable in `git diff` (research.md R10). Its test mirrors T025's: log line on stderr, stdout empty. Keep the two `main.go` files diff-clean against each other; a reviewer should see one changed import and one changed call
- [x] T028 [US1] Implement `internal/cmd/sizecheck/main.go` — build both probes with `-trimpath -ldflags="-s -w"` into a temp directory, `stat` each, and enforce **two** hardcoded constants (following the `covercheck`/`benchcheck`/`surfacecheck` precedent so every change is a reviewable commit auditable via `git blame`): (a) an absolute ceiling for `examples/logging` at 3,000,000 bytes per SC-001, with headroom documented against the ~2,250,000-byte measurement; and (b) a minimum reduction of 75% for `1 − size(examples/logging) / size(examples/rootlogging)` per SC-002. Report three outcomes distinctly — build failure, ceiling breach, ratio breach. Document in the constant's doc comment why the two are adjusted under different rules: the ceiling drifts with the toolchain and may be raised for a reviewed reason, while the ratio is toolchain-independent (both probes move together), so a ratio breach always means the isolated surface gained weight the root facade did not, and lowering it is a spec change rather than a calibration (quickstart.md § Adjusting the size gate)
- [x] T029 [US1] Extend `internal/cmd/doccover/main.go` to make required symbols **package-qualified** — `requiredSymbols()` returns `ax.NewLogger`, `logging.NewLogger`, … instead of bare names; `scanPackage` is called per package and its results stay keyed by package rather than unioned; and `baseline.txt` lines carry the same qualification, with existing lines migrated by prefixing `ax.`. Scanning a second directory without qualifying is the trap: root already has `ExampleNewLogger`, so a union would satisfy the new surface's requirement for free and the gate would pass while enforcing nothing (research.md R12). Ratchet semantics are unchanged — the gate stays one-way and the baseline still burns down to empty; only the key gains a package prefix
- [x] T030 [US1] Add the `size-check` target to `Makefile` (running `go run ./internal/cmd/sizecheck`), append it to the `ci` target's dependency list, and add its `help` line alongside `surface-check` and `bench-check`
- [x] T031 [US1] Add a size-gate step to `.github/workflows/ci.yml` running `make size-check`, following the `surface-check` job's shape; note in the step comment that ceilings are pinned to linux/amd64 with symbol stripping and vary by toolchain version
- [x] T032 [US1] Add `rootImportPath + "/logging"` to `PublicPackages()` in `internal/cmd/surfacecheck/inventory.go`, keeping the list sorted, and update the doc comment at `internal/cmd/surfacecheck/main.go:14` from "24 loads of the **six** public packages" to "**seven**". The load **count does not change**: a load is one (configuration, profile) combination and `scanCombination` loads every requested package within it, so 4 × 6 = 24 regardless of package count. Do not write "28" — an earlier draft of research.md R9 claimed the matrix grew to 28, which was wrong and is corrected there (research.md R9)
- [x] T033 [US1] Add `"github.com/rshade/ax-go/logging"` to `allowedPackages()` in `internal/cmd/apidiff-verdict/main.go` in the same change as T032 — the two allowlists are duplicated by design and guarded by a test that parses one and compares, so updating only one fails the `check-packages` guard in `.github/workflows/apidiff.yml`
- [x] T034 [US1] Run `make surface-update` and review every line of `git diff internal/cmd/surfacecheck/baseline.json`. Expect **two** groups of new entries, not one: the `logging` package's own features, and `logcore.Config` with its fields — reachable from the gated surface because `logging.LoggerOption` aliases `logcore.Option`, whose signature names `*logcore.Config`, and `surfacecheck` inventories reachable hidden concrete types. Both groups are correct and expected; record in the commit message the durable consequence that `logcore.Config`'s field set is now reviewed public-surface drift despite living under `internal/` (data-model.md § Option). Confirm every new entry's `configurations` and `profiles` presence sets are the `"all"` sentinel — `logging` links none of the trees the build constraints decline, so its presence is configuration-independent (FR-014) — and treat anything else as unreviewed drift rather than a baseline to regenerate blindly
- [x] T035 [US1] Validate the story end to end per quickstart.md: run the `go list -deps` isolation greps by hand (expect zero forbidden lines, and `zerolog`/`otel/trace` present), run the size reproduction block — which builds `./examples/logging` and `./examples/rootlogging` in-module and computes the ratio with `awk`, no synthesised module involved — and record the measured bytes and reduction percentage against SC-001 (< 3 MB) and SC-002 (≥ 75%, expected 81%). Confirm the numbers agree with what `make size-check` reports; a disagreement means the gate and the documented procedure measure different things

**Checkpoint**: User Story 1 is fully functional and independently testable — the isolated surface exists, is proven isolated in all four configurations, and its size is gated.

---

## Phase 4: User Story 2 - Existing adopters notice nothing (Priority: P2)

**Goal**: Code built on root `ax` compiles untouched, behaves identically, emits byte-identical log lines, and passes `go-apidiff` with no breaking change.

**Independent Test**: Compile and run the existing root-package suite and the integration example unchanged, diff emitted log lines against pre-change output, and confirm the public API comparison reports no breaking change.

### Tests for User Story 2 (write first, verify they fail for the right reason)

- [x] T036 [P] [US2] Write `logging/parity_test.go` in the **external test package `logging_test`** and in an **untagged** file, asserting FR-006/SC-004: with identical writers, level, and labels, a line emitted through `ax.NewLogger` and one through `logging.NewLogger` are byte-identical, including under an active span where `trace_id`/`span_id` must match, covering L-10. Both constraints are deliberate. Untagged means it runs under every build configuration — a parity claim verified only by the default build proves nothing (AGENTS.md). External package means the file's import of root `ax` can never contribute to `logging`'s own dependency graph, keeping T020's isolation assertion unambiguous; it matches T037's placement for the same reason. This is safe only because root `ax` delegates to `logcore` rather than to `logging` (T014) — under the chain reading, this file would be an import cycle
- [x] T037 [P] [US2] Write `logging/identity_test.go` in external test package `logging_test` asserting the contracts/logging-package.md § Cross-surface identity contract compiles and runs: `var a ax.Logger = logging.NewLogger(ctx)`, `var b logging.Logger = ax.NewLogger(ctx)`, `ax.Flush(ctx, logging.NewLogger(ctx))`, `logging.Flush(ctx, ax.NewLogger(ctx))`, and the sharpest case `logging.NewLogger(ctx, ax.WithLokiFromEnv())` — an option manufactured by root `ax` accepted by the isolated constructor, proving the alias chain is unbroken (FR-005). An external test package keeps root `ax` out of `logging`'s non-test dependency graph, so T020 is unaffected
- [x] T038 [P] [US2] Add a regression assertion that `ax.NewLogger` and `ax.Flush` remain **functions** rather than variables, guarding the func→var conversion `go-apidiff` classifies as breaking (research.md R7); a compile-time `var _ func(context.Context, ax.Logger) error = ax.Flush` in an existing root test file is sufficient

### Implementation / verification for User Story 2

- [x] T039 [US2] Confirm the root-package suite passes with only the T017 mechanical renames — run `go test -race ./...` and diff `logger_test.go`, `logger_bench_test.go`, `golden_test.go`, and `documentation_test.go` against their pre-change versions to prove no assertion was weakened
- [x] T040 [US2] Run the public API verdict locally against the base branch (`go-apidiff` plus `go run ./internal/cmd/apidiff-verdict`, and `go run ./internal/cmd/apidiff-verdict check-packages`) and confirm it reports no breaking change over the seven-package public surface, satisfying SC-003; if it reports one, the cause is almost certainly a redeclared rather than aliased type or a func→var conversion (quickstart.md § Troubleshooting)
- [x] T041 [US2] Confirm `examples/integration` compiles and passes unchanged in both `make build-example` and `make build-example-minimal`, proving the root path — including `WithLokiFromEnv` and `ax.Flush` — needs zero source changes (FR-017 first clause)

**Checkpoint**: Both surfaces work, and the change is provably non-breaking for existing adopters.

---

## Phase 5: User Story 3 - Log shipping is untouched (Priority: P3)

**Goal**: Loki direct push activates, labels, drains, and fails exactly as before.

**Independent Test**: Run the existing log-shipping suite with no substantive modification (SC-005).

**Note on independence**: this story verifies the code T015 modifies, so it runs after Foundational. Its assertions are preservation guarantees, not new capability.

### Tests for User Story 3

- [x] T042 [P] [US3] Verify `loki_test.go` passes with a diff containing **only** the `drain` → `Drain` and `sanctionLabels` → `SanctionLabels` renames and the `logcore.Config` field references — review the diff explicitly and record that no assertion changed, which is the whole of SC-005
- [x] T043 [P] [US3] Add a table-driven test in `loki_test.go` covering FR-009 acceptance 3: `WithLokiFromEnv` applied **before** and **after** `WithLoggerLabels` yields identical stream-label promotion, exercised through both `ax.NewLogger` and `logging.NewLogger` (the latter accepting the root-manufactured option via the T037 alias chain), which is the regression guard for the `&cfg.Writer` address capture T015 preserves
- [x] T044 [P] [US3] Assert the cardinality split still holds after the `LabelSanctioner` indirection replaced the `*lokiWriter` type assertion: only the sanctioned low-cardinality pairs are promoted to stream labels, and a payload field reusing a label key name stays payload-only (Constitution Principle VIII, FR-009 acceptance 2)
- [x] T045 [P] [US3] Assert drain semantics are unchanged — buffered entries are delivered within the documented `lokiFlushTimeout` deadline, `Flush` on a nil logger returns `nil`, and a drain failure never changes the process exit code (FR-009 acceptance 4, spec.md Edge Cases)
- [x] T046 [US3] Assert the derived-logger edge case end to end: a logger derived via `WithLabels` carries its Loki sink forward so a later `ax.Flush` still drains buffered entries, and the derived labels are re-sanctioned (C-08, spec.md Edge Cases)

**Checkpoint**: All three user stories are independently verified; log shipping is provably untouched.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, gate calibration, and release hygiene (FR-013, FR-016, FR-017, SC-008).

- [x] T047 [P] Update `README.md` — describe the `logging` surface, add the quickstart.md § Choosing a surface table, show the minimal consumer snippet, state the measured size reduction, and refresh the `## Compatibility` matrix if the public-package count is stated there. Then extend root `documentation_test.go`, which already asserts the README names each public import path and its guidance phrases: add `"github.com/rshade/ax-go/logging"` and the choosing-a-surface guidance to `TestDocumentationExplainsPublicImportChoices`, so this documentation stays verified rather than drifting silently
- [x] T048 [P] Update `AGENTS.md` — add `logging` to the approved public packages paragraph and the repository layout (noting it sits over `internal/logcore` exactly as `mcp` sits over `internal/mcpserver`, and that it **is** an import-isolated surface unlike `mcp`); add a **Binary Size Gate** section for `internal/cmd/sizecheck` mirroring the surface/coverage/benchmark gate sections, documenting **both** constants and why the ceiling and the reduction ratio are adjusted under different rules; update the surface-gate sentence from "six public packages" to "seven" while **leaving the load count at 24** (loads are configuration × profile combinations and do not scale with package count — see T032); update the Coverage Policy table with the T052 floors; and reconcile the Accepted Architecture sentence "The canonical constructor is `ax.NewLogger(ctx)`" — `ax.NewLogger` stays canonical, and the sentence gains that `logging.NewLogger` is an identity-preserving alias of the same constructor, per constitution `1.2.1` (T060). Do this only here, in the same change that makes the package real, so no derived doc ever describes unshipped code
- [x] T049 [P] Update `CONTEXT.md` with the new package boundary and the two-surface story
- [x] T050 [P] Update `ROADMAP.md` — mark issue #144 shipped with a link to `specs/017-import-isolated-logging/`, and note that the "never couple a log-shipping backend into the core logger" line is now enforced by `logcore`'s C-13 assertion rather than by convention alone
- [x] T051 [P] Add a `logging`-surface page to the `docs/` Starlight site under the appropriate Diátaxis quadrant (a how-to guide for choosing a surface plus an explanation of the isolation boundary), and run markdownlint on it
- [x] T052 Add per-package coverage floors for `github.com/rshade/ax-go/internal/logcore`, `github.com/rshade/ax-go/logging`, `github.com/rshade/ax-go/internal/cmd/sizecheck`, `github.com/rshade/ax-go/examples/logging`, and `github.com/rshade/ax-go/examples/rootlogging` to `defaultFloorConfig()` in `internal/cmd/covercheck/main.go`, each calibrated ~2pp below its measured coverage per the established convention, and record the rationale for the commit message. **Also re-check the root package's existing 80% floor**: this feature moves a well-tested implementation out of `github.com/rshade/ax-go` into `logcore`, which changes root's covered-statement ratio in a direction nobody has measured. T019 will surface a breach via `make cover-check`, but no task currently authorises the response — so if root's measured coverage moves materially, adjust the `"github.com/rshade/ax-go"` floor here, in the same reviewed commit, with the reason recorded. Floors are policy; moving one silently to make a gate green is the failure mode this note exists to prevent
- [x] T053 [P] Update `examples/integration/README.md` and `examples/integration/AUDIT.md` to point at `examples/logging` as the isolated-surface counterpart, keeping the integration example's own root-path coverage (Loki + drain) described as-is (FR-017)
- [x] T054 [P] Run markdownlint across every changed Markdown file (`README.md`, `AGENTS.md`, `CONTEXT.md`, `ROADMAP.md`, `docs/`, `examples/`, and this spec directory) and fix all findings
- [x] T055 **[UNVERIFIABLE LOCALLY — see note below]** Run `make bench-check` against `BENCH_BASE_REF` and confirm the tracked benchmarks stay within the 5% `ns/op` and +1 `allocs/op` budget, specifically that the relocated hot path did not regress `BenchmarkLogger*` and that no previously-tracked benchmark is reported `Missing` (SC-006). Additionally verify the T007 naming rule held in practice: `go test -run='^$' -bench=. -cpu=1 ./... | grep -c '^BenchmarkLogcore'` should be non-zero while no benchmark name appears in two packages — `benchcheck` keys `Missing` on the bare name, so a duplicate would make the comparison ambiguous rather than loud
- [x] T056 Execute `specs/017-import-isolated-logging/quickstart.md` end to end as written — the full gate set, the four-configuration loop, the isolation greps, the size reproduction, and the baseline-update walkthrough — and correct any command that does not work verbatim
- [x] T057 Run the complete CI gate set one final time across all four build configurations: `make test`, `make validate`, `make lint`, `make doc-coverage`, `make cover-check`, `make surface-check`, `make size-check`, `make bench-check` (SC-007, SC-008). **Pass condition, stated explicitly**: every gate green except — permissibly — the inherited `actionlint` SC2086 failure that T062 tracks. Any other failure blocks. Without this carve-out "the complete gate set passes" has no defined meaning on a branch that inherits a known-red check
- [x] T062 **[ALREADY FIXED UPSTREAM — verified, no action needed]** Resolve or explicitly accept the inherited lint failure in `.github/workflows/crosscompile.yml` (SC2086 at lines 73, 81, 87), which arrives from PR #150 and is not caused by this feature (research.md R9, checklists/requirements.md). The naive fix is wrong: plain quoting passes an empty string as an argument when the variable is unset. Use conditional expansion (`${VAR:+-tags=$VAR}`) or build an argument array and expand it as `"${args[@]}"`, so an unset variable contributes **no** argument. If the fix belongs upstream on PR #150 rather than here, record that decision and the tracking link in the PR message so T057's carve-out has a documented owner rather than an open-ended exception
- [x] T058 [P] Confirm no stale ADR reference rode along with the move — `grep -rn 'ADR-0' internal/logcore/ logger.go loki.go trace.go` — expecting zero hits for files that no longer exist in `docs/adr/` (only ADR-0004 and ADR-0008 remain). T012 corrects the known `ADR-0009` citation in place; this task verifies it was the only one, which is cheaper than filing a follow-up issue for a citation nobody can act on later
- [x] T059 Write `PR_MESSAGE.md` as a Conventional Commit (`feat(logging): ...`) describing the new import-isolated surface, the measured size reduction, the Principle VIII remediation, and the two allowlist updates; validate with `cat PR_MESSAGE.md | npx commitlint`. Do **not** hand-edit `CHANGELOG.md` — release-please owns it

---

## Dependencies & Execution Order

### Phase Dependencies

- **Governance (Phase 0)**: no dependencies — **must land before any code**, since it is what makes the surface being built constitutional (T060)
- **Setup (Phase 1)**: depends on Governance
- **Foundational (Phase 2)**: depends on Setup; **blocks all user stories**. `logcore` must exist before `logging` can wrap it, and Loki must be decoupled before `logcore` can satisfy C-13
- **US1 (Phase 3)**: depends on Foundational only — the MVP
- **US2 (Phase 4)**: depends on Foundational; its parity and identity tests additionally need `logging` from US1 (T026)
- **US3 (Phase 5)**: depends on Foundational (specifically T015, the `loki.go` method renames); T043 additionally needs `logging` from US1
- **Polish (Phase 6)**: depends on all desired stories. No ADR-retirement task exists — plan.md establishes this feature is governed by no ADR

### Within Each Phase

- Every test task precedes the implementation task it constrains and must be observed failing for the right reason first (Principle VII, binding)
- T010 → T011 → T012 are ordered by type dependency (`Config`/`Logger` before `Sink`/`Flush` before the hook)
- T014 depends on T010–T012; T015 depends on T014; T017 depends on T014 and T015
- T018 must follow T017 (the surface renders only once the aliases compile)
- T032 and T033 must land in the **same change** or the `check-packages` guard fails CI
- T034 must follow T026, T032, and T033
- T061 depends on nothing in `logging` (it imports root `ax`), but T028 depends on **both** T027 and T061 — the ratio needs both probes to exist
- T029 must follow T022 and T024; T035 must follow T028 and T061
- T052 must follow T019 (root's coverage cannot be re-checked until the extraction compiles) and every test task whose coverage it calibrates against

### Parallel Opportunities

- T001 and T002 are independent files
- T003–T009 are seven independent test files and can be written concurrently
- T020–T025 are six independent test files across four packages
- T036–T038 are independent
- T042–T045 touch `loki_test.go` — treat T042 as sequential with the others if a single file, or split into `loki_labels_test.go`/`loki_drain_test.go` to parallelize
- T027 and T061 are independent files and can be written concurrently
- T047–T051, T053, T054, T058, and T062 are independent documentation and process tasks

---

## Parallel Example: Foundational Test Wave

```bash
# Seven independent test files, one agent each:
Task: "Write internal/logcore/logcore_test.go (C-01, C-02, C-03, C-07, C-08)"
Task: "Write internal/logcore/sink_test.go (C-09, C-10, C-11)"
Task: "Write internal/logcore/tracing_test.go (C-04, C-05, C-06)"
Task: "Write internal/logcore/loki_free_test.go (C-13)"
Task: "Write internal/logcore/logcore_bench_test.go (C-05 allocation contract)"
Task: "Write internal/logcore/concurrency_test.go (L-09)"
Task: "Write internal/testutil/imports_test.go logging-surface cases"
```

## Parallel Example: User Story 1 Test Wave

```bash
Task: "Write logging/import_isolation_test.go across four build configurations"
Task: "Write logging/logging_test.go (L-01..L-08, L-11, L-12 doc assertion)"
Task: "Write logging/example_test.go (ExampleNewLogger, verified output)"
Task: "Write internal/cmd/sizecheck/main_test.go (build failure vs ceiling vs ratio)"
Task: "Extend internal/cmd/doccover/main_test.go for package-qualified requirements"
Task: "Write examples/logging/main_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 0: Governance (T060) — already done in the remediation pass; verify the diff
2. Complete Phase 1: Setup
3. Complete Phase 2: Foundational — **critical**, blocks everything, and is where the Principle VIII Loki-coupling remediation lands
4. Complete Phase 3: User Story 1
5. **STOP and VALIDATE**: build `examples/logging` and `examples/rootlogging`, confirm zero forbidden imports in all four configurations, measure under 3 MB and at least a 75% reduction
6. The isolated surface is shippable at this point, but do not release without US2 — a non-breaking claim unverified by `go-apidiff` is not a non-breaking change

### Incremental Delivery

1. Governance → the constitution sanctions a second name for one constructor
2. Setup + Foundational → one logger, Loki decoupled, all existing gates green
3. US1 → isolated surface exists, proven isolated, size and ratio gated (**MVP**)
4. US2 → parity, alias identity, and `go-apidiff` prove existing adopters are untouched (**release gate**)
5. US3 → log shipping proven unchanged
6. Polish → documentation, coverage floors, size-gate documentation, PR message

### Parallel Team Strategy

Foundational is genuinely serial through T010–T017 (a single type graph moving between two packages), so parallelism there is limited to the test wave. Once T019's checkpoint is green, US1 and the US3 Loki assertions can proceed concurrently; US2's parity and identity tests need T026 from US1 first.

---

## Notes

- `[P]` = different files, no dependencies
- `[Story]` maps a task to a spec.md user story for traceability
- Verify every test fails for the right reason before implementing
- Commit after each task or logical group; never hand-edit `CHANGELOG.md`
- A green default build covers none of the three declined configurations — pass tags explicitly
- Never move a `sizecheck` constant to silence a failure whose cause you have not identified. The ceiling may be raised for a reviewed reason; the reduction ratio is toolchain-independent, so lowering it is a spec change, not a calibration (quickstart.md § Adjusting the size gate)
- A gate that passes without enforcing anything is worse than one that fails. Two tasks exist specifically to prevent that shape: T024/T029 (an unqualified `doccover` requirement satisfied by the wrong package's example) and T021's L-12 assertion (a documented promise carried only by prose)

---

## Implementation Findings

Two tasks did not conclude the way the plan anticipated. Both are recorded here
rather than reconciled away.

### T040 — the apidiff gate was wrong, not the code

The plan expected `go-apidiff` to stay silent because the types are aliased. It
does not: it keys type identity on the **declaring package**, so relocating
`Logger`, `Labels`, and `Option` into `internal/logcore` produces nine
`Incompatible changes` findings — one of which (`Flush`) has textually identical
"before" and "after" renderings.

The decisive evidence is this repository's own history. The v0.1.0 → v0.2.0
release performed the same refactor for `Error`, `Mode`, `Envelope`, `Schema`,
and the config/schema option types, shipped as a plain `feat:`, and was a no-op
for adopters. `go-apidiff` across that tag boundary reports **37 findings of the
same class today**. The gate landed later (PR #82) and had never been run against
the pattern the project had already blessed.

Resolution: a narrow type-relocation classifier in
`internal/cmd/apidiff-verdict`, not a `feat!:` and not a label. It excuses only
textually-identical renderings and same-name (or prefix-stripped) relocations
within this module; removals, renames, signature and member changes, and
out-of-module relocations stay breaking. Excused findings print in their own
report section, and `surfacecheck` still gates structural change. Its acceptance
test is that it must rule the shipped v0.2.0 diff non-breaking —
`TestClassifierMatchesShippedReleaseHistory`.

Verified: `apidiff-verdict` reports `public_breaking=false` for this branch, and
also for v0.1.0 → v0.2.0.

### T055 — `bench-check` is not measurable on this host

`make bench-check` fails locally, but the failures are not attributable to this
change and the measurement cannot support a verdict here.

A control experiment settles it: comparing **the identical tree against itself**
produces three budget failures, including **+72.6%** on
`BenchmarkLoggerTracingHook/active_trace_context`. Two runs of the real
comparison also disagreed sharply with each other (`BenchmarkParseConfig*` moved
from +340% to +5%; `BenchmarkWriteError` from passing to +518%) — and both of
those benchmarks are in code this feature never touches. The host is a 2-CPU
WSL2 VM sitting at load average 2.6, so the noise floor exceeds the 5% `ns/op`
budget by a wide margin.

What *is* verified, because it is load-independent:

- **Zero `allocs/op` violations** in every run. Allocation counts are
  deterministic, and the +1 absolute budget was never breached.
- The C-05 allocation contract is asserted directly by
  `TestNoActiveSpanPathIsAllocationFree` in **both** `internal/logcore` and
  `logging`, using `testing.AllocsPerRun`, and both pass.
- No previously-tracked benchmark is reported `Missing`: every root
  `BenchmarkLogger*` still exists, and the new `BenchmarkLogcore*` set uses
  distinct names (verified: zero duplicate bare benchmark names across packages,
  which is what `benchcheck` keys `Missing` on).

CI runs `bench-check` on a dedicated runner, where AGENTS.md makes it
authoritative. **It must be green there before merge**; a reviewer should not
read this note as a waiver.

### T062 — already fixed upstream

The inherited `crosscompile.yml` SC2086 failure is not present in the merged
file. It resolves build tags through a `GOFLAGS` env var and quotes every
expansion, so an unset variable contributes no argument — the same outcome the
task proposed via `${VAR:+...}`. Verified by `actionlint` (clean with shellcheck
disabled) and by scanning every `run:` block for unquoted expansions (none).
`actionlint`'s shellcheck pass cannot run in this environment: the snap-packaged
shellcheck times out waiting for snap system profiles, which is environmental
and unrelated to the repository.

Consequently T057's carve-out is unnecessary — there is no inherited red check to
excuse.
