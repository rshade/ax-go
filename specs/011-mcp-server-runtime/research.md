# Research: ax-go mcp-server runnable wrapper

**Feature**: 011-mcp-server-runtime | **Date**: 2026-06-26 | **Spec**: [spec.md](spec.md)

This document resolves the open decisions deferred from `/speckit-specify` and
`/speckit-clarify` (MCP protocol implementation, package placement, runnable form)
and records the rationale and rejected alternatives for each.

## Decision Records Absorbed

**N/A.** The issue (#10) cites ADR-0003 ("`__schema` Output Format"), but ADR-0003
was already absorbed into
[`specs/010-import-isolated-contracts/research.md`](../010-import-isolated-contracts/research.md)
and the ADR file was deleted as feature 010's final task. There is **no governing ADR
to absorb or retire** in this feature; ADR-0003 is referenced for context only. No new
ADRs are created (Constitution §Governance).

## D1: MCP protocol implementation — adopt the official Go SDK

**Decision**: Depend on `github.com/modelcontextprotocol/go-sdk/mcp` (v1.x GA; pin a
specific tagged version in `go.mod`, currently `v1.5.0`, 2026-04-07) for the protocol
engine, handshake, JSON-RPC framing, and both transports. ax-go code stays a thin
mapping layer: reflect the Cobra command tree into SDK tool registrations and dispatch
each call back into the command tree.

**Rationale**:

- The user-selected scope is **stdio + streamable HTTP** (clarify Q1). Hand-rolling
  JSON-RPC 2.0, the `initialize` handshake, capability negotiation, and *streamable
  HTTP* correctly is exactly the "non-trivial to write inline" case Constitution
  Principle X says justifies a dependency (stdlib first, but this is well beyond stdlib).
- The SDK is **official, GA (v1.0.0), and maintained in collaboration with Google** —
  low abandonment risk and a clear upgrade path as the MCP wire revision evolves.
- The SDK already models **tool failures as `CallToolResult{IsError: true}` rather than
  JSON-RPC protocol errors** (v1.5.0 release note), which is a direct match for FR-009 /
  FR-012 ("return a structured tool error, keep serving") — we get the desired
  fail-soft semantics without inventing them.
- It keeps ax-go *thin*: our code is command-tree → tool registration + a dispatch
  handler, not a protocol stack.

**Alternatives considered**:

- **Hand-roll with stdlib only** (`encoding/json`, `net/http`): zero new dependencies,
  but ax-go would own protocol correctness, capability negotiation, and streamable-HTTP
  semantics forever. Rejected: large, error-prone surface for a "brake, not engine"
  library; contradicts the thinness goal more than one well-scoped dependency does.
- **Hand-roll stdio only, defer HTTP**: smallest hand-rolled surface, but reopens the
  settled stdio+HTTP scope decision and still leaves us owning the protocol. Rejected:
  the user kept HTTP in scope.

**Consequences**:

- One new direct dependency (`github.com/modelcontextprotocol/go-sdk`) plus its
  transitive set; justified in the PR description per Principle X.
- The targeted MCP wire revision is whatever the pinned SDK version implements; we do
  not pin the revision independently (D11).
- The dependency is confined to `internal/mcpserver/` and the thin public `mcp` package;
  the import-isolated contract packages (`contract`, `config`, `schema`, `id`) MUST NOT
  import it (enforced by their existing import-isolation tests).

## D2: Package placement — new public `mcp/` package + `internal/mcpserver/` engine

**Decision**: Deliver the feature as its **own dedicated public package** `mcp/` at the
module root, holding only the thin entry points (`Serve`, `NewCommand`, functional
options, the transport selector). All protocol/transport/dispatch mechanics live in a
new `internal/mcpserver/` package. The root `ax` facade gains **no new exported
symbols** (FR-023: "at most one" — we use zero).

**Rationale**:

- Honors the maintainer directive ("its own package, keep it thin") and FR-023/FR-024:
  the server is not absorbed into root `ax`, and the public surface is the minimal
  `mcp.Serve` / `mcp.NewCommand` entry points.
- Mirrors the established split already in the repo: public `schema/` package over
  `internal/schema/`, public `config/` over `internal/config/`. A public `mcp/` over
  `internal/mcpserver/` is the same pattern. (The existing `internal/mcp/` adapter —
  `Build` — stays as-is; the new engine is named `internal/mcpserver/` to avoid
  collision.)
- Keeps the SDK dependency and protocol code behind `internal/`, so the public surface
  is a stable, thin contract.

**Alternatives considered**:

- **Root `ax.ServeMCP(...)` + `internal/mcpserver`** (no new public package): avoids an
  apidiff-allowlist change, but grows the root facade and reads as "crammed into root,"
  which the maintainer explicitly rejected. Rejected.
- **Internal-only, no public entry point**: impossible — adopting CLIs must be able to
  launch the server over their own command tree. Rejected.

**Consequences**:

- `github.com/rshade/ax-go/mcp` becomes a **new public package**. The apidiff allowlist
  (`allowedPackages` in `internal/cmd/apidiff-verdict/main.go`) and the `check-packages`
  guard MUST be updated to include it, or CI fails (AGENTS.md, Constitution Principle
  XI). This is a sanctioned Spec-Kit-feature addition.
- The `mcp` package needs its own `import_isolation_test.go` asserting it does not pull
  the contract packages into a runtime graph they must stay free of, and that the
  contract packages do not import `mcp`.

## D3: Runnable form — opt-in Cobra subcommand, not auto-mounted

**Decision**: The runnable server is exposed as a Cobra subcommand (`mcp-server`) that an
adopting CLI mounts explicitly via `mcp.NewCommand(root, opts...)`. It is **not**
auto-mounted by `ax.Execute()` (unlike `__schema`). The canonical runnable instance in
this repo is the `examples/integration` CLI, which mounts the command to demonstrate
`integration mcp-server`.

**Rationale**:

- `__schema` is read-only and safe to auto-mount everywhere; `mcp-server` **executes
  commands** and opens a transport, so making every ax-go CLI a server by default is a
  surprising, security-relevant side effect. Opt-in is the "brake" choice.
- The idiomatic MCP launch model is a client spawning `mycli mcp-server` as a subprocess
  — i.e., the binary IS the adopting CLI with the mounted subcommand. No separate ax-go
  binary is required.
- Keeps `ax.Execute()` unchanged and thin.

**Alternatives considered**:

- **Auto-mount in `Execute()` like `__schema`**: zero per-CLI work, but turns every
  ax-go CLI into a command-executing server implicitly. Rejected on safety grounds.
- **Standalone `cmd/` launcher binary**: AGENTS.md reserves `cmd/` for a future
  `ax-go mcp-server`, but a generic launcher has no command tree of its own and would be
  a behavior-free placeholder — which the constitution forbids ("do not create
  placeholder commands before behavior exists"). The runnable binary form is realized by
  any adopting CLI's mounted subcommand (demonstrated by `examples/integration`).
  Rejected for now; remains available as a future additive change if a concrete generic
  launcher behavior emerges. This refines spec FR-023's parenthetical ("binary in the
  reserved `cmd/` location"): the binary is the adopting CLI, not a separate ax-go
  command.

## D4: Tool execution model — isolated in-process re-dispatch, always machine mode

**Decision**: On `tools/call`, map the MCP tool input (a JSON object of flag/arg values)
onto an **isolated invocation** of the named command through the same Cobra root,
capturing that invocation's `stdout` into a buffer. Each call constructs its own
arguments, output buffers, and `context.Context`; no mutable state is shared between
concurrent calls. Output mode is forced to machine/JSON (agent) mode for every served
call (FR-026); a `--format=human` value in tool args is normalized to machine mode.

**Rationale**:

- "No per-tool work" requires reusing the existing command tree and its flag parsing
  rather than re-declaring tool inputs.
- Cobra commands hold flag state on shared `*cobra.Command` objects; concurrent HTTP
  calls demand per-call isolation (FR-021) so flag values from one call cannot leak into
  another. The dispatch path must run `-race`-clean.
- Forcing machine mode keeps the tool result the machine payload (there is no TTY, and
  the consumer is always an agent).

**Alternatives considered**:

- **Shell out to the CLI binary per call** (`os/exec`): perfectly isolated, but slow,
  loses in-process context/telemetry propagation, and needs the binary path. Rejected
  for the in-process default; could be a future transport-agnostic option.
- **Reuse a single shared command invocation**: simplest, but not race-safe under
  concurrent HTTP. Rejected.

## D5: Result & error mapping

**Decision**:

- **Success** (exit 0): return a `CallToolResult` whose content is a single text block
  containing the command's **verbatim `stdout` bytes** (clarify Q3) — no re-parse /
  re-serialize, preserving byte-identical determinism.
- **Failure** (non-zero exit): return `CallToolResult{IsError: true}` whose content
  carries the `ax.Error` envelope JSON (`error_code`, `message`, exit-code mapping).
  The server keeps serving (FR-009/FR-010).
- A panic in an adopting command is recovered at the dispatch boundary and converted to
  an `IsError` result (internal_error, exit 1), never crashing the server.

**Rationale**: Matches the SDK's tool-error model (errors as results, not protocol
errors), the determinism contract (Principle II), and the stream-separation contract
(command `stderr` → server `stderr` logs; command `stdout` → buffered → tool result,
never onto the protocol channel — FR-013/FR-014).

**Alternatives considered**: returning JSON-RPC protocol errors for command failures —
rejected because it conflates protocol-level failures with command-level failures and
tends to drop the session.

## D6: Transports & bind safety

**Decision**: stdio transport (`&mcp.StdioTransport{}`) is the default. The streamable
HTTP transport (`mcp.StreamableHTTPHandler`) binds to **loopback (`127.0.0.1`) by
default**; a non-loopback/public bind requires an explicit operator opt-in (clarify Q1;
FR-016/FR-018). Secure transport defaults only; never disable TLS verification. ax-go
holds no credentials and implements no auth flow — authn/authz for an exposed HTTP
endpoint is the deployment's responsibility.

**Rationale**: Fail-closed against accidental network exposure of a command-executing
server, while still ceding real auth to the deployment per the constitution.

## D7: Observability — per-call span with trace propagation

**Decision**: Each tool call is its own span. The dispatch handler extracts W3C trace
context from the incoming request's metadata when present (continuing the caller's
trace) and otherwise starts a fresh root trace; `trace_id`/`span_id` appear on every log
line emitted while serving the call (FR-025, Principle VIII). Telemetry is flushed on
server shutdown (FR-020), reusing the existing `StartTelemetry`/shutdown lifecycle.

**Rationale**: Honors W3C propagation "by default," gives agents end-to-end traces, and
degrades gracefully when the client sends no trace context. Reuses the existing
`TRACEPARENT`-extraction infrastructure and its fuzz-test pattern.

## D8: Discovery parity & tool selection

**Decision**: `tools/list` is derived from `schema.BuildMCPSchema(root)` (the existing,
already-public adapter). Hidden commands and the reserved commands (`__schema` and the
`mcp-server` command itself) are excluded from the callable tool set. The `tools/list`
output for a fixed command tree is guarded by a **golden file** (Principle II/III).

**Rationale**: Reuses the single source of truth (issue acceptance criterion), keeps the
live server and static `__schema --as=mcp` in lock-step (SC-002), and prevents exposing
the server-launcher or schema command as callable tools.

## D9: Streaming boundary

**Decision**: Buffer the full command output and return it as one tool result;
incremental MCP streaming/progress notifications are out of scope for the MVP (clarify
Q2). Documented limitation: extremely large/unbounded streaming commands are buffered,
not chunked.

**Rationale**: Matches MCP's request/response shape and keeps the bridge thin. Incremental
streaming is a clean future addition.

## D10: Testing strategy

Test-first, per Constitution Principle VII:

- **Golden file** for `tools/list` over a fixed example command tree (schema stability).
- **Table-driven** unit tests for argument mapping, result/error mapping, mode forcing,
  reserved/hidden exclusion.
- **Integration tests** driving an in-memory client↔server pair through
  `initialize → tools/list → tools/call` (success and failure) over the stdio transport,
  and a parallel HTTP test asserting transport parity (SC-007).
- **`-race`** across all tests; a concurrency test issuing simultaneous HTTP tool calls
  (FR-021).
- **Fuzz** for argument-object decoding and trace-context extraction (parser surfaces).
- **`ExampleXxx`** on `mcp.Serve` and `mcp.NewCommand` (primary API; `make doc-coverage`).
- **Import-isolation test** for the new `mcp` package and re-run of contract-package
  isolation tests (they must not import `mcp`).
- A stream-separation assertion that no non-protocol bytes reach the protocol channel
  across a representative session (SC-005).

## D11: MCP protocol revision

**Decision**: Targeted wire revision = whatever the pinned SDK version implements; not
pinned independently. Record the SDK version in `go.mod` and `research.md`; upgrades ride
the normal dependency-update flow.

## Open items for `/speckit-tasks`

- Confirm the SDK's exact dynamic tool-registration entry point (non-generic
  `Server.AddTool` with a raw input schema + a handler receiving raw arguments) during
  implementation; the contracts describe the behavior, not the SDK's internal signature.
- Decide the precise flag/arg-mapping rules for positional args vs. flags when the MCP
  input object is flat (documented in `data-model.md`).
