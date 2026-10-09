# ax-go Strategic Roadmap

> Vision and boundaries live in [CONTEXT.md](./CONTEXT.md); behavioral contracts
> live in the constitution, Spec Kit features, and remaining frozen ADRs. This
> roadmap tracks the gap between *specified contract* and
> *runtime-promise-delivered + test-discipline-satisfied*.

## Status note

All open items are filed as GitHub issues and carry `roadmap/*` + `effort/*`
labels synced to the sections below. Level-of-effort indicators: `[S]` Small
(1-2h), `[M]` Medium (½-1d), `[L]` Large (multi-day). release-please is live:
`v0.0.1`–`v0.9.0` have been cut automatically with generated `CHANGELOG.md`
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
`internal/cmd/benchcheck`). #18 (`internal/` migration audit) shipped
2026-07-23, closing the epic opened by #20; the agent-safety envelope's first
entry, `--yes` no-prompt invariant (#121), shipped 2026-08-26, alongside two
more deferred refactors (#69, #120). The `axtest` full-lifecycle test helper
(#178, 2026-08-26) and the Guard/Perform audit-logging variant (#179,
2026-08-28) shipped the same week. `WithFlushFunc` `ExecuteOption` (#119) and
the live-resolving `ax.MetadataFromContext` accessor (#212) both shipped
2026-09-04/08. A wave of 20 new AX-surface issues (#119–#138) was filed
2026-07-19 (verified against live code before filing) and is now tracked
across Near-Term and Future Vision below.

**2026-09-08 sync**: a P0 correctness bug (#218 — agent-safety context lost
when a subcommand declares its own persistent hook) surfaced the moment
Immediate Focus emptied out. The operator chose a one-time exception to the
single-WIP Promotion Gate (`target_focus_depth: 1` unchanged in CONTEXT.md)
and promoted three items together: #218, #123, and #122. #218 shipped the
same day; #123 and #122 remain in flight. Future syncs return to single-WIP
discipline.

**2026-10-07 sync**: a second deliberate exception, sized for an unattended
overnight agent run. The operator promoted four well-scoped items that need
no operator decision — #237 (MCP subtree `tools/call` bug), plus #230, #229,
and #231 (quality-gate reports) — kept #123 in flight, and demoted #122 to
Near-Term because a default-inverting public-contract change should not run
unattended. #233 and #234 were relabelled `spike` + `timebox/1d`: their
deliverable is a verdict, which an agent can research but not render.

**2026-10-08 sync**: the overnight queue cleared. #230, #237, #229, #231,
and #123 all shipped on 2026-10-08, which retires both deliberate exceptions.
Immediate Focus now holds what is actually in flight: #270, claimed through
`/pick-issue`, and #28, which open PR #272 implements.

**2026-10-09 sync**: #270, #28, #274, and #232 all shipped, and `v0.9.0` was
released. Immediate Focus is back at single-WIP with #284 (PR #296 open), so
the Promotion Gate passed silently. The operator then promoted the
follow-up, #285, by hand, stacked on #284's branch. `main`'s CI had been red
since a Renovate `x/tools` bump left stale `go.sum` hashes; #295 fixed that on
2026-10-09.

## Vision

Make every `rshade` Go CLI predictable for LLM agents and ergonomic for humans,
by owning the cross-cutting primitives once: stream separation, deterministic
exit codes, the `ax.Error` envelope, `__schema` discoverability, agent-safety
primitives, and short-lived-process-correct observability.

## Immediate Focus (v0.3.0 — v1.0 readiness & governance)

Single-WIP per the Promotion Gate (`target_focus_depth: 1`). Two items are in
flight, one over target: the operator promoted #285 by hand while #284 was in
review, because #285 stacks directly on #284's branch.

- [ ] #284 `__schema` `known_codes` omits the config package's runtime error
  codes [S] — the list grows from 4 to 9 codes. The five `config_*` codes become
  `contract` constants, and spec 032 records the scope rule. In review as PR
  #296. *Claimed via `/pick-issue`; recorded by /roadmap sync on 2026-10-09.*
- [ ] #285 `error_code` values are unregistered string literals, so
  `known_codes` can drift [M] — give every runtime code a `contract` constant,
  plus an explicit authoring-time exclusion for `invalid_schema_declaration`.
  Convert the remaining emit sites (`execute.go`, `confirm.go`, the MCP server,
  `mcp`, `schema`) and add a source-walking test that fails on any `NewError`
  code that is not a registered constant. Unblocks #125 and #130. *Promoted
  from Near-Term by the operator on 2026-10-09 via `/pick-issue 285`, stacked
  on #284's branch until PR #296 merges.*

## Near-Term Vision (v0.3.0 — governance queue)

- [ ] #122 Dry-run-by-default with `--apply` and declared side-effect class
  [L] — invert the default so mutating commands are safe unless explicitly
  applied. Extends the shipped #13 `--dry-run` guards. *Demoted from
  Immediate Focus on 2026-10-07: a default-inverting public-contract change
  needs an attended Spec Kit run.*
**On deck for the next single-WIP promotion:** #234 (mutation-testing
spike), whose trigger fired on 2026-10-09 and which consumes no lane. #137
(declare per-command MCP elicitation points) is epic-eligible now that its
parent #121 has shipped, but its `roadmap-meta trigger-pending:
issue-121-shipped` field is stale. Clear it before treating #137 as ready.

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

### Agent-contract deepening (new — 2026-07-19)

*Filed 2026-07-19 from the `kimi-features` audit; each verified as
not-yet-implemented against live code before filing. All widen the
machine-contract half of AX (discoverability, error detail, agent-safety) and
route through a Spec Kit feature — plus a Constitution amendment where they add
a runtime contract.*

- [ ] #124 Field-level validation detail (`violations`) on the exit-2 error
  envelope [M] — per-field failure structure so an agent can self-correct input.
- [ ] #125 Structured "did you mean" suggestions in the agent-mode error
  envelope [S] — typo/near-miss command + flag hints as data.
- [ ] #126 Scope `__schema` output to a command subtree via positional path
  [M] — emit only the relevant slice of the command tree.
- [ ] #127 Surface Cobra deprecation markers in `__schema` and MCP metadata
  [S] — carry `//Deprecated:` status into the machine contract.
- [ ] #128 Sensitive-flag marking with guaranteed redaction in schema, logs,
  and envelopes [M] — one declaration, redacted everywhere (ties to the no-PII
  guardrail).
- [ ] #129 Agent-facing `--timeout` flag + expected-duration schema hints [M]
  — bounded execution with a declared budget agents can plan around.
- [ ] #130 Structured error envelope + deterministic exit on SIGTERM/SIGINT
  [M] — signal handling that still emits a clean `ax.Error` and a stable code.
- [ ] #131 Output budget: `--fields` projection, `--limit`/`--cursor`
  pagination, `truncated`/`next_cursor` envelope [L] — bound unbounded result
  sets without breaking determinism.
- [ ] #132 Cross-environment determinism principle + audit + `testutil` helper
  [S] — pin byte-identical output across hosts, not just across runs.
- [ ] #133 Structured NDJSON progress events on `stderr` with MCP
  `progressToken` mapping [M] — machine-readable progress that bridges to MCP.
- [ ] #134 `__selftest` read-only probe (version, config validity, dependency
  reachability) [M] — a safe pre-flight an agent can call before real work.
- [ ] #135 `resume_token` envelope convention for checkpointable multi-step
  commands [S] — resumability signal without persisting state in `ax-go`.
- [ ] #136 Static auth-preflight metadata in `__schema` (contract-declared
  CLAIM) [M] — declare required auth up front; mechanics still delegated.
- [ ] #137 Declare per-command MCP elicitation points (contract-first) [M] —
  schema-declared prompts the MCP layer can drive.

### Quality gates & slop detection (new — 2026-09-08)

*Filed from a Go slop-detection audit. #229, #230, and #231 shipped
2026-10-08, and #232 shipped 2026-10-09. The two spikes remain. #234's trigger
("run after #229 and #232 land") has now fully fired.*

- [ ] #233 NilAway periodic sweep for interprocedural nil-flow findings —
  **spike**, `timebox/1d`. Deliverable: triaged first-run findings plus a
  sweep-cadence decision; deliberately not a CI gate.
- [ ] #234 Mutation-testing spike against `covercheck`'s coverage floor —
  **spike**, `timebox/1d`. Deliverable: an adopt / revisit / drop verdict on
  whether the floors measure assertion, not just execution.

### v1.0 readiness & governance

- [ ] #24 Supply chain: SBOM + signed releases [M] — CycloneDX SBOM and cosign
  keyless signing on release artifacts.

### Recurring maintenance

*Not issue-tracked — release-cadence upkeep, no `roadmap/*` label sync.*

- Re-run the Go Proverbs audit at each `0.MINOR.0` release and refresh both
  outputs: the Diátaxis explanation page
  ([docs explanation: ax-go and the Go Proverbs](./docs/src/content/docs/explanation/go-proverbs-audit.md))
  and the shareable artifact mirror it references. The audit's snapshot facts
  (dependency count, `fmt.Errorf` totals, interface method counts, panic/unsafe
  greps) drift with the code; the design commitments are the durable part.
  Last run 2026-08-31 at `v0.5.0`.

## Completed Milestones

### 2026-Q4

- [x] #232 `slopcheck` gate for struct fields assigned but never read. Closed 2026-10-09. [M]
- [x] #270 Live `mcp-server` serves declared prompts, resources, instructions. Closed 2026-10-08. [L]
- [x] #28 Per-flag `__schema` enums, examples, and capability classes. Closed 2026-10-08. [M]
- [x] #274 Nil-command `RegisterEnvelope` subtest asserts its no-op contract. Closed 2026-10-08. [S]
- [x] #123 Structured success warnings with `--strict` escalation. Closed 2026-10-08. [M]
- [x] #237 MCP subtree `tools/call` dispatches on the real root. Closed 2026-10-08. [M]
- [x] #231 Report-only `dupl` clone detection over test files. Closed 2026-10-08. [M]
- [x] #229 `ast-grep` report for assertion-free subtests. Closed 2026-10-08. [M]
- [x] #230 Pin `revive` to an explicit opt-in rule set. Closed 2026-10-08. [S]
- [x] #228 Enable the `modernize` linter across the build-tag matrix. Closed 2026-10-08. [S]
- [x] #138 Declare MCP prompts and static resources in `__schema`. Closed 2026-10-08. [M]

### 2026-Q3

- [x] #227 `deadcode` reachability gate for unexported and internal symbols. Closed 2026-09-24. [S]
- [x] #253 `mcp.Exclude` node-only tool exclusion; auto-skip groups and help. Closed 2026-09-24. [M]
- [x] #218 `ax.Execute` wraps persistent hooks on every command, not just root. Closed 2026-09-08. [L]
- [x] #212 `ax.MetadataFromContext` with live-resolving trace/span IDs. Closed 2026-09-08. [S]
- [x] #119 `WithFlushFunc` ExecuteOption drains `ax.Flush` on shutdown. Closed 2026-09-04. [S]
- [x] #69 `covercheck` type-design hardening: derived fields, integer floors. Closed 2026-08-30. [S]
- [x] #120 Reconcile SPECKIT plan pointers; dedupe spec dir numbering. Closed 2026-08-30. [S]
- [x] #179 Guard/Perform audit-logging variant. Closed 2026-08-28. [M]
- [x] #178 `axtest` full-lifecycle command test helper. Closed 2026-08-26. [M]
- [x] #121 `--yes` no-prompt invariant (`confirmation_required` envelope). Closed 2026-08-26. [M]
- [x] #144 Import-isolated `logging` package (−81% logging-only binary). Closed 2026-07-24. [L]
- [x] #143 Independent `ax_no_otlp` / `ax_no_grpc` opt-out build tags. Closed 2026-07-23. [L]
- [x] #18 Move remaining non-public helpers under `internal/` before v1.0. Closed 2026-07-23. [L]
- [x] #145 Docs: import-isolated packages provide no live tracing. Closed 2026-07-23. [S]
- [x] #16 `__schema` `non_deterministic_fields` enumeration per command. Closed 2026-07-22. [M]
- [x] #22 Performance regression budget via `benchstat` in CI. Closed 2026-07-08. [M]
- [x] #25 CI cross-compile matrix across `GOOS`/`GOARCH`. Closed 2026-07-08. [S]
- [x] #65 `internal/schema` unit tests and coverage-floor enrollment. Closed 2026-07-06. [S]
- [x] #64 `internal/mcp` unit tests and coverage-floor enrollment. Closed 2026-07-06. [S]
- [x] #63 `internal/cli` unit tests and coverage-floor enrollment. Closed 2026-07-06. [S]
- [x] #19 `SECURITY.md` vulnerability disclosure policy. Closed 2026-07-06. [S]

### 2026-Q2

- [x] #27 `ax.Error` `retryable` / `retry_after_seconds` recovery fields. Closed 2026-06-30. [M]
- [x] #15 `examples/integration` audit across the full Common DNA surface. Closed 2026-06-30. [L]
- [x] #13 `ax.Guard` / `ax.Perform` `--dry-run` side-effect guards. Closed 2026-06-29. [M]
- [x] #11 Hot-path logger benchmarks with `-benchmem`. Closed 2026-06-29. [M]
- [x] #20 Directory-layout decision recorded in `AGENTS.md` and README. Closed 2026-06-29. [S]
- [x] #12 Unit tests for `context.go`, `http.go`, `trace.go`. Closed 2026-06-28. [S]
- [x] #10 `ax-go mcp-server` runnable wrapper. Closed 2026-06-28. [L]
- [x] #9 Hujson AST `Patch` write path preserving formatting. Closed 2026-06-27. [L]
- [x] #26 `go-apidiff` in CI with label-gated override. Closed 2026-06-26. [M]
- [x] #78 Import-isolated public contract packages. Closed 2026-06-21. [L]
- [x] #23 README compatibility matrix. Closed 2026-06-21. [S]
- [x] #71 Adopt shared rshade-theme design tokens. Closed 2026-06-21. [S]
- [x] #21 Test-coverage policy and `covercheck` CI gate. Closed 2026-06-20. [M]
- [x] #68 Scaffold Astro Starlight docs site. Closed 2026-06-20. [S]
- [x] #17 Stability and deprecation policy (Principles XI + XII). Closed 2026-06-17. [M]
- [x] #14 Wire up the release-please flow. Closed 2026-06-17. [S]
- [x] #7 Opt-in Loki direct-push addon (`loki.go`). Closed 2026-06-16. [M]
- [x] #8 Logger label-cardinality discipline enforcement. Closed 2026-06-16. [M]
- [x] #5 Output-determinism harness. Closed 2026-06-15. [M]
- [x] #4 Fuzz tests for every parser surface. Closed 2026-06-14. [M]
- [x] #45 Refactor telemetry internals. Closed 2026-06-14. [S]
- [x] #46 First unit tests for `internal/telemetry`. Closed 2026-06-14. [M]
- [x] #47 Inject `service.version` in the integration example. Closed 2026-06-14. [S]
- [x] #48 Telemetry doc fixes. Closed 2026-06-14. [S]
- [x] #3 Golden-file tests for `__schema` and the `ax.Error` envelope. Closed 2026-06-11. [M]
- [x] #6 Build-time version injection via `-ldflags`. Closed 2026-06-10. [S]
- [x] #1 Hujson read parsing with a bounded 1 MiB read cap. Closed 2026-06-06. [S]
- [x] #2 Real OTel export and span lifecycle. [L]
- [x] Legacy ADRs 0001–0011 accepted. [L]
- [x] Mode resolution skeleton (`--format` > `AGENT_MODE` > TTY). [M]
- [x] `ax.Error` envelope shape, options, and exit-code mapping. [M]
- [x] `__schema` reflection with ax and MCP emit. [M]
- [x] `Execute()` Cobra lifecycle wrapper. [L]
- [x] ID generation — UUID v4/v7. [S]
- [x] JSON and NDJSON envelope writers. [S]
- [x] zerolog logger and trace-correlation hook. [M]
- [x] Integration example CLI (`examples/integration/`). [M]
- [x] Directory layout documented. [S]

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
