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

The governance foundations have now all shipped: the stability + deprecation
policy (#17, Principles XI + XII), the coverage policy + CI gate (#21), the
README compatibility matrix (#23), the import-isolated public contract packages
(#78, spec/010), and the release-please flow confirmation (#14). With #17 in
place, `go-apidiff` (#26) and the `internal/` boundary migration (#18) are
unblocked. The Immediate Focus slot is open pending the next promotion — #27
(`ax.Error` recovery) is the natural candidate as the AX audit's highest-value
in-scope win. The coverage escalation queue (#63–#65, #69) provides parallel
quick wins that directly raise the repo-wide coverage floor toward 85%.

## Vision

Make every `rshade` Go CLI predictable for LLM agents and ergonomic for humans,
by owning the cross-cutting primitives once: stream separation, deterministic
exit codes, the `ax.Error` envelope, `__schema` discoverability, agent-safety
primitives, and short-lived-process-correct observability.

## Immediate Focus (v0.3.0 — v1.0 readiness & governance)

Single-WIP per the Promotion Gate (`target_focus_depth: 1`). **#17 (stability +
deprecation policy) shipped 2026-06-17**, delivering Principles XI + XII to the
constitution and unblocking #18 and #26. The focus slot is now open.

**Next promotion candidate: #27** (`ax.Error` recovery/remediation fields) — the
AX source audit's #1 in-scope win. It deepens the machine-contract half of AX and
enables agent self-correction without breaking the existing envelope shape.

## Near-Term Vision (v0.3.0 — governance queue)

- [ ] #27 `ax.Error` recovery/remediation fields [M] — add optional `retryable` /
  `recovery` / `next_action` so an agent can self-correct, not just report. The
  AX source audit's #1 in-scope win. *Candidate for next single-WIP promotion.*
- [ ] #12 Dedicated unit tests for `context.go`, `http.go`, `trace.go` [S] —
  quick win; these are currently exercised only indirectly. Parallel-friendly.
- [ ] #19 `SECURITY.md` disclosure policy [S] — reporting channel, SLA,
  supported-versions, out-of-scope (giant-Hujson DoS is a validation error).
- [ ] #69 `covercheck` type-design hardening — derived fields as methods +
  integer floor comparison [S] — two deferred refactors from spec/009; no
  public-API impact.
- [ ] #63 `internal/cli` unit tests + coverage floor enrollment [S] — remove
  from `excludedPackages`; direct path toward raising repo-wide floor to 85%.
- [ ] #64 `internal/mcp` unit tests + coverage floor enrollment [S] — same
  pattern; unlocks coverage improvement on the MCP adapter.
- [ ] #65 `internal/schema` unit tests + coverage floor enrollment [S] — same
  pattern; unlocks coverage improvement on the schema reflection layer.

## Future Vision (Long-Term)

### Library & runtime

- [ ] #53 Unified multi-format JSON codec [L] — one codec that reads
  JSON/Hujson/JSON5/NDJSON/JSONL with auto-detection and a convert API, while
  keeping output constitutionally strict. Widens the input contract beyond
  Hujson, so it is governed through a Spec Kit feature **and** a Constitution
  Principle V amendment. Reuse existing parsers, not hand-rolled dialects.
- [ ] #9 Hujson AST `Patch` write path [L] — mutate an existing Hujson file
  while preserving user formatting/comments, since strict-JSON writes can't.
  The retired Hujson input decision's read/write consequences are absorbed in
  [`specs/001-bound-config-reads/research.md`](specs/001-bound-config-reads/research.md).
- [ ] #10 `ax-go mcp-server` runnable wrapper [L] — wrap an ax-go CLI
  as a live MCP server with no per-tool work, building on the existing
  `__schema --as=mcp` adapter.
- [ ] #67 `ax-go mcp-server` — pkg.go.dev module metadata enrichment [M] —
  enrich MCP tool descriptions with synopsis, vuln summary, and version status at
  startup. *Blocked on #10 and pkg.go.dev `v1` stable API.*
- [ ] #11 Hot-path benchmarks with `-benchmem` [M] — back the zerolog
  "zero/near-zero allocation" claim (ADR-0009) with a real `testing.B`.
- [ ] #13 Agent-safety helpers for `--dry-run` side-effect suppression [M] —
  make it hard to accidentally cause side effects when `dry_run: true`.

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
- [ ] #18 Move remaining non-public helpers under `internal/` before v1.0 [L] —
  keep drawing the public-API boundary while it's still cheap. *Unblocked by #17.*
- [ ] #20 ADR-0012: directory layout [S] — document the flat-root-plus-internal
  layout decision before v1.0 locks it in.
- [ ] #22 benchstat regression budget in CI [M] — track hot-path benchmark
  deltas against a baseline. *Blocked on #11.*
- [ ] #24 Supply chain: SBOM + signed releases [M] — CycloneDX SBOM and cosign
  keyless signing on release artifacts.
- [ ] #25 CI cross-compile matrix [S] — `GOOS`/`GOARCH` build+vet across
  linux/darwin/windows × amd64/arm64.
- [ ] #26 go-apidiff in CI [M] — catch breaking public-API changes on PRs with a
  label-gated override. *Unblocked by #17.*
- [ ] #66 `doccover` — cross-validate `requiredSymbols` against pkg.go.dev
  `/v1/symbols` [M] — surface exported-but-ungated symbols automatically.
  *Blocked on pkg.go.dev `v1` stable API.*

## Completed Milestones

### 2026-Q2

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
- [x] Directory layout [S] — documents the public root facade, narrow contract
  packages, `internal/` implementation packages, `cmd/` runnable binaries, and
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
- Change public API or runtime behavior without a Spec Kit feature first.
