# Data Model: ax-go mcp-server runnable wrapper

**Feature**: 011-mcp-server-runtime | **Date**: 2026-06-26 | **Spec**: [spec.md](spec.md)

This feature is a protocol bridge, not a persistence feature: it has **no stored
state**. The "entities" below are in-memory configuration and request/response shapes
that cross the public `mcp` package boundary or the MCP wire. Public shapes use structs
(never maps) per Constitution Principle II; the MCP wire shapes are owned by the SDK and
listed here only to pin behavior.

## Entity: ServerOptions (public, `mcp` package)

Configuration assembled from functional options (`mcp.WithTransport`, `mcp.WithHTTPAddr`,
`mcp.WithVersion`, …). Per Principle X, functional options are used because there are
more than 2-3 knobs.

| Field | Type | Default | Validation / Notes |
|-------|------|---------|--------------------|
| `transport` | enum {`stdio`, `http`} | `stdio` | Selects the active transport (FR-015/FR-016). |
| `httpAddr` | string (host:port) | `127.0.0.1:0`-style loopback | Used only when `transport == http`. A non-loopback host MUST be an explicit opt-in (FR-016/FR-018); a loopback default is fail-closed. |
| `allowNonLoopback` | bool | `false` | Must be `true` for `httpAddr` to bind a non-loopback/public interface; otherwise startup is a validation error (exit 2). |
| `version` | string | injected build version | Reported in the MCP `initialize` handshake; never `dev`/`unknown`. An empty or placeholder (`dev`/`unknown`) version is rejected fail-closed at startup (validation error, exit 2), so adopters/tests MUST inject a real version via `-ldflags` or `WithVersion` (FR-003, Principle X). |
| `serverName` | string | root command `Name()` | Reported as the MCP server implementation name. |

**State transitions** (server lifecycle): `configured → listening → serving ⇄ (per-call)
→ draining → stopped`. Shutdown is triggered by context cancellation or signal; on
entering `draining` the server stops accepting new calls, in-flight calls observe the
canceled `context.Context` and return promptly (never abandoned mid-write onto the
protocol channel), and telemetry is flushed before `Serve` returns `nil` (FR-020, C-19).

## Entity: Tool (projection, derived — not stored)

A one-to-one projection of a non-hidden, non-reserved command, produced by
`schema.BuildMCPSchema(root)`. The feature does not define a new tool shape; it reuses
the existing `schema.MCPTool`.

| Field | Source | Notes |
|-------|--------|-------|
| `name` | command path | Unique per command tree; the tool identity. |
| `description` | command `Short` | May be empty. |
| `inputSchema` | command flags | JSON-schema object: `{type: object, properties: {<flag>: {type, description, default}}}`. Cobra multi-value flags are advertised as JSON arrays with an `items` schema, so MCP clients can preserve repeated values. |

**Exclusion rules** (FR-005, D8): hidden commands, the `__schema` command, and the
`mcp-server` command itself are NOT exposed as callable tools. These three are the ONLY
exclusions layered on top of `schema.BuildMCPSchema`; every other command — including the
root command and any parent/group commands — is projected exactly as the static adapter
projects it (an adopter suppresses a bare root/parent by marking it `Hidden`). Selection
parity with `__schema --as=mcp` (minus the reserved commands) is guarded by a golden file
(SC-002, SC-006).

## Entity: ToolCall (request, MCP wire — SDK-owned)

| Field | Type | Notes |
|-------|------|-------|
| `name` | string | Target tool; MUST match a discoverable tool, else a validation tool error (FR-012). |
| `arguments` | JSON object | Flag/arg values; mapped onto an isolated command invocation (D4). May include `dry-run` (bool) and `idempotency-key` (string) safety inputs (FR-011). A `format=human` value is normalized to machine mode (FR-026). |
| `_meta` (trace) | object | Optional W3C trace context; continued if present, else a fresh root trace is started (FR-025, D7). |

**Argument-mapping rules** (D4, refined in tasks):

- Each key in `arguments` maps to a command flag of the same name; scalar values are
  stringified per the flag's type before parsing.
- Cobra multi-value flags (`StringSlice`, `StringArray`, numeric slices, bool slices,
  duration/IP slices) accept JSON arrays as their MCP encoding. Each call replaces that
  flag's value for the isolated invocation; omitted multi-value flags are restored to
  their defaults, and values never append/leak across calls.
- JSON numbers are decoded with exact number preservation and passed to integer/unsigned
  flags as their original decimal text, so 64-bit IDs above `2^53` are not rounded.
- Unknown argument keys produce a validation tool error (exit 2 mapping), never silent
  drops.
- Each call builds its own argument vector and buffers; no shared mutable flag state
  across concurrent calls (FR-021).

## Entity: ToolResult (response, MCP wire — SDK-owned)

| Case | Shape | Notes |
|------|-------|-------|
| Success (exit 0) | `CallToolResult{ IsError: false, Content: [TextContent{ text: <verbatim stdout bytes> }] }` | Verbatim bytes; no re-serialization (FR-008, clarify Q3). For a dry-run call, the payload carries `dry_run: true`; the resolved `idempotency_key` is surfaced (FR-011). |
| Failure (exit ≠ 0) | `CallToolResult{ IsError: true, Content: [TextContent{ text: <ax.Error envelope JSON> }] }` | Envelope: `error_code`, `message`, plus the standard required fields; exit code mapped to category (FR-009). |
| Panic in command | recovered → `IsError: true` (internal_error, exit 1) | Never crashes the server (FR-010). |

## Relationships

```text
ServerOptions ──configures──▶ MCP Server (internal/mcpserver)
                                   │
                                   ├─ tools/list ──reflects──▶ schema.BuildMCPSchema(root) ──▶ [Tool]
                                   │
                                   └─ tools/call ──dispatches──▶ isolated command invocation (Cobra root)
                                                                       │
                                                                       ├─ stdout (buffered) ──▶ ToolResult.Content
                                                                       └─ non-zero exit ──▶ ax.Error ──▶ ToolResult(IsError)
Transport {stdio | http} ──carries──▶ MCP protocol  (logs/diagnostics ──▶ stderr, never the protocol channel)
```

## Invariants

- **INV-1** (stream separation): no non-protocol bytes ever reach the protocol channel;
  command `stdout` is captured to a buffer, command `stderr` goes to the server's `stderr`
  (FR-013/FR-014, SC-005).
- **INV-2** (determinism): `tools/list` is byte-identical across runs for a fixed command
  tree, modulo documented non-deterministic fields (FR-019, SC-006).
- **INV-3** (discovery parity): the live `tools/list` set equals the static
  `__schema --as=mcp` set minus the reserved `__schema`/`mcp-server` commands — i.e. the
  non-hidden, non-reserved set (FR-004/FR-005, SC-002). The static adapter does not drop
  the reserved commands; the server layers that single exclusion on top.
- **INV-4** (fail-soft): a single tool call's failure never terminates the server
  (FR-010, SC-004).
- **INV-5** (isolation): concurrent tool calls share no mutable command/flag state and
  run `-race`-clean (FR-021).
- **INV-6** (mode): every served call resolves to machine/JSON mode (FR-026).
