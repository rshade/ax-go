# Contract: MCP server protocol behavior

**Feature**: 011-mcp-server-runtime | **Spec**: [../spec.md](../spec.md)

Defines the observable behavior of the running server across the MCP lifecycle. The wire
framing is owned by `github.com/modelcontextprotocol/go-sdk` (research D1); these are the
ax-go-specific guarantees layered on top.

## Handshake (`initialize`)

- **C-1**: On `initialize`, the server reports its implementation name (root command
  name) and the injected build version (never `dev`/`unknown`), and advertises the
  `tools` capability (FR-003). An empty or placeholder version is rejected fail-closed at
  startup (validation error, exit 2) rather than reported.

## Discovery (`tools/list`)

- **C-2**: `tools/list` returns exactly the tools produced by `schema.BuildMCPSchema(root)`
  for the command tree, **minus** hidden commands, the reserved `__schema` and
  `mcp-server` commands, and commands that require positional arguments (FR-004/FR-005, D8,
  research D12). Those four are the only exclusions; the root command and any parent/group
  commands are projected exactly as `BuildMCPSchema` projects them (an adopter suppresses a
  bare root/parent by marking it `Hidden`).
- **C-3**: For a fixed command tree, `tools/list` is byte-identical across runs (modulo
  documented non-deterministic fields) and is guarded by a golden file:
  `testdata/mcp_tools_list.golden.json` (FR-019, SC-006).
- **C-4**: The live tool set equals the non-hidden, flag-satisfiable set emitted by
  `__schema --as=mcp` — that set minus commands that require positional arguments
  (discovery parity, SC-002, research D12).

## Execution (`tools/call`)

- **C-5**: A call dispatches to the named command via an isolated in-process invocation
  of the Cobra root, mapping `arguments` → flags/args (D4, data-model argument-mapping
  rules). Multi-value Cobra flags use JSON arrays in MCP arguments and are replaced per
  call, never appended from prior calls. JSON numbers are decoded without `float64`
  materialization so `int64`/`uint64` flag values retain their original decimal
  precision.
- **C-6** (success): exit 0 → `CallToolResult{IsError:false}` with one text content block
  = the command's **verbatim stdout bytes** (FR-008, clarify Q3).
- **C-7** (failure): non-zero exit → `CallToolResult{IsError:true}` with one text content
  block = the `ax.Error` envelope JSON (`error_code`, `message`, exit-code mapping). The
  server keeps serving (FR-009).
- **C-8** (unknown/invalid): unknown tool name or argument-validation failure →
  `IsError:true` with a validation envelope (exit 2 mapping); server keeps serving and
  still emits the per-call span/continued trace context when request metadata supplies
  W3C trace context (FR-012/FR-025).
- **C-9** (panic safety): a panic inside an adopting command is recovered at the dispatch
  boundary → `IsError:true` (internal_error, exit 1); the server never crashes (FR-010).
- **C-10** (agent-safety passthrough): `arguments.dry-run == true` runs the command with
  no side effects and `dry_run:true` in the payload; an `idempotency-key` argument (or
  auto-generated when absent) is surfaced in the result envelope (FR-011).
- **C-11** (mode): every served call resolves to machine/JSON (agent) mode regardless of
  TTY/env; a `format=human` argument is normalized to machine mode (FR-026).

## Stream separation

- **C-12**: MCP protocol bytes travel only on the transport channel (stdout for stdio);
  logs/progress/diagnostics go to stderr; no non-protocol bytes appear on the protocol
  channel — verified by feeding a representative session's protocol channel through a
  strict parser (FR-013, SC-005).
- **C-13**: A command's own stdout during a call is captured to a buffer and returned as
  the result; it is never written to the server's protocol channel (FR-014).

## Transports

- **C-14**: `stdio` is the default transport (`&mcp.StdioTransport{}`); a single session,
  requests handled sequentially (FR-015).
- **C-15**: `http` uses the SDK's `StreamableHTTPHandler`, binding **loopback by default**;
  a non-loopback bind requires explicit opt-in (`WithAllowNonLoopback`) or startup fails
  closed with a validation error (FR-016/FR-018, clarify Q1).
- **C-16**: Both transports expose the identical tool set and tool-call behavior
  (transport parity, SC-007).
- **C-17**: Secure transport defaults only; TLS verification is never disabled; the server
  holds no credentials and runs no auth flow (FR-018, Principle IX).

## Observability & lifecycle

- **C-18**: Each tool call is its own span; W3C trace context is extracted from the
  request when present (continuing the caller's trace) else a fresh root trace is started;
  `trace_id`/`span_id` appear on log lines emitted while serving the call (FR-025).
- **C-19**: On context cancellation/signal the server stops accepting new calls, lets
  in-flight calls observe the canceled `context.Context` and return promptly (never
  abandoned mid-write onto the protocol channel), and flushes telemetry before `Serve`
  returns `nil` (FR-020). A clean-shutdown test asserts `Serve` returns `nil` and telemetry
  is flushed; a concurrency-shutdown test asserts in-flight HTTP calls drain
  deterministically.
- **C-20**: Concurrent calls (HTTP) are race-free at the server boundary; the dispatch
  path shares no mutable command/flag state and runs `-race`-clean (FR-021).

## Streaming boundary

- **C-21**: A streaming/NDJSON command's full output is buffered and returned as one tool
  result; incremental MCP streaming/progress notifications are out of scope for the MVP
  (clarify Q2, FR-008).
