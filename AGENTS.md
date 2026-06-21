# Repository Guidelines

## Project Overview

`ax-go` is the Agentic Experience foundation for Go CLI tools in the
`rshade` portfolio. Its purpose is to make Go CLIs predictable for LLM agents
and still ergonomic for human engineers.

The module is `github.com/rshade/ax-go`, the package name should be `ax`, and
the project currently targets Go `1.26.4`. The canonical source of truth for
behavior and public API decisions is the constitution at
`.specify/memory/constitution.md`. The ADRs in `docs/adr/` are a FROZEN legacy
decision log being retired through the Spec Kit feature workflow; where an ADR
conflicts with the constitution, the constitution wins.

## Repository Layout

- `go.mod` declares the module and Go version.
- `README.md` explains the project mission, standards, and roadmap.
- `.specify/memory/constitution.md` is the canonical, supreme governance
  document; `docs/adr/`, this file, `CONTEXT.md`, `GEMINI.md`, and `CLAUDE.md`
  are derived and reconciled to it.
- `docs/adr/` contains a FROZEN legacy log of Architecture Decision Records. Do
  not create or edit ADRs. When a Spec Kit feature is governed by one, absorb its
  decisions into the feature's `research.md` and delete it as the final task.
- `internal/` contains private implementation packages behind the public
  package `ax` and the narrow public contract packages. Do not move code into
  additional public subpackages without a Spec Kit feature or constitution
  amendment.
- `contract/`, `config/`, `schema/`, and `id/` are approved public contract
  packages for thin consumers. They must remain import-isolated from the root
  runtime facade, telemetry exporters/SDK setup, logger/Loki, HTTP
  instrumentation, and gRPC runtime adapters.
- `testdata/` contains golden fixtures for stable public JSON contracts.
- `cmd/` is reserved for runnable support binaries such as the future
  `ax-go mcp-server`; do not create placeholder commands before behavior
  exists.
- Do not add `pkg/` or `src/`; public package `ax` lives at the module root
  to preserve the `github.com/rshade/ax-go` import path.
- `examples/integration/` contains the runnable integration command for
  exercising the public API from a real Cobra CLI. Keep it current when
  public behavior or examples change.
- `AGENTS.md` is the canonical agent instruction file. `GEMINI.md` and
  `CLAUDE.md` should import this file rather than duplicate guidance.

## Core AX Mandates

- Stream separation is non-negotiable:
  - `stdout` is reserved for the final machine payload.
  - `stderr` is used for logs, progress, diagnostics, and structured error
    envelopes.
- Exit codes are deterministic:
  - `0`: success
  - `1`: unknown/internal error
  - `2`: validation/bad input
  - `3`: network/timeout
  - `4`: authentication/permission
- Every CLI built on ax-go must expose `__schema`, emitting a structured JSON
  description of command tree, flags, types, and examples. A companion
  `__schema --as=mcp` adapter emits MCP-tool-compatible output so an ax-go CLI
  can be wrapped as an MCP server via `ax-go mcp-server` with no per-tool work.
- Input accepts Hujson for human convenience on **reads only** — writes emit
  strict JSON (Hujson cannot Marshal comments). To mutate an existing Hujson
  file while preserving user formatting, use the AST `Patch` path
  preserved in `specs/001-bound-config-reads/research.md`. Output emits strict,
  minified JSON for bounded payloads and NDJSON for streaming or unbounded
  result sets (absorbed into specs/006-output-determinism-harness/research.md).
- All commands must support agent-safety primitives:
  - `--idempotency-key`, auto-generating UUID v4 when absent and surfacing the
    key in the output envelope.
  - `--dry-run`, producing the same envelope with `dry_run: true` and no side
    effects.
  - output mode resolution through `--format`, `AGENT_MODE`, and TTY detection.
- Errors use the standard `ax.Error` envelope and are emitted to `stderr`.
- **Output is deterministic.** Given the same inputs, two runs of the same
  command produce byte-identical `stdout` payloads, modulo fields documented
  as non-deterministic (timestamps, `trace_id`, auto-generated
  `idempotency_key`). Use structs (not maps) for any envelope shape; emit
  timestamps as RFC 3339 UTC; never use bare `float64` for IDs or money
  values (precision loss). Agents compare outputs across runs —
  non-determinism silently breaks their flows.

## Accepted Architecture

- Agent mode precedence is `--format` flag, then `AGENT_MODE`, then TTY
  detection. Carry the resolved mode in `context.Context`.
- Cobra is the CLI framework. `ax.Execute()` wraps Cobra execution to add mode
  resolution and OpenTelemetry flush-on-exit behavior.
- Contexts must propagate W3C Trace Context IDs by default through the
  OpenTelemetry SDK.
- Grafana Loki is the log aggregation target; Tempo, Jaeger, and
  Honeycomb-compatible systems are valid trace backends through OTel.
  Default log shipping is decoupled (`stderr` → Promtail/Alloy DaemonSet);
  direct push is opt-in via `AX_LOKI_URL` (Constitution Principle VIII;
  see `specs/007-loki-direct-push/research.md`). Enforce label
  cardinality discipline at the logger API level: `environment`,
  `application`, `level`, `host`, and `version` are labels
  (low-cardinality, indexed); `trace_id`, `span_id`, `user_id`, durations,
  and resource IDs are payload (never promoted to labels).
- Use OpenTelemetry trace/span IDs for observability. Use **UUID v7** (via
  `github.com/google/uuid`) for resource and entity IDs, and **UUID v4**
  (same library) for auto-generated idempotency keys. Never mix observability
  IDs with resource/entity IDs.
- Use `github.com/rs/zerolog` for structured logging. The canonical
  constructor is `ax.NewLogger(ctx)`, returning an `ax.Logger` (initially
  backed by `*zerolog.Logger`) with trace correlation wired in.
- The `ax.Logger` interface exists ONLY as a migration seam for a future
  superseding ADR. Do not introduce parallel-pluggable logger backends, an
  `ax.WithLogger(...)`-style runtime selection API, or a second concrete
  logger implementation while ADR-0009 stands.
- Stability and deprecation are governed by Constitution Principle XI
  (**Stability & SemVer**) and Principle XII (**Deprecation Lifecycle**).
  Pre-v1.0 (`0.x`): a `0.x.PATCH` release is bug-fixes-only and always safe to
  take; a `0.MINOR.0` bump MAY break (Go API surface OR machine-payload shapes
  like `ax.Error` / `__schema`, which are additive-tolerant); breaking changes
  ride the minor digit and never auto-promote to `1.0.0`. Deprecate an exported
  symbol with a `//Deprecated:` doc-comment paragraph carrying a migration note,
  let it ship in ≥1 published `0.MINOR.0` release (`staticcheck SA1019` flags
  call sites — already enabled), then remove it. See the constitution principles
  for the full policy.

## Development Workflow

1. Start by checking `git status --short` and reading the constitution
   (`.specify/memory/constitution.md`) plus any governing (frozen) ADRs.
2. Keep changes scoped to the requested behavior and the constitution-defined
   contract.
3. Prefer idiomatic, small Go packages. Avoid speculative abstractions.
4. Keep `README.md` and `examples/integration/` current with public API,
   command behavior, flags, output shapes, and roadmap changes. If a change
   affects how users should run or understand ax-go, update the docs and the
   integration example in the same change.
5. Run `gofmt` on Go changes.
6. Run `go test -race ./...` before handing work back. The race detector
   is REQUIRED, not optional — concurrent code is ubiquitous (OTel
   exporters, Loki push, ZeroLog hooks, idempotency).
7. Run `go vet ./...`, `golangci-lint run`, and `make doc-coverage`
   (ExampleXxx coverage on the primary API). All must be clean.
8. Use `testing.B` with `-benchmem` for allocation or hot-path performance
   claims. Do not assert numeric performance targets without a benchmark.

### Changelog & Releases

- **Never hand-edit or create `CHANGELOG.md`.** It is strictly managed by
  release-please from Conventional Commit history. Manual edits will be
  overwritten and create release-note conflicts. This overrides any general
  "always update the changelog" guidance from global agent instructions.
- Capture user-facing changes in the commit message (Conventional Commits:
  `feat:`, `fix:`, `feat!:` / `BREAKING CHANGE:`), not in the changelog file.
- The release flow (versioning, tags, `CHANGELOG.md`) is owned by
  release-please via `release-please-config.json` and
  `.release-please-manifest.json`. Do not duplicate it manually.

## Testing-First Discipline

**Tests land before implementation.** For every new behavior or
exported function, write the test that asserts the contract first and
verify it fails for the right reason. The implementation then makes it
pass without weakening the assertion. Bug fixes start with a failing
regression test.

Test forms and when to use each:

- **Table-driven tests** are the default for any function with more
  than one input shape. Standard form: `[]struct{}` cases, a `for`
  loop, `t.Run(tc.name, ...)`. Skip table-driven only when a single
  happy path is genuinely all that exists.
- **`ExampleXxx` functions** are how agents learn the API — they
  compile, run, and appear in godoc, the highest-leverage doc artifact
  Go offers. Coverage is two-tier:
  - *Required and gated:* a verified `ExampleXxx` on the **primary API
    surface** (constructors, the core exported types, and top-level
    entry points). `make doc-coverage` (`internal/cmd/doccover`)
    enforces this in CI, ratcheted through `baseline.txt` so it can
    never silently regress.
  - *Encouraged, not gated:* examples for other exported symbols where
    they add clarity. `WithX` functional options are demonstrated
    **inside** a parent example, not gated individually.
  Doc-comment *presence* is a separate, stricter rule and stays at
  100%: every exported symbol MUST carry a doc comment, enforced by
  `golangci-lint` (`godoclint`'s `require-doc`).
- **Golden-file tests** for `__schema` output, the `ax.Error` envelope
  JSON shape, and any output that is stable-by-contract. Schema
  stability is part of the public API.
- **Fuzz tests** (`func FuzzXxx(f *testing.F)`) for every parser
  surface: Hujson input, idempotency-key validation, error-envelope
  round-trip, `TRACEPARENT` extraction.
- **`testing.B` benchmarks with `-benchmem`** for any allocation or
  hot-path performance claim. Do not assert numeric targets without a
  benchmark.

Test surfaces every change must exercise (where applicable):

- stdout/stderr separation — nothing leaks to `stdout` that isn't the
  documented payload
- exit-code mapping
- `ax.Error` envelope shape (golden file)
- `__schema` output stability (golden file)
- agent-mode precedence (flag > env > TTY-detect)
- Hujson input parsing AND strict JSON output (round-trip + the
  read-Hujson/write-JSON asymmetry)
- `--dry-run` side-effect suppression
- idempotency-key generation, propagation, and envelope surfacing
- OTel trace correlation in logs (`trace_id` and `span_id` on every
  line when a span is active)
- Output determinism (same input → byte-identical envelope modulo
  documented non-deterministic fields)

## Documentation Discipline (Contracts, Not Narration)

Coding agents read doc comments as much as humans do, and an agent
working in the package learns the contract from them. Write doc comments
as contracts, not narration.

- **Document the contract, not the code.** State inputs, outputs, the
  error a function returns and the **exit code** it maps to, invariants,
  units, and fail-closed semantics. Mirror the stable `error_code` style
  the specs use (e.g. FR-007 / SC-005 of the bounded-config feature).
- **Never write "what" comments that restate the code.** A comment that
  narrates the line above it (`// increment i`) is noise on a good day
  and a lie after the next refactor. Comment the **why**: rationale,
  constraints, the non-obvious.
- **Prefer verified docs over prose; treat drift as a defect.** An
  `ExampleXxx` compiles and (with `// Output:`) is executed by
  `go test`, so it cannot silently drift; a golden file breaks on
  change; a type pins a shape. When a fact can be pinned by an example,
  a golden test, or a type, prefer that over a sentence.
- **Presence is gated; quality is on you.** `godoclint`'s `require-doc`
  gates that every exported symbol HAS a doc comment. It cannot tell a
  contract from narration — that is the reviewer's job.

## Go Discipline

Conventions LLM agents frequently slip on. Every implementation
follows them.

- **`context.Context` first parameter** on every function that does
  I/O, makes outbound calls, runs goroutines, or could be canceled.
  No "convenience" overloads without context.
- **Wrap errors with `%w`**, never `%s` or `%v`.
  `fmt.Errorf("loading config: %w", err)` preserves the chain;
  `fmt.Errorf("%s", err)` destroys it. `errors.Is` and `errors.As`
  MUST work against every error returned from this library.
- **No `panic` in library code.** Return errors. `panic` is reserved
  for programmer-error invariants that can never legitimately occur —
  never for user input, network failures, or missing resources.
- **Avoid `any` / `interface{}`.** Prefer a concrete type or a
  tightly scoped interface. When `any` is genuinely needed (decoding
  a flexible shape, for example), comment why at the use site.
- **No mutable package-level state.** No global maps, slices, or
  counters. No `init()` that mutates globals, reads files, makes
  network calls, or reads the environment for runtime behavior.
  Configuration enters via constructors.
- **Minimal dependencies.** Every new dep is maintenance cost and a
  future migration target. Before adding one: confirm the stdlib
  doesn't cover it, the existing dep set doesn't, and the added
  function is non-trivial to write inline. Justify the addition in
  the PR description.
- **`defer Close()` for every `io.Closer`.** Never let one leak. If
  `Close` returns an error that matters, log it via the canonical
  logger; never silently discard.
- **Functional options** for constructors with more than 2-3 config
  knobs: `ax.NewLogger(ctx, ax.WithLevel(...), ax.WithSink(...))`.
  Avoids "config struct vs. positional arg" churn as the API grows.
- **`internal/` packages** for anything consumers shouldn't import.
  The Go toolchain enforces it; use it freely.
- **Version injection at build.** Binaries built from ax-go-based
  CLIs inject a real version into `__schema`'s `version` field via
  `-ldflags "-X ..."`. Never ship `dev` or `unknown` to production
  agents.

## Guardrails For Agents

- Never write logs, warnings, progress, or prompts to `stdout`.
- Never relax machine-readable output guarantees for human convenience.
- Never use trace IDs as resource IDs or resource IDs as trace IDs.
- **Never log PII, secrets, tokens, or credentials** — not in Loki labels,
  not in JSON payload. The cardinality split (Constitution Principle VIII)
  governs index
  performance; the no-PII rule is privacy/security and applies to all log
  output regardless.
- **Never compose log messages from un-sanitized user-controlled strings.**
  Control characters (newlines, ANSI escapes) in user input can forge log
  entries. Use ZeroLog's field methods (`.Str`, `.Int`, etc.) which escape
  correctly; never `.Msg(fmt.Sprintf(...userInput...))`.
- **Never skip TLS verification** on outbound HTTP/gRPC calls. The `ax`
  helpers (`ax.HTTPClient`, `ax.GRPCDial`) ship secure defaults; do not
  pass `InsecureSkipVerify: true` or equivalents.
- **Never read unbounded user input.** Hujson config files MUST be
  size-capped at the read boundary (default 1 MiB, configurable). An
  oversized config is a validation error, not an OOM.
- Public API or runtime-behavior changes are specified through the Spec Kit
  feature workflow (GitHub issue → spec → plan → tasks), NOT new ADRs — ADRs are
  frozen. Record the decision in the feature's `research.md` and, when
  cross-cutting, amend the constitution. Even a renamed exported identifier is a
  breaking change.
- Keep `GEMINI.md` and `CLAUDE.md` as thin imports of this file so all
  agent guidance stays synchronized.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
at specs/010-import-isolated-contracts/plan.md
<!-- SPECKIT END -->
