# ax-go Context & Boundaries

> This file is a DERIVED expansion of the canonical project constitution at
> `.specify/memory/constitution.md`; on any conflict the constitution wins. It
> distills the durable *identity and limits* that `/roadmap` and contributors
> reason against. Every roadmap decision and feature must respect both the
> constitution and the boundaries defined here. When an idea would cross a
> boundary, the answer is "delegate it to the adopting CLI," not "relax the
> boundary." The canonical agent-instruction file is `AGENTS.md`.

## Core Architectural Identity

`ax-go` (module `github.com/rshade/ax-go`, package `ax`) is the **Agentic
Experience foundation** for Go CLI tools in the `rshade` portfolio. It is a
*library*, not an application: it makes any Cobra-based Go CLI predictable for
LLM agents while staying ergonomic for humans.

Its job is to own the cross-cutting, easy-to-get-wrong primitives once, so every
adopting tool inherits them for free:

- Stream separation (machine payload on `stdout`, everything else on `stderr`).
- Deterministic exit codes and a uniform `ax.Error` envelope.
- Machine discoverability via `__schema` (ax-native and MCP variants).
- Agent-safety primitives: `--idempotency-key`, `--dry-run`, output-mode
  resolution.
- Observability wiring (OpenTelemetry trace context, structured logging) that
  short-lived CLI processes usually get wrong.

The root package `github.com/rshade/ax-go` as `ax` remains the full-runtime
facade for complete CLIs: execution, telemetry lifecycle, logging, schema
command wiring, and transport helpers. The narrow public packages
`contract`, `config`, `schema`, and `id` are additive contract surfaces for
thin consumers that do not need runtime adapters. Those packages must stay
import-isolated from root `ax`, telemetry exporters/SDK setup, logger/Loki,
HTTP instrumentation, and gRPC runtime helpers.

The canonical behavioral authority is the constitution at
`.specify/memory/constitution.md`. The ADRs in `docs/adr/` are a FROZEN legacy
decision log being retired through the Spec Kit feature workflow — each ADR's
decisions are absorbed into a feature's `research.md` and the ADR is deleted as
that feature's final task. Where an ADR conflicts with the constitution, the
constitution wins.

## Technical Boundaries ("Hard No's")

`ax-go` deliberately does **not**:

- **Implement domain commands.** It wraps and instruments the adopting CLI's
  commands; it never ships business verbs of its own (beyond the reserved
  `__schema`).
- **Expose a "code-mode" / batch / `exec` MCP tool.** The MCP bridge maps
  **one command → one tool**, and `tools/call` runs exactly that command and
  returns its verbatim payload. `ax-go` never adds a tool that accepts a
  script, a code string, or a list of sub-commands to run server-side.
  Composition — loops, chaining, filtering, fan-out — belongs to the calling
  agent's own runtime (its "code mode"); re-implementing it here would only
  insert a redundant layer between the agent and the raw command. When callers
  need to move less data, the answer is a richer *contract* on the raw endpoint
  (field projection, pagination, progress events — see the `#131`/`#133`
  roadmap items), not a server-side compute layer. Each granular command *is*
  the raw endpoint an external code-mode agent should compose against.
- **Write anything but the machine payload to `stdout`.** Logs, progress,
  diagnostics, and error envelopes go to `stderr` — no exceptions for human
  convenience.
- **Emit non-strict output.** Hujson is accepted on **reads only**. Writes emit
  strict, minified JSON (bounded) or NDJSON (streaming). Hujson never leaves the
  process on `stdout`.
- **Persist state.** No databases, no on-disk caches, no mutable
  package-level globals, no `init()` that touches the network/filesystem/env.
  Configuration enters through constructors.
- **Run its own observability backend.** Tracing delegates to OTel-compatible
  systems (Tempo, Jaeger, Honeycomb). Default log shipping is decoupled
  (`stderr` → Promtail/Alloy); direct Loki push is opt-in via `AX_LOKI_URL` and
  lives as a separate addon (`loki.go`), never coupled into the core logger
  (`logger.go`). The logger must remain shippable with no Loki dependency.
- **Invent ID schemes.** Observability IDs come from the OTel SDK (W3C); entity
  IDs are UUID v7 and idempotency keys are UUID v4, both via `google/uuid`.
  Observability IDs and resource IDs are never interchanged.
- **Offer a pluggable logger backend.** The `ax.Logger` interface exists *only*
  as a migration seam for a future superseding decision. No
  `ax.WithLogger(...)`-style runtime backend selection, no second concrete
  logger (Constitution Principles VI and VIII).
- **Add a second CLI framework.** Cobra is the only framework (ADR-0008).
- **Compromise security defaults.** Never skip TLS verification, never log PII /
  secrets / tokens, never compose log messages from un-sanitized user input,
  never read unbounded user input (config reads are size-capped).
- **Ship `dev`/`unknown` versions.** Version is injected at build via
  `-ldflags "-X ..."` and surfaced in `__schema` and error envelopes.

## Delegated AX Pillars

The Agentic Experience (AX) discipline that `ax-go` operationalizes is broader
than what a CLI foundation should own (see
`docs/src/content/docs/sources.md`). `ax-go`
deliberately addresses the **machine-contract** half of AX — discoverability,
determinism, transparency, agent-safety, guardrails — and **cedes the
stateful/experiential half to the adopting CLI and the agent runtime.** These
omissions are decisions, not oversights:

- **Access / authentication & agent identity.** `ax-go` ships secure transport
  defaults (`ax.HTTPClient`, `ax.GRPCDial`) and an auth-failure exit code (`4`),
  but holds no credentials and implements no auth/identity-delegation flow. Auth
  mechanics belong to the adopting CLI. (`ax.GRPCDial` is present by default; a
  consumer may decline it at build time — see **Optional Dependency Roots**
  below. No insecure alternative is ever offered in its place.)
- **Orchestration.** Triggering, coordinating, and scaling multi-agent or
  multi-step runs is out of scope. `__schema --as=mcp` / `mcp-server` is the one
  bridge `ax-go` provides — it makes an ax-go CLI a *node* an external
  orchestrator can compose; it is not itself an orchestrator.
- **Persistent memory / cross-session context.** Forbidden by the "no persisted
  state" Hard No. Memory is real AX infrastructure — it lives in the application
  layer, not here.
- **Preference learning / inference.** Stateful and domain-specific; delegated.
- **Natural-language intent parsing.** The agent forms intent; `ax-go` is the
  deterministic, typed tool the agent *calls* after intent is formed.

`ax-go` is the **brake, not the engine**: the layer that makes an autonomous
agent's actions safe, reversible, and auditable — not the layer that makes them
proactive.

## Optional Dependency Roots

Two negative build constraints let a consumer decline the two heaviest
dependency roots. Both default to off; setting neither is the state every
existing build is already in.

| Tag | Declines | Effect |
| --- | --- | --- |
| `ax_no_otlp` | OTLP HTTP trace export | endpoint becomes one stderr diagnostic; still fail-open |
| `ax_no_grpc` | `ax.GRPCDial` | identifier absent from the build |

The size benefit requires **both** — each removes one of two independent roots
over the same gRPC subtree. Measured on linux/amd64: `ax_no_grpc` alone −0.00%,
`ax_no_otlp` alone −15.1%, together **−63.3%** and exactly zero packages from
the gRPC, protobuf, OTLP-proto, and grpc-gateway trees.

**Boundary this does not cross:** tracing degrades to *no export*, never to *no
tracing*. W3C context extraction, the recording root span, `trace_id`/`span_id`
log correlation, and `AX_OTEL_DEBUG` span output are behaviourally identical in
all four configurations, and every machine payload is byte-identical.
`ax.GRPCDial` is the only public identifier whose presence varies — enforced on
every PR by `make surface-check`.

**The thin packages are not a tracing escape hatch.** `contract`, `config`,
`schema`, and `id` link zero gRPC and always have, but they carry **no live
tracing**: `contract.TraceIDFromContext` reads a value previously stored in the
context and does not resolve an active span. Live tracing exists only in the
root facade.

## Data Source of Truth

- **Behavioral contract:** the constitution at `.specify/memory/constitution.md`
  (canonical). `docs/adr/` is a frozen legacy log retired via the Spec Kit
  workflow; changing public API or runtime behavior is specified through a Spec
  Kit feature (GitHub issue → spec → plan → tasks), not a new ADR.
- **Agent instructions:** `AGENTS.md` (imported by `CLAUDE.md` and `GEMINI.md`).
- **Inbound runtime data:** command-line flags, Hujson config on stdin/file,
  and W3C trace context via the `TRACEPARENT` / `TRACESTATE` environment
  variables. `ax-go` holds no authoritative data of its own.
- **Roadmap state:** `ROADMAP.md` (this repo) plus GitHub issues/labels when
  they exist.

## Interaction Model

**Inbound:**

- Humans and agents invoke an adopting CLI; `ax.Execute()` wraps the Cobra root.
- Output mode is resolved as `--format` flag → `AGENT_MODE` env → TTY detection
  and carried in `context.Context`.
- Config is read as Hujson (size-capped); trace context is extracted from the
  environment.

**Outbound:**

- `stdout`: strict JSON / NDJSON machine payload, byte-deterministic modulo
  documented non-deterministic fields (timestamps, `trace_id`, auto-generated
  `idempotency_key`).
- `stderr`: structured zerolog JSON, progress, and `ax.Error` envelopes.
- OTel spans to a configured exporter; optional Loki push when `AX_LOKI_URL` is
  set; outbound HTTP/gRPC calls auto-propagate trace context with secure
  defaults.

## Verification

To check whether a proposed change respects these boundaries, ask:

1. **Stream purity:** Does anything non-payload reach `stdout`? If yes, reject.
2. **Determinism:** Do two runs with identical input produce byte-identical
   `stdout` (modulo documented non-deterministic fields)? Envelopes must use
   structs, not maps; timestamps must be RFC 3339 UTC.
3. **Constitution & ADR alignment:** Does the change satisfy the constitution's
   principles? Public API / runtime changes go through the Spec Kit feature
   workflow (absorbing any governing frozen ADR into research.md), not a new ADR.
4. **Scope:** Is this a cross-cutting primitive (belongs in `ax-go`) or a domain
   feature (belongs in the adopting CLI)? When in doubt, delegate.
5. **Security:** TLS preserved, no PII/secret logging, user input sanitized via
   zerolog field methods, reads bounded?
6. **Dependency discipline:** Does the stdlib or an existing dependency already
   cover this? New dependencies must be justified.
7. **Import isolation:** If the change touches `contract`, `config`, `schema`,
   or `id`, do their dependency graphs still exclude root `ax` and runtime
   adapters?
8. **Build-tag coverage:** If the change touches code behind `//go:build
   ax_no_grpc` or `//go:build ax_no_otlp`, has it been vetted, linted, and tested
   *with those tags passed*? A green default run does not cover them. Does
   `make surface-check` still pass, and if the exported surface moved
   intentionally, was the regenerated baseline reviewed line by line?

## Roadmap Sync Behavior

`ax-go` opts into the `/roadmap` Promotion Gate to enforce single-WIP
discipline. It is a solo-maintained foundational library where finishing one
contract before starting the next matters more than breadth.

- `target_focus_depth`: 1
- `composition_required`: []
- `procrastination_threshold_days`: 7
- `epic_promotion`: enforce

### `roadmap-meta` issue-body convention

Issues may embed an HTML comment to drive promotion ordering:

```html
<!-- roadmap-meta
trigger: free-form description of what makes this ready
trigger-pending: YYYY-MM-DD | event-name | upstream-flag-name
unblocks: 12, 13
epic-parent: 9
-->
```

Field semantics: `trigger` documents the readiness condition; `trigger-pending`
defers eligibility until a date/event arrives; `unblocks` lists issues this one
unblocks (raises its promotion score); `epic-parent` references the prerequisite
issue whose closure makes this child promotion-eligible. All fields are
optional; an absent block means "no metadata, always eligible."
