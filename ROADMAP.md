# ax-go Strategic Roadmap

> Vision and boundaries live in [CONTEXT.md](./CONTEXT.md); behavioral contracts
> live in [`docs/adr/`](./docs/adr/). This roadmap tracks the gap between
> *ADR-accepted + shape-stubbed* and *runtime-promise-delivered +
> test-discipline-satisfied*.

## Status note

All open items are filed as GitHub issues (`#4`–`#48`) and carry `roadmap/*` +
`effort/*` labels synced to the sections below. Level-of-effort indicators:
`[S]` Small (1-2h), `[M]` Medium (½-1d), `[L]` Large (multi-day). `v0.0.1` is
released via release-please; the `v0.1.0` output contracts are frozen and await
the next release tag.

The load-bearing runtime promises are now landing: bounded config reads (#1) and
real OTel export + span lifecycle (#2) have shipped. What remains to make the
foundation fully honest is the test discipline (#4 fuzz, #5 determinism), the
telemetry follow-up polish (#45–#48), and the v1.0 governance surface below.

## Vision

Make every `rshade` Go CLI predictable for LLM agents and ergonomic for humans,
by owning the cross-cutting primitives once: stream separation, deterministic
exit codes, the `ax.Error` envelope, `__schema` discoverability, agent-safety
primitives, and short-lived-process-correct observability.

## Immediate Focus (v0.2.0 — deliver the runtime promises)

Single-WIP per the Promotion Gate (`target_focus_depth: 1`). The prior slot
holder (#6, build-time version injection) shipped 2026-06-10 and #2 (real OTel
export) closed 2026-06-13, opening this slot.

- [ ] #45 Refactor telemetry internals [S] — dedupe the sanitizer + mutex
  writer and simplify the fail-open helpers in `internal/telemetry`, hardening
  the code merged for #2 while it is fresh.
  *Promoted by /roadmap sync on 2026-06-12 — epic-child of the just-closed #2
  (score +25); fills the single Immediate Focus slot.*

## Near-Term Vision (v0.2.0 — remaining runtime promises)

- [ ] #46 Telemetry unit tests [M] — first unit tests for `internal/telemetry`,
  covering fail-open, `tracestate`, and debug-export paths on the #2 code.
- [ ] #47 Inject `service.version` in the integration example [S] — set the OTel
  resource `service.version` so the reference CLI stops emitting
  `version=0.0.0-unknown`.
- [ ] #48 Telemetry doc fixes [S] — correct the understated `Start` doc, drop the
  stale `WithSyncer` reference, and add the writer rationale.
- [ ] #4 Fuzz tests for every parser surface (AGENTS mandate) [M] — Hujson
  config input, idempotency-key validation, error-envelope round-trip, and
  `TRACEPARENT` extraction each need a `FuzzXxx`. This is the canonical tracker
  for the parser-fuzz deferral recorded in
  [`specs/001-bound-config-reads/plan.md`](specs/001-bound-config-reads/plan.md)
  and
  [`specs/001-bound-config-reads/research.md`](specs/001-bound-config-reads/research.md);
  do not open a parallel tracker for the bounded-config feature.
- [ ] #5 Output-determinism harness [M] — assert byte-identical `stdout` across
  two runs of the same input, modulo documented non-deterministic fields.

## Future Vision (Long-Term)

### Library & runtime

- [ ] #7 Loki direct-push addon (`loki.go`, ADR-0006) [M] — opt-in via
  `AX_LOKI_URL` as a **separate addon file**, never coupled into `logger.go`;
  the core logger must stay shippable with no Loki dependency. Non-blocking
  push, failures never break the CLI's primary work.
- [ ] #8 Logger cardinality-discipline enforcement [M] — enforce the
  label/payload split (ADR-0006) at the API level so high-cardinality fields
  can't be promoted to Loki labels.
- [ ] #9 Hujson AST `Patch` write path [L] — mutate an existing Hujson file
  while preserving user formatting/comments, since strict-JSON writes can't.
  The retired Hujson input decision's read/write consequences are absorbed in
  [`specs/001-bound-config-reads/research.md`](specs/001-bound-config-reads/research.md).
- [ ] #10 `ax-go mcp-server` runnable wrapper (ADR-0003) [L] — wrap an ax-go CLI
  as a live MCP server with no per-tool work, building on the existing
  `__schema --as=mcp` adapter.
- [ ] #11 Hot-path benchmarks with `-benchmem` [M] — back the zerolog
  "zero/near-zero allocation" claim (ADR-0009) with a real `testing.B`.
- [ ] #12 Dedicated unit tests for `context.go`, `http.go`, `trace.go` [S] —
  these are currently exercised only indirectly.
- [ ] #13 Agent-safety helpers for `--dry-run` side-effect suppression [M] —
  make it hard to accidentally cause side effects when `dry_run: true`.
- [ ] #14 Wire up release-please flow [S] — config exists
  (`release-please-config.json`) but no release is cut. release-please must
  *generate* `CHANGELOG.md` from commit history; it is never hand-authored
  (see AGENTS.md → Changelog & Releases).

### AX surface enhancements

*From the AX source audit ([`docs/sources.md`](./docs/sources.md)) — in-scope
gaps that deepen the machine-contract half of AX.*

- [ ] #27 `ax.Error` recovery/remediation fields (amend ADR-0002) [M] — add
  optional `retryable` / `recovery` / `next_action` so an agent can self-correct,
  not just report. The audit's #1 in-scope win.
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
  `idempotency_replayed`, `requires_confirmation` (human handoff). Extends #13.
- [ ] #31 Agent-acceptance test harness [M] — drive the CLI as an agent would
  (parse `__schema` → invoke → assert envelope). Folds into #15.

### v1.0 readiness & governance

- [ ] #15 `examples/integration` audit + extend to full Common DNA surface [L] —
  exercise every Core AX Mandate in the reference CLI; pin `__schema` and
  envelope shapes with golden files. Pairs with #3/#5.
- [ ] #16 `__schema` non-deterministic-field enumeration [M] — declare a
  `non_deterministic_fields` array per command so agents diff safely. Pairs
  with #5.
- [ ] #17 ADR-0013 (SemVer/stability) + ADR-0014 (deprecation) [M] — define what
  "breaking" means, pre-v1.0 contract, and the deprecation lifecycle. Gates
  #18, #23, #26.
- [ ] #18 Move remaining non-public helpers under `internal/` before v1.0 [L] —
  keep drawing the public-API boundary while it's still cheap.
- [ ] #19 `SECURITY.md` disclosure policy [S] — reporting channel, SLA,
  supported-versions, out-of-scope (giant-Hujson DoS is a validation error).
- [ ] #21 Test-coverage policy + CI enforcement [M] — baseline, floor, and a CI
  gate that fails below it.
- [ ] #22 benchstat regression budget in CI [M] — track hot-path benchmark
  deltas against a baseline. *Blocked on #11.*
- [ ] #23 README compatibility matrix [S] — ax-go ↔ Go ↔ consumer-version
  table; references ADR-0013 (#17).
- [ ] #24 Supply chain: SBOM + signed releases [M] — CycloneDX SBOM and cosign
  keyless signing on release artifacts.
- [ ] #25 CI cross-compile matrix [S] — `GOOS`/`GOARCH` build+vet across
  linux/darwin/windows × amd64/arm64.
- [ ] #26 go-apidiff in CI [M] — catch breaking public-API changes on PRs with a
  label-gated override. *Blocked on #17.*

## Completed Milestones

### 2026-Q2

- [x] ADRs 0001–0011 accepted [L] — agent-mode trigger, error envelope, schema
  format, trace-ID format, OTel integration, Loki integration, ID strategy,
  Cobra framework, zerolog, Hujson input, JSON output.
- [x] Mode resolution skeleton (ADR-0001) [M] — `--format` > `AGENT_MODE` > TTY,
  carried in `context.Context`, fully tested.
- [x] `ax.Error` envelope shape (ADR-0002) [M] — struct, options, exit-code
  mapping, `stderr` writer.
- [x] `__schema` reflection — ax + mcp emit (ADR-0003) [M] — Cobra command-tree
  reflection, auto-injected reserved command.
- [x] `Execute()` Cobra lifecycle wrapper [L] — flag injection, schema
  injection, mode/idempotency/dry-run context, error normalization.
- [x] ID generation — UUID v4/v7 (ADR-0007) [S].
- [x] JSON + NDJSON envelope writers (ADR-0011) [S].
- [x] #1 Hujson read parsing and bounded read cap
  ([specs/001-bound-config-reads](specs/001-bound-config-reads)) [S] — default
  1 MiB `io.LimitReader`; an oversized config is an `ExitValidation` error, not
  an OOM. Shipped 2026-06-06 (`ea74c7d`).
- [x] zerolog logger + trace-correlation hook (ADR-0009) [M].
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
- [x] ADR-0012 directory layout [S] — documents the public root facade,
  `internal/` implementation packages, `cmd/` runnable binaries, and
  `testdata/` public contract fixtures.

## Boundary Safeguards

From [CONTEXT.md](./CONTEXT.md) — roadmap items must never:

- Write non-payload data to `stdout`, or emit non-strict JSON output.
- Persist state or introduce mutable package-level globals.
- Couple a log-shipping backend (Loki) into the core logger.
- Invent ID schemes or interchange observability and resource IDs.
- Add a pluggable logger backend while ADR-0009 stands.
- Add a second CLI framework, skip TLS, log PII/secrets, or read unbounded input.
- Ship `dev`/`unknown` versions to production agents.
- Change public API or runtime behavior without an ADR first.
