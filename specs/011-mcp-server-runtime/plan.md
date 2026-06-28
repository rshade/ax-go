# Implementation Plan: ax-go mcp-server runnable wrapper

**Branch**: `011-mcp-server-runtime` | **Date**: 2026-06-26 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/011-mcp-server-runtime/spec.md`

## Summary

Make any ax-go CLI runnable as a **live MCP server** with no per-tool work, building on
the already-public `schema.BuildMCPSchema` discovery adapter. The server answers the MCP
`initialize` handshake and `tools/list` (discovery parity with `__schema --as=mcp`) and
executes `tools/call` by dispatching back into the existing Cobra command tree, returning
the command's verbatim `stdout` bytes as the tool result and the `ax.Error` envelope on
failure. It runs over stdio (default) and a streamable HTTP transport (loopback by
default, fail-closed against accidental public exposure). The protocol engine is the
official MCP Go SDK; ax-go's code stays a thin mapping layer delivered as its own public
package `mcp/` over an `internal/mcpserver/` engine, leaving the root `ax` facade
unchanged.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: Existing set (`cobra`, `pflag`, OpenTelemetry SDK, `zerolog`,
`google/uuid`) **plus one new direct dependency**:
`github.com/modelcontextprotocol/go-sdk` (v1.x GA, pin a tagged version; research D1).
Reuses `github.com/rshade/ax-go/schema` (`BuildMCPSchema`) and
`github.com/rshade/ax-go/contract` (exit codes, error envelope).

**Storage**: N/A — protocol bridge, no persistent state (data-model.md).

**Testing**: `go test -race ./...`, `go vet ./...`, `golangci-lint run`,
`make doc-coverage`; golden file for `tools/list`; in-memory client↔server integration
tests (stdio + HTTP); concurrency (`-race`) test; fuzz for argument decoding and trace
extraction; import-isolation test for the new `mcp` package.

**Target Platform**: Go library consumers on the platforms ax-go/CI already support.

**Project Type**: Go library — new thin public package `mcp/` + `internal/mcpserver/`
engine; runnable form is a mountable Cobra subcommand.

**Performance Goals**: No numeric targets asserted (thin bridge; per-command perf is the
adopting CLI's). If any allocation/hot-path claim is made later, back it with `testing.B`
`-benchmem` (Principle VII). Concurrency must be `-race`-clean.

**Constraints**:

- Stream separation: protocol I/O on the transport channel; logs/diagnostics on `stderr`;
  no non-protocol bytes on the protocol channel (FR-013/FR-014).
- HTTP binds loopback by default; non-loopback requires explicit opt-in (FR-016/FR-018).
- Served calls always resolve to machine/JSON mode (FR-026).
- Determinism: `tools/list` byte-identical across runs (golden-guarded).
- The SDK dependency is confined to `internal/mcpserver/` + the `mcp` package; the
  import-isolated contract packages (`contract`, `config`, `schema`, `id`) MUST NOT
  import `mcp` or the SDK.
- Root `ax` gains no new exported symbols.

**Scale/Scope**: One new public package (`mcp`), one new internal package
(`internal/mcpserver`), an apidiff-allowlist update, an `examples/integration` mount, a
golden fixture, README/quickstart docs, and the full test matrix above.

**Governing ADR(s)**: **N/A.** Issue #10 cites ADR-0003, but it was already absorbed into
`specs/010-import-isolated-contracts/research.md` and retired in feature 010. No ADR to
absorb or retire here (research.md "Decision Records Absorbed" = N/A).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Stream Separation | PASS | Protocol I/O on the transport channel only; command `stdout` buffered into the tool result, command `stderr`/server logs → `stderr`; SC-005 asserts no leak (C-12/C-13). |
| II. Deterministic Output & Exit Codes | PASS | `tools/list` golden-guarded; verbatim `stdout` bytes (no re-serialize); exit codes mapped into the `ax.Error` envelope; public shapes are structs (data-model). |
| III. Machine Discoverability via `__schema` | PASS | Reuses `schema.BuildMCPSchema`; live `tools/list` keeps parity with `__schema --as=mcp` (SC-002); golden file guards the shape. |
| IV. Agent-Safety Primitives | PASS | `--dry-run`/`--idempotency-key` flow through `tools/call`; mode forced to machine (FR-011/FR-026). |
| V. Asymmetric JSON I/O | PASS | Writes are strict JSON (verbatim payload bytes); config-reading commands keep the bounded-read cap; no Hujson on the protocol channel. |
| VI. ADR-Governed Scope — Library, Not Application | PASS | The server is the sanctioned single bridge ("a node an orchestrator composes; not itself an orchestrator"); no orchestration, auth flow, persistence, or domain logic added. |
| VII. Test-First Discipline | PASS | Tasks lead with failing golden/integration/import-isolation/example tests; `-race`, fuzz, and `make doc-coverage` are gates (research D10). |
| VIII. Observability & ID Discipline | PASS | Per-call span; W3C trace context continued when present; `trace_id`/`span_id` on log lines; telemetry flush on shutdown (FR-020/FR-025). |
| IX. Security & Resource Safety | PASS | Loopback-default HTTP (fail-closed); no TLS-verify disabling; no credentials held; panics recovered (no `panic` escape); errors wrapped with `%w`; bounded reads preserved. |
| X. Idiomatic Go & Dependency Minimalism | PASS (with justified dep) | One new dependency (MCP Go SDK) justified in research D1 (non-trivial protocol/transport, official + GA); `context.Context` first; functional options; no package-level state; SDK confined behind `internal/`. |
| XI. Stability & SemVer | PASS | New `mcp` public package is **additive** → pre-v1.0 minor (`0.MINOR.0`), `feat:`; apidiff allowlist updated (`internal/cmd/apidiff-verdict`) so `check-packages` and the diff gate stay correct; no `breaking-change-approved` label needed. |
| XII. Deprecation Lifecycle | PASS | No deprecations or removals. |

**ADR absorption gate (Constitution §Governance)**: PASS — Governing ADR(s) = N/A;
`research.md` records why (ADR-0003 retired in feature 010). No ADR-retirement task is
required in `tasks.md`.

**Post-design re-check**: PASS. Phase 1 artifacts (data-model, contracts, quickstart) keep
the feature additive, thin, and import-isolated; the only governance action is the
additive public-package allowlist update, which is sanctioned via this Spec Kit feature.

## Project Structure

### Documentation (this feature)

```text
specs/011-mcp-server-runtime/
├── plan.md              # This file
├── research.md          # Phase 0 — decisions (Decision Records Absorbed = N/A)
├── data-model.md        # Phase 1 — config + request/response shapes, invariants
├── quickstart.md        # Phase 1 — adopter usage (stdio + HTTP)
├── contracts/
│   ├── public-api.md    # `mcp` package public surface + apidiff allowlist
│   └── mcp-server.md    # protocol behavior contract (initialize/list/call/transports)
├── checklists/
│   └── requirements.md  # spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 — /speckit-tasks (NOT created here)
```

### Source Code (repository root)

```text
mcp/                        # NEW public package (thin entry points only)
├── server.go               # Serve(ctx, root, opts...)
├── command.go              # NewCommand(root, opts...) -> "mcp-server" cobra command
├── options.go              # Option, WithTransport, WithHTTPAddr, WithAllowNonLoopback, WithVersion
├── doc.go
├── server_test.go          # integration: initialize/list/call over stdio + HTTP
├── example_test.go         # ExampleServe / ExampleNewCommand (doc-coverage)
└── import_isolation_test.go

internal/mcpserver/         # NEW internal engine (SDK lives here)
├── server.go               # build SDK server, register tools from BuildMCPSchema
├── dispatch.go             # tools/call -> isolated command invocation, result/error mapping
├── transport.go            # stdio + streamable HTTP wiring, loopback guard
├── trace.go                # W3C trace-context extraction per call
├── dispatch_test.go        # table-driven: arg mapping, mode forcing, error mapping
├── transport_test.go       # loopback fail-closed; transport parity
├── concurrency_test.go     # -race: simultaneous HTTP calls
└── fuzz_test.go            # argument-object decode + trace extraction

schema/                     # reuse BuildMCPSchema (unchanged)
internal/mcp/               # existing adapter (unchanged)

internal/cmd/apidiff-verdict/main.go   # add "github.com/rshade/ax-go/mcp" to allowedPackages

testdata/
└── mcp_tools_list.golden.json         # NEW golden for tools/list parity + determinism

examples/integration/       # mount mcp.NewCommand(root); add server demo + README note
README.md                   # document `mcp-server` usage + Compatibility note if needed
```

**Structure Decision**: A new public package `mcp/` holds only the thin entry points; the
SDK and all protocol/transport/dispatch logic live in `internal/mcpserver/`. This mirrors
the repo's existing public/`internal` split (`schema/`↔`internal/schema/`,
`config/`↔`internal/config/`) and satisfies the maintainer's "own package, keep it thin"
directive without growing the root `ax` facade (FR-023/FR-024). The runnable binary form
is a Cobra subcommand mounted by the adopting CLI (and the `examples/integration` CLI),
which is the idiomatic MCP-subprocess launch model; a standalone `cmd/` launcher is
intentionally omitted to avoid a behavior-free placeholder (research D3).

## Complexity Tracking

No constitution violations requiring justification. The single notable addition — one new
public package and one new direct dependency — is handled within the rules, not against
them:

| Item | Why needed | Why the simpler path was not taken |
|------|------------|------------------------------------|
| New public `mcp` package | Adopting CLIs must launch the server over their own command tree; a public entry point is unavoidable | Putting it in root `ax` was explicitly rejected by the maintainer ("its own package, keep thin"); internal-only can't be imported by adopters |
| New dependency (MCP Go SDK) | stdio + streamable HTTP + handshake + capability negotiation is non-trivial and beyond stdlib (Principle X test) | Hand-rolling owns protocol correctness forever and contradicts thinness more than one official, GA, Google-maintained dependency; confined behind `internal/` |
| apidiff allowlist update | Required so `check-packages` + the diff gate recognize the new public package (Principle XI) | Not optional — CI fails otherwise; it is the sanctioned mechanism for adding a public package via a Spec Kit feature |
