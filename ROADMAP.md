# ax-go Strategic Roadmap

> Vision and boundaries live in [CONTEXT.md](./CONTEXT.md); behavioral contracts
> live in the constitution, Spec Kit features, and remaining frozen ADRs. This
> roadmap tracks the gap between *specified contract* and
> *runtime-promise-delivered + test-discipline-satisfied*.

## Status note

All open items are filed as GitHub issues and carry `roadmap/*` + `effort/*`
labels synced to the sections below. Level-of-effort indicators: `[S]` Small
(1-2h), `[M]` Medium (½-1d), `[L]` Large (multi-day). release-please is live:
`v0.0.1`–`v0.2.0` have been cut automatically with generated `CHANGELOG.md`
entries; the `v0.1.0` output contracts remain frozen.

The governance foundations have all shipped: the stability + deprecation
policy (#17, Principles XI + XII), the coverage policy + CI gate (#21), the
README compatibility matrix (#23), the import-isolated public contract packages
(#78, spec/010), the release-please flow confirmation (#14), and `go-apidiff`
in CI (#26). A wave of runtime contracts also landed — the Hujson AST `Patch`
write path (#9), the `ax-go mcp-server` runnable wrapper (#10), the
`--dry-run` side-effect guards (#13, spec/012), and `ax.Error`
recovery/remediation fields (#27) — alongside dedicated unit tests for
`context.go`/`http.go`/`trace.go` (#12), hot-path logger benchmarks (#11), the
`examples/integration` Common DNA audit (#15), `SECURITY.md` (#19), the
coverage-floor escalation trio for `internal/cli`/`internal/mcp`/`internal/schema`
(#63–#65), and the CI performance regression budget (#22, `benchstat` via
`internal/cmd/benchcheck`). **The single-WIP slot in Immediate Focus is now
open**; #18 (`internal/` migration audit) is the leading epic-promotion
candidate for the next `/roadmap sync`.

## Vision

Make every `rshade` Go CLI predictable for LLM agents and ergonomic for humans,
by owning the cross-cutting primitives once: stream separation, deterministic
exit codes, the `ax.Error` envelope, `__schema` discoverability, agent-safety
primitives, and short-lived-process-correct observability.

## Immediate Focus (v0.3.0 — v1.0 readiness & governance)

Single-WIP per the Promotion Gate (`target_focus_depth: 1`).

*No active WIP — #22 (performance regression budget) shipped 2026-07-08.
Awaiting the next `/roadmap sync` to promote #18 (see Near-Term Vision).*

## Near-Term Vision (v0.3.0 — governance queue)

**On deck — next promotion:** #18 is now epic-promotion-eligible — its
blocking directory-layout decision (#17/#20) closed 2026-06-29 — and is the
leading candidate for Immediate Focus now that #22 has closed.

- [ ] #18 Move remaining non-public helpers under `internal/` before v1.0 [L]
  — keep drawing the public-API boundary while it's still cheap. Unblocked by
  #17/#20 (directory-layout decision, closed 2026-06-29).
- [ ] #69 `covercheck` type-design hardening — derived fields as methods +
  integer floor comparison [S] — two deferred refactors from spec/009; no
  public-API impact.

## Future Vision (Long-Term)

### Library & runtime

- [ ] #53 Unified multi-format JSON codec [L] — one codec that reads
  JSON/Hujson/JSON5/NDJSON/JSONL with auto-detection and a convert API, while
  keeping output constitutionally strict. Widens the input contract beyond
  Hujson, so it is governed through a Spec Kit feature **and** a Constitution
  Principle V amendment. Reuse existing parsers, not hand-rolled dialects.

### AX surface enhancements

*From the AX source audit
([`docs/src/content/docs/sources.md`](./docs/src/content/docs/sources.md)) —
in-scope gaps that deepen the machine-contract half of AX.*

- [ ] #28 Richer per-flag `__schema` semantics [M] — defaults, enums,
  required-vs-optional, per-flag examples, side-effect class. Lets an agent infer
  correct calls from the contract alone. Complements #16.
- [ ] #29 Static agent-discovery artifact (`llms.txt`) — emit vs delegate [M] —
  ADR decision on fetch-before-invoke discovery derived from `__schema`.
- [ ] #32 Build-time `llms.txt` generation [L] — an exported
  `ax.GenerateLLMsTxt(...)` plus a `cmd/` docs tool that merge the reflected
  command/flag skeleton (the same reflection that powers `__schema`) with an
  author-supplied curated preamble and link graph. A *documentation artifact*,
  not a new runtime machine format —
  `__schema` JSON + `--as=mcp` stay unchanged. Consensus of a `/decide` debate;
  deferred until the first real downstream consumer needs a published `llms.txt`.
  Pairs with #29 (the emit-vs-delegate decision it implements).
- [ ] #30 Envelope runtime trust signals [M] — `side_effects_performed`,
  `idempotency_replayed`, `requires_confirmation` (human handoff). Extends the
  now-shipped #13 `--dry-run` guards.
- [ ] #31 Agent-acceptance test harness [M] — drive the CLI as an agent would
  (parse `__schema` → invoke → assert envelope). Folds into #15.
- [ ] #66 doccover: cross-validate `requiredSymbols` against pkg.go.dev
  `/v1beta/symbols` [M] — catch doc-coverage baseline drift against the live
  published symbol set.
- [ ] #67 mcp-server: enrich MCP tool descriptions with pkg.go.dev module
  metadata [M] — deepen `__schema --as=mcp` output so wrapped tools carry richer
  agent-facing descriptions (synopsis, vuln summary, version status). Builds on
  the now-shipped #10 wrapper; *blocked on pkg.go.dev `v1` stable API.*

### v1.0 readiness & governance

- [ ] #16 `__schema` non-deterministic-field enumeration [M] — declare a
  `non_deterministic_fields` array per command so agents diff safely. Pairs
  with #5.
- [ ] #24 Supply chain: SBOM + signed releases [M] — CycloneDX SBOM and cosign
  keyless signing on release artifacts.
- [ ] #25 CI cross-compile matrix [S] — `GOOS`/`GOARCH` build+vet across
  linux/darwin/windows × amd64/arm64.

## Completed Milestones

### 2026-Q3

- [x] #22 Performance regression budget: `benchstat` in CI [M] —
  `internal/cmd/benchcheck` compares hot-path benchmark deltas (logger hook,
  `__schema` reflection, error-envelope marshal, Hujson parse) against a
  same-host run from the pull request base ref using
  `golang.org/x/perf/benchstat`; fails CI beyond the regression budget (>5%
  ns/op, significant-only; >1 alloc/op).
  Shipped via
  [`specs/014-bench-regression-budget`](specs/014-bench-regression-budget/).
  Closed 2026-07-08.
- [x] #65 `internal/schema` unit tests + coverage floor enrollment [S] —
  removed from `excludedPackages`; calibrated a 95% per-package floor. Closed
  2026-07-06.
- [x] #64 `internal/mcp` unit tests + coverage floor enrollment [S] — removed
  from `excludedPackages`; calibrated a 90% per-package floor. Closed
  2026-07-06.
- [x] #63 `internal/cli` unit tests + coverage floor enrollment [S] — removed
  from `excludedPackages`; calibrated a 100% per-package floor. Closed
  2026-07-06.
- [x] #19 `SECURITY.md` disclosure policy [S] — vulnerability disclosure
  policy, reporting channel, and supported-versions table. Closed 2026-07-06.

### 2026-Q2

- [x] #27 `ax.Error` recovery/remediation fields [M] — added optional
  `retryable` (tri-state) and `retry_after_seconds` (relative backoff) so an
  agent can self-correct, not just report; `actionable_fix`/`suggestions` stay
  the free-text recovery hints. Shipped via PR #95. Closed 2026-06-30.
- [x] #15 `examples/integration` audit + extend to full Common DNA surface [L]
  — exercises every Core AX Mandate in the reference CLI; pins `__schema` and
  envelope shapes with golden files. Closed 2026-06-30.
- [x] #13 Agent-safety helpers for `--dry-run` side-effect suppression [M] —
  `ax.Guard` / `ax.Perform` make it hard to accidentally cause side effects when
  `dry_run: true`; suppressed actions log to `stderr`. Shipped via
  [`specs/012-dry-run-guards`](specs/012-dry-run-guards/). Closed 2026-06-29.
- [x] #11 Hot-path benchmarks with `-benchmem` [M] — `BenchmarkLogger*` (incl.
  the tracing hook) and `BenchmarkLoggerDisabledLevel` in `logger_bench_test.go`
  back ADR-0009's "zero/near-zero allocation" claim: 0 allocs/op on the no-span
  and disabled paths, an honest 48 B / 2 allocs on active-span (OTel ID
  hex-encoding). Landed in `a6f09c7`. Closed 2026-06-29.
- [x] #20 Directory-layout decision recorded [S] — the flat-root public `ax`
  facade + narrow contract packages + `internal/` implementation layout is
  documented in `AGENTS.md` "Repository Layout" and README. The originally
  requested ADR-0012 is obsolete under the ADR freeze (decisions are absorbed
  into Spec Kit features / the constitution, not new ADRs). Closed 2026-06-29.
- [x] #12 Dedicated unit tests for `context.go`, `http.go`, `trace.go` [S] —
  direct coverage for helpers previously exercised only indirectly. Closed
  2026-06-28.
- [x] #10 `ax-go mcp-server` runnable wrapper [L] — wrap any ax-go CLI as a live
  MCP server with no per-tool work, building on the `__schema --as=mcp` adapter.
  Closed 2026-06-28.
- [x] #9 Hujson AST `Patch` write path [L] — mutate an existing Hujson file while
  preserving user formatting/comments, since strict-JSON writes can't. The
  read/write consequences of the retired Hujson input decision are absorbed in
  [`specs/001-bound-config-reads/research.md`](specs/001-bound-config-reads/research.md).
  Closed 2026-06-27.
- [x] #26 go-apidiff in CI [M] — catch breaking public-API changes on PRs with a
  label-gated override; scoped to the public surface (`ax` + the `contract`,
  `config`, `schema`, `id` packages). Closed 2026-06-26.
- [x] #78 Import-isolated public contract packages (`contract/`, `config/`,
  `schema/`, `id/`) [L] — narrow public packages for thin consumers, fully
  import-isolated from the root runtime facade, telemetry, and gRPC adapters.
  Shipped via
  [`specs/010-import-isolated-contracts`](specs/010-import-isolated-contracts/).
  Closed 2026-06-21.
- [x] #23 README compatibility matrix [S] — ax-go ↔ Go ↔ consumer-version
  table added to README; references Principles XI + XII (#17). Closed 2026-06-21.
- [x] #71 Adopt shared rshade-theme design tokens [S] — consistent design tokens
  across the Astro Starlight docs site. Closed 2026-06-21.
- [x] #21 Test-coverage policy + CI enforcement [M] — repo-wide 70% floor,
  per-package overrides, `internal/cmd/covercheck` CI gate, and Codecov
  integration. Shipped via
  [`specs/009-coverage-policy-ci`](specs/009-coverage-policy-ci/). Closed
  2026-06-20.
- [x] #68 Scaffold Astro Starlight docs site [S] — bootstrapped `docs/`
  Astro Starlight site for ax-go documentation. Closed 2026-06-20.
- [x] #17 Stability + deprecation policy [M] — defined what "breaking" means for
  pre-v1.0; added Principles XI + XII to the constitution; unblocks #18 and #26.
  Shipped via
  [`specs/008-stability-deprecation-policy`](specs/008-stability-deprecation-policy/).
  Closed 2026-06-17.
- [x] #14 Wire up release-please flow [S] — release-please confirmed working;
  `v0.0.1`, `v0.0.2`, `v0.1.0`, and `v0.2.0` cut automatically with generated
  `CHANGELOG.md` entries; commit conventions documented. Closed 2026-06-17.
- [x] #7 Loki direct-push addon (`loki.go`) [M] — opt-in via `AX_LOKI_URL` as a
  **separate addon file**, never coupled into `logger.go`; the core logger stays
  shippable with no Loki dependency. Non-blocking push, failures never break the
  CLI's primary work. Shipped via
  [`specs/007-loki-direct-push`](specs/007-loki-direct-push/). Closed 2026-06-16.
- [x] #8 Logger cardinality-discipline enforcement [M] — enforces the
  label/payload split (Constitution Principle VIII / FR-009) in `loki.go`
  `buildStreamMap`, so high-cardinality fields can't be promoted to Loki labels.
  Delivered with #7; closed by sync 2026-06-16.
- [x] #5 Output-determinism harness [M] — asserts byte-identical `stdout` across
  two runs of the same input, modulo documented non-deterministic fields. Closed
  2026-06-15.
- [x] #4 Fuzz tests for every parser surface [M] — `FuzzXxx` over Hujson config
  input, idempotency-key validation, error-envelope round-trip, and `TRACEPARENT`
  extraction (AGENTS mandate). Closed 2026-06-14.
- [x] #45 Refactor telemetry internals [S] — deduped the sanitizer + mutex writer
  and simplified the fail-open helpers in `internal/telemetry`, hardening the #2
  code. Closed 2026-06-14.
- [x] #46 Telemetry unit tests [M] — first unit tests for `internal/telemetry`,
  covering fail-open, `tracestate`, and debug-export paths on the #2 code. Closed
  2026-06-14.
- [x] #47 Inject `service.version` in the integration example [S] — sets the OTel
  resource `service.version` so the reference CLI no longer emits
  `version=0.0.0-unknown`. Closed 2026-06-14.
- [x] #48 Telemetry doc fixes [S] — corrected the understated `Start` doc, dropped
  the stale `WithSyncer` reference, and added the writer rationale. Closed
  2026-06-14.
- [x] Legacy ADRs 0001–0011 accepted [L] — agent-mode trigger, error envelope,
  schema format, trace-ID format, OTel integration, Loki integration, ID
  strategy, Cobra framework, zerolog, Hujson input, JSON output.
- [x] Mode resolution skeleton [M] — `--format` > `AGENT_MODE` > TTY,
  carried in `context.Context`, fully tested.
- [x] `ax.Error` envelope shape [M] — struct, options, exit-code
  mapping, `stderr` writer.
- [x] `__schema` reflection — ax + mcp emit [M] — Cobra command-tree
  reflection, auto-injected reserved command.
- [x] `Execute()` Cobra lifecycle wrapper [L] — flag injection, schema
  injection, mode/idempotency/dry-run context, error normalization.
- [x] ID generation — UUID v4/v7 [S].
- [x] JSON + NDJSON envelope writers [S].
- [x] #1 Hujson read parsing and bounded read cap
  ([specs/001-bound-config-reads](specs/001-bound-config-reads)) [S] — default
  1 MiB `io.LimitReader`; an oversized config is an `ExitValidation` error, not
  an OOM. Shipped 2026-06-06 (`ea74c7d`).
- [x] zerolog logger + trace-correlation hook (Constitution Principle VIII) [M].
- [x] #2 Real OTel export + span lifecycle
  ([specs/004-real-otel-export](specs/004-real-otel-export)) [L] —
  recording root span around `Execute`, log correlation with no collector,
  OTLP HTTP export with synchronous bounded attempts, `AX_OTEL_DEBUG` stderr
  export, fail-open diagnostics, and outbound propagation coverage.
- [x] #6 Build-time version injection via `-ldflags` [S] — `__schema.version`,
  the `ax.Error` envelope `version`, and the logger `version` label resolve
  through `ax.ResolveVersion`; never ships `dev`/`unknown`. Closed 2026-06-10.
- [x] #3 Golden-file tests for `__schema` + `ax.Error` envelope [M] — public
  output shapes pinned by `testdata/` fixtures so schema drift breaks CI. Closed
  2026-06-11.
- [x] Integration example CLI (`examples/integration/`) [M].
- [x] Directory layout [S] — documents the public root facade, narrow contract
  packages, `internal/` implementation packages, `cmd/` runnable binaries, and
  `testdata/` public contract fixtures.

## Boundary Safeguards

From [CONTEXT.md](./CONTEXT.md) — roadmap items must never:

- Write non-payload data to `stdout`, or emit non-strict JSON output.
- Persist state or introduce mutable package-level globals.
- Couple a log-shipping backend (Loki) into the core logger.
- Invent ID schemes or interchange observability and resource IDs.
- Add a pluggable logger backend (Constitution Principle VI: the `ax.Logger`
  interface is a migration seam, not a backend selector).
- Add a second CLI framework, skip TLS, log PII/secrets, or read unbounded input.
- Ship `dev`/`unknown` versions to production agents.
- Change public API or runtime behavior without a Spec Kit feature first.
