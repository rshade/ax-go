<!--
SYNC IMPACT REPORT
==================
Version change: template (unversioned placeholders) → 1.0.0
Bump rationale: First formal ratification. The file previously held only raw
[BRACKET] template tokens; filling it with concrete principles and governance
is the initial adoption, recorded as "template → 1.0.0".

Modified principles (placeholder → concrete):
  [PRINCIPLE_1] → I. Stream Separation (NON-NEGOTIABLE)
  [PRINCIPLE_2] → II. Deterministic Output & Exit Codes
  [PRINCIPLE_3] → III. Machine Discoverability via __schema
  [PRINCIPLE_4] → IV. Agent-Safety Primitives
  [PRINCIPLE_5] → V. Asymmetric JSON I/O
  (added)       → VI. ADR-Governed Scope — Library, Not Application
  (added)       → VII. Test-First Discipline (NON-NEGOTIABLE)
  (added)       → VIII. Observability & ID Discipline
  (added)       → IX. Security & Resource Safety
  (added)       → X. Idiomatic Go & Dependency Minimalism

Added sections:
  - Additional Constraints & Boundaries (the library "Hard No's")
  - Development Workflow & Quality Gates (incl. ADR-retirement mechanic)
  - Governance (canonical authority, frozen-ADR policy, amendment/versioning)

Removed sections: none (template placeholders replaced in place).

Templates requiring updates:
  ✅ .specify/templates/plan-template.md  (Governing ADR(s) field + Constitution
       Check ADR-absorption gate; updated in this change)
  ✅ .specify/templates/spec-template.md  (Source-inputs assumption; updated in
       this change)
  ✅ .specify/templates/tasks-template.md (ADR-retirement final Polish task;
       updated in this change)
  (no .specify/templates/commands/ directory present — nothing to reconcile there)

Derived docs reconciled in this change:
  ✅ CONTEXT.md  (no longer self-declares as "the project constitution"; reframed
       as a derived Context & Boundaries expansion)
  ✅ AGENTS.md   ("add/update an ADR first" mandate flipped to record-decisions-in-
       research.md + no-new-ADRs; points at this constitution)

Follow-up TODOs (retired incrementally, NOT in this change):
  - README.md ADR index/links, ROADMAP.md issue↔ADR tags, and Go doc-comments
    (error.go, logger.go, mode.go) are updated per-feature as
    each governing ADR is absorbed into a feature's research.md and deleted as that
    feature's final task (Principle VI + Governance).
-->

# ax-go Constitution

ax-go is the Agentic Experience (AX) foundation library (Go module
`github.com/rshade/ax-go`, package `ax`) for Go CLI tools in the rshade
portfolio. Its purpose: make every adopting Go CLI as predictable for LLM agents
as it is ergonomic for humans, by owning the cross-cutting, easy-to-get-wrong
primitives once. ax-go is the *brake, not the engine* — the layer that makes an
autonomous agent's actions safe, reversible, and auditable.

## Core Principles

### I. Stream Separation (NON-NEGOTIABLE)

- `stdout` MUST carry ONLY the final machine payload (strict JSON, or NDJSON for
  streaming).
- `stderr` MUST carry everything else: logs, progress, diagnostics, and `ax.Error`
  envelopes.
- There are NO exceptions for human convenience.

**Rationale**: An agent must be able to pipe `stdout` straight into a JSON parser
while humans and log collectors read `stderr`. Any leak to `stdout` breaks that
contract for every consumer.

### II. Deterministic Output & Exit Codes

- The same input MUST produce byte-identical `stdout`, modulo documented
  non-deterministic fields (timestamps, `trace_id`, auto-generated
  `idempotency_key`).
- Envelopes MUST be modeled with structs, never maps; timestamps MUST be RFC 3339
  UTC; IDs and money values MUST NOT use bare `float64` (precision loss).
- Exit codes are fixed: `0` success, `1` unknown/internal, `2` validation/bad
  input, `3` network/timeout, `4` authentication/permission.
- Errors MUST be emitted as the `ax.Error` envelope on `stderr`.

**Rationale**: Determinism is the machine equivalent of trust — agents diff
outputs across runs to detect drift, so non-determinism silently breaks their
flows.

### III. Machine Discoverability via `__schema`

- Every CLI MUST expose `__schema`, emitting structured JSON of the command tree,
  flags, types, and examples.
- A `__schema --as=mcp` adapter MUST emit MCP-tool-compatible output.
- `__schema` output and the `ax.Error` envelope are public API and MUST be guarded
  by golden-file tests.

**Rationale**: Agents ground themselves without guessing, and an ax-go CLI can be
wrapped as an MCP server with no per-tool work.

### IV. Agent-Safety Primitives

- `--idempotency-key` MUST auto-generate a UUID v4 when absent and surface the key
  in the output envelope.
- `--dry-run` MUST emit the same envelope with `dry_run: true` and cause no side
  effects.
- Output mode MUST resolve in the order `--format` flag > `AGENT_MODE` env > TTY
  detection, and MUST be carried in `context.Context`.

**Rationale**: These make a CLI safe under agent retries and rehearsals, killing
duplicate-create and surprise side effects.

### V. Asymmetric JSON I/O

- Reads MAY accept Hujson (comments, trailing commas) for human convenience.
- Config reads MUST be size-capped at the read boundary (default 1 MiB,
  configurable); an oversized config is a validation error (exit `2`), never an
  OOM.
- Writes MUST emit strict, minified JSON (bounded) or NDJSON (streaming); Hujson
  MUST NEVER reach `stdout`.
- Mutating an existing Hujson file MUST use the AST `Patch` path to preserve user
  formatting and comments.

**Rationale**: Humans get convenience on input; agents get strictness and
determinism on output.

### VI. ADR-Governed Scope — Library, Not Application

- ax-go owns cross-cutting AX primitives only and MUST delegate all domain commands
  to the adopting CLI (beyond the reserved `__schema`).
- It MUST NOT persist state (no databases, on-disk caches, mutable package-level
  globals, or `init()` that touches network/filesystem/env for runtime behavior).
- It MUST NOT run its own observability backend, MUST NOT add a second CLI framework
  (Cobra only), and MUST NOT offer a pluggable logger backend while the zerolog
  decision stands.
- Any public-API or runtime-behavior change — including a renamed exported
  identifier — MUST be specified through the Spec Kit feature workflow, NOT a new
  ADR.

**Rationale**: ax-go is the brake, not the engine; scope creep is how a foundation
library becomes an unmaintainable application.

### VII. Test-First Discipline (NON-NEGOTIABLE)

- Tests land before implementation: every new behavior or exported function starts
  with a failing test asserting the contract; every bug fix starts with a failing
  regression test.
- Use table-driven tests by default; an `ExampleXxx` for every exported symbol
  agents or humans invoke; golden files for stable-by-contract output; fuzz tests
  for every parser surface; and `testing.B` with `-benchmem` for any
  allocation/performance claim.
- `go test -race ./...` is REQUIRED, not optional. `go vet ./...` and
  `golangci-lint run` MUST be clean.

**Rationale**: The contract is verified, not asserted; `ExampleXxx` functions are
also how agents learn the API surface.

### VIII. Observability & ID Discipline

- Contexts MUST propagate W3C Trace Context by default via the OpenTelemetry SDK.
- Logging MUST go through `ax.NewLogger(ctx)` (zerolog), with `trace_id`/`span_id`
  on every line when a span is active.
- Loki shipping MUST be decoupled (`stderr` → Promtail/Alloy by default);
  `AX_LOKI_URL` direct push is opt-in and MUST live as a separate addon, never
  coupled into `logger.go`.
- The cardinality split MUST be enforced at the API level: `environment`,
  `application`, `level`, `host`, and `version` are labels;
  `trace_id`/`span_id`/`user_id`/durations/resource IDs are payload.
- IDs: OTel trace/span IDs for observability, UUID v7 for entity/resource IDs,
  UUID v4 for idempotency keys. Observability IDs and resource IDs MUST NEVER be
  interchanged.

**Rationale**: Short-lived CLI processes usually get observability wrong; ax-go
gets it right once for the whole portfolio.

### IX. Security & Resource Safety

- NEVER log PII, secrets, tokens, or credentials — not in labels, not in payload.
- NEVER compose log messages from un-sanitized user input; use zerolog field
  methods that escape (`.Str`, `.Int`, …), never `.Msg(fmt.Sprintf(...userInput...))`.
- NEVER skip TLS verification; use the `ax.HTTPClient` / `ax.GRPCDial` secure
  defaults.
- NEVER read unbounded user input (see Principle V).
- NEVER `panic` in library code — return errors, and wrap with `%w` so `errors.Is`
  and `errors.As` work against every returned error.

**Rationale**: Privacy, security, and DoS-resistance are defaults, not options.

### X. Idiomatic Go & Dependency Minimalism

- `context.Context` MUST be the first parameter of any function doing I/O, making
  outbound calls, running goroutines, or otherwise cancelable.
- No mutable package-level state; configuration enters via constructors; use
  functional options beyond 2-3 config knobs.
- Non-public mechanics live under `internal/`; the public package `ax` stays at the
  module root (no `pkg/` or `src/`).
- `defer Close()` every `io.Closer`.
- Justify every new dependency (stdlib first, existing deps next).
- Version MUST be injected at build via `-ldflags "-X ..."`; never ship `dev` or
  `unknown` to production agents.

**Rationale**: A predictable, low-maintenance foundation that ages well and stays
cheap to migrate.

## Additional Constraints & Boundaries

ax-go deliberately addresses the **machine-contract** half of AX
(discoverability, determinism, transparency, agent-safety, guardrails) and cedes
the **stateful/experiential** half to the adopting CLI and the agent runtime.
These are decisions, not oversights:

- **Domain commands, auth/identity, orchestration, persistent memory, preference
  learning, and natural-language intent parsing are out of scope.** ax-go ships
  secure transport defaults and an auth-failure exit code (`4`) but holds no
  credentials and implements no auth flow. `__schema --as=mcp` / `mcp-server` is
  the single bridge that makes an ax-go CLI a *node* an external orchestrator can
  compose; it is not itself an orchestrator.
- **When an idea would cross a boundary, the answer is "delegate it to the adopting
  CLI," not "relax the boundary."**

## Development Workflow & Quality Gates

- **Spec Kit is the change workflow.** Features are driven by GitHub issues and any
  governing (frozen) ADR(s), specified via `/speckit-specify`, planned via
  `/speckit-plan`, broken down via `/speckit-tasks`, and built via
  `/speckit-implement`.
- **ADR absorption is part of every feature it touches.** When a feature is
  governed by one or more ADRs, that ADR's decision, considered alternatives, and
  consequences MUST be transcribed into the feature's `research.md` (Phase 0). The
  feature's `tasks.md` MUST include, as its FINAL task, deletion of those ADR
  file(s) and the updating of every reference to them. An ADR MUST NOT be deleted
  until its decisions are recorded in `research.md`.
- **Constitution Check gate.** `/speckit-plan` MUST verify the principles above AND
  that any governing ADR's decisions are absorbed into `research.md` before that ADR
  is deleted. Violations MUST be justified in the plan's Complexity Tracking table.
- **Before handing work back:** run `gofmt`, `go test -race ./...`, `go vet ./...`,
  and `golangci-lint run`; all MUST be clean. Keep `README.md` and
  `examples/integration/` current with public behavior.
- **Releases** are owned by release-please from Conventional Commit history;
  `CHANGELOG.md` is never hand-edited.

## Governance

- This constitution is the single, SUPREME, living governance document for ax-go and
  supersedes all other practices and documents. `CONTEXT.md`, `AGENTS.md`,
  `GEMINI.md`, and `CLAUDE.md` are DERIVED artifacts reconciled to it; on any
  conflict, this constitution wins.
- The Architecture Decision Records in `docs/adr/` are a FROZEN legacy decision log.
  They MUST NOT be created or edited going forward; where an ADR conflicts with this
  constitution, the constitution wins. Their standing cross-cutting decisions already
  live as principles above.
- ADRs are retired EXCLUSIVELY through the Spec Kit feature workflow, never by ad-hoc
  edits or deletions. When a feature (driven by a GitHub issue) is specified and
  planned, the decisions, considered alternatives, and consequences of every ADR that
  governs that feature MUST be captured in the feature's `research.md` (Phase 0). That
  feature's `tasks.md` MUST include, as its FINAL task, deletion of those ADR file(s)
  and updating every reference to them (the README ADR index and links, `CONTEXT.md`,
  `AGENTS.md`, `ROADMAP.md`, and Go doc-comments). An ADR
  MUST NOT be deleted until its decisions are recorded in `research.md`.
- New architectural decisions are recorded in the consuming feature's `research.md`
  and, when cross-cutting, elevated to a constitution principle by amendment — not as
  new ADRs.
- **Amendment procedure**: propose via a PR that edits this file plus a Sync Impact
  Report, applies the correct semantic-version bump, re-syncs the dependent templates
  (plan/spec/tasks), and reconciles the derived docs in the same change.
- **Versioning policy** (semantic): MAJOR = backward-incompatible governance/principle
  removal or redefinition; MINOR = a new principle/section or materially expanded
  guidance; PATCH = clarifications and wording.
- **Compliance**: the `/speckit-plan` Constitution Check gate MUST verify these
  principles and the ADR-absorption rule; every PR review verifies compliance; any
  violation MUST be justified in the plan's Complexity Tracking table.

**Version**: 1.0.0 | **Ratified**: 2026-06-01 | **Last Amended**: 2026-06-01
