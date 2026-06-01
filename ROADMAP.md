# ax-go Strategic Roadmap

> Vision and boundaries live in [CONTEXT.md](./CONTEXT.md); behavioral contracts
> live in [`docs/adr/`](./docs/adr/). This roadmap tracks the gap between
> *ADR-accepted + shape-stubbed* and *runtime-promise-delivered +
> test-discipline-satisfied*.

## Status note

All open items are filed as GitHub issues (`#1`–`#31`) and carry `roadmap/*` +
`effort/*` labels synced to the sections below. Level-of-effort indicators:
`[S]` Small (1-2h), `[M]` Medium (½-1d), `[L]` Large (multi-day). No milestones
or release tags exist yet (pre-v0.1.0).

The codebase today is a **functional skeleton**: the envelope/schema/mode
*shapes* are real and tested, but the load-bearing runtime promises (real OTel
export, bounded reads, the full test discipline) are deferred. The work below
makes the foundation honest.

## Vision

Make every `rshade` Go CLI predictable for LLM agents and ergonomic for humans,
by owning the cross-cutting primitives once: stream separation, deterministic
exit codes, the `ax.Error` envelope, `__schema` discoverability, agent-safety
primitives, and short-lived-process-correct observability.

## Immediate Focus (v0.1.0 — make the skeleton safe)

Single-WIP per the Promotion Gate (`target_focus_depth: 1`).

- [ ] #1 Bound config reads at the read boundary [S] — `config.go:15` uses
  unbounded `io.ReadAll`, violating the "never read unbounded user input"
  guardrail. Add a default 1 MiB `io.LimitReader` cap (configurable via option);
  an oversized config is an `ExitValidation` error, not an OOM. Land the failing
  size-limit test first.

## Near-Term Vision (v0.2.0 — deliver the runtime promises)

- [ ] #2 Real OTel export + span lifecycle (ADR-0005) [L] — `telemetry.go:60`
  installs a `TracerProvider` with **no exporter**, so spans are discarded and
  no span wraps command execution (logger `trace_id`/`span_id` stay zero unless
  `TRACEPARENT` is injected). Add a configurable OTLP exporter, create a root
  span around `Execute`, and use a flush path that guarantees zero loss on a
  short-lived process exit.
- [ ] #3 Golden-file tests for stable-by-contract output [M] — `__schema` (ax +
  mcp) and the `ax.Error` envelope JSON are public API. Add `testdata/` golden
  files and diff tests so schema drift is caught.
- [ ] #4 Fuzz tests for every parser surface (AGENTS mandate) [M] — Hujson
  config input, idempotency-key validation, error-envelope round-trip, and
  `TRACEPARENT` extraction each need a `FuzzXxx`.
- [ ] #5 Output-determinism harness [M] — assert byte-identical `stdout` across
  two runs of the same input, modulo documented non-deterministic fields.
- [ ] #6 Build-time version injection [S] — wire `-ldflags "-X ..."` in the
  `Makefile` and `examples/integration/` so `__schema.version` is a real value,
  never empty/`dev`.

## Future Vision (Long-Term)

### Library & runtime

- [ ] #7 Loki direct-push addon (`loki.go`, ADR-0006) [M] — opt-in via
  `AX_LOKI_URL` as a **separate addon file**, never coupled into `logger.go`;
  the core logger must stay shippable with no Loki dependency. Non-blocking
  push, failures never break the CLI's primary work.
- [ ] #8 Logger cardinality-discipline enforcement [M] — enforce the
  label/payload split (ADR-0006) at the API level so high-cardinality fields
  can't be promoted to Loki labels.
- [ ] #9 Hujson AST `Patch` write path (ADR-0010) [L] — mutate an existing
  Hujson file while preserving user formatting/comments, since strict-JSON
  writes can't.
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
- [x] Hujson read parsing (ADR-0010 read path) [S].
- [x] zerolog logger + trace-correlation hook (ADR-0009) [M].
- [x] Telemetry W3C extraction + no-op scaffold (ADR-0005, partial) [M] —
  `TRACEPARENT`/`TRACESTATE` extraction and provider lifecycle; real export
  still pending (see Near-Term).
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
