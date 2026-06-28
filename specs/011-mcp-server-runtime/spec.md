# Feature Specification: ax-go mcp-server runnable wrapper

**Feature Branch**: `011-mcp-server-runtime`

**Created**: 2026-06-26

**Status**: Draft

**Input**: User description: "ax-go mcp-server runnable wrapper (ADR-0003) — wrap an
ax-go CLI as a live MCP server with no per-tool work, building on the existing
`__schema --as=mcp` adapter. Only static MCP schema emit exists today." (GitHub
issue #10)

## Clarifications

### Session 2026-06-26

- Q: What should the HTTP transport's default bind address be, given the server executes commands and ax-go cedes auth to the deployment? → A: Default to loopback (`127.0.0.1`) only; binding to a non-loopback/public address requires an explicit, deliberate operator opt-in.
- Q: How should streaming/NDJSON command output cross `tools/call`, given MCP is request/response? → A: Buffer the command's full stdout and return a single tool result; incremental MCP streaming/progress notifications are out of scope for the MVP.
- Q: What MCP content representation should a successful tool result use? → A: Return the command's exact `stdout` bytes as a single text content block (verbatim, no re-serialization), preserving byte-identical determinism.
- Q: How should trace context propagate across the MCP boundary for a tool call? → A: Extract W3C trace context from incoming request metadata when present (continue the caller's trace), else start a fresh root trace; each tool call is its own span with `trace_id`/`span_id` on log lines.
- Q: What output mode should served commands run in? → A: Always machine/JSON (agent) mode for every served tool call; any human-format request in tool args is normalized to machine mode (the MCP consumer is always an agent).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Run a CLI as a live MCP server and discover its tools (Priority: P1)

A developer who has built a CLI on ax-go wants an MCP-capable agent runtime to
discover that CLI's commands as callable tools without hand-authoring a tool
definition for each command. They launch the CLI in its MCP-server mode; an MCP
client connects, performs the protocol handshake, and asks for the tool list.
Every non-hidden command in the CLI's command tree appears as a tool — names,
descriptions, and input fields — derived automatically from the same tool-discovery
adapter that already powers `__schema --as=mcp`.

**Why this priority**: This is the smallest slice that delivers the issue's core
value ("expose the command tree as MCP tools … with no per-tool work") and is the
one behavior that distinguishes a *live* server from the existing one-shot static
emit. Discovery alone is a demonstrable, viable product: an agent can ground itself
in the CLI's capabilities over a live connection.

**Independent Test**: Launch the CLI's MCP-server mode, drive the handshake and a
tool-list request from an MCP client, and assert the returned tool set exactly
matches what the existing static MCP adapter produces for the same command tree.

**Acceptance Scenarios**:

1. **Given** a CLI with several commands and flags, **When** an MCP client connects
   and completes the protocol handshake, **Then** the server reports its identity,
   the injected build version, and its supported capabilities.
2. **Given** the connected client requests the tool list, **When** the server
   responds, **Then** every non-hidden, flag-satisfiable command is present as a tool with
   the same name, description, and input fields the static MCP adapter would emit
   (commands that require positional arguments are excluded — see research D12).
3. **Given** a CLI that later adds a new command or flag, **When** the server is run
   again, **Then** the new command/flag appears in the tool list with no additional
   per-tool authoring.
4. **Given** reserved/internal commands (the schema command and the server-launcher
   command itself), **When** the tool list is requested, **Then** those reserved
   commands are not exposed as callable tools.

---

### User Story 2 - Invoke a command through the server and get its machine payload (Priority: P2)

An agent orchestrator, having discovered the tools, invokes one of them with
arguments. The server runs the corresponding command, and the command's final
machine payload is returned as the tool result. If the command fails, the agent
receives a structured error (the standard ax error envelope, including the mapped
exit code) instead of a crashed connection, and the server stays available for the
next call.

**Why this priority**: Execution is what makes the server "runnable" rather than a
live mirror of static discovery. It depends on US1 (a tool must be discoverable
before it can be called) but adds the round-trip that lets an agent actually *use*
the CLI.

**Independent Test**: With the server running, call a known tool with valid
arguments and assert the result equals that command's stdout payload; then call a
tool whose command exits non-zero and assert the client receives a structured error
carrying the ax error envelope while the server continues to serve subsequent calls.

**Acceptance Scenarios**:

1. **Given** a discoverable tool, **When** the client calls it with valid arguments,
   **Then** the server executes the matching command and returns that command's
   `stdout` machine payload as the tool result, with nothing else mixed in.
2. **Given** a tool call whose command exits non-zero, **When** the server responds,
   **Then** the result is a structured tool error carrying the ax error envelope
   (`error_code`, `message`, and the exit-code mapping), and the server remains
   running.
3. **Given** a tool call that includes the dry-run safety input, **When** the command
   runs, **Then** it produces its envelope with the dry-run marker set and causes no
   side effects.
4. **Given** a tool call that includes an idempotency key (or omits it), **When** the
   command runs, **Then** the resolved key is surfaced in the result envelope, matching
   the CLI's own behavior.
5. **Given** a call to a tool name that does not exist, or with malformed arguments,
   **When** the server responds, **Then** it returns a validation-style tool error and
   keeps serving; it does not terminate.

---

### User Story 3 - Serve the same tools over an HTTP transport (Priority: P3)

An operator wants to host the CLI's MCP surface as a network endpoint rather than a
locally launched subprocess. They start the server in its HTTP transport mode bound
to a configured address; an MCP client connects over HTTP and gets the identical
tool set and execution behavior available over the local (stdio) transport.

**Why this priority**: HTTP broadens deployment options but is not required for the
core "wrap a CLI as an MCP node" value, which the local transport already delivers.
It also carries the largest surface and the clearest external boundary (network
exposure, transport security), so it is sequenced last.

**Independent Test**: Start the server in HTTP mode on a test address, connect an MCP
client over HTTP, and assert the handshake, tool list, and a representative tool call
behave identically to the local transport.

**Acceptance Scenarios**:

1. **Given** the server started in HTTP mode on a configured address, **When** an MCP
   client connects over HTTP, **Then** the handshake, tool list, and tool-call
   behavior are identical to the local transport.
2. **Given** the HTTP transport, **When** secure transport is required, **Then** the
   server uses secure defaults and never silently disables transport verification.
3. **Given** the operator selects a transport, **When** the server starts, **Then**
   the chosen transport (local or HTTP) and its bind address are explicit and
   discoverable, and the local transport is the default when none is chosen.

---

### Edge Cases

- **Reserved commands**: the schema command and the server-launcher command must not
  appear as callable tools; hidden commands are excluded from the tool list.
- **Positional-argument commands**: a command that requires positional arguments (its
  Cobra `Args` validator rejects an empty argument vector) is excluded from the tool list,
  because the flat MCP argument object maps only onto flags and such a tool could never be
  satisfied (research D12); positional-argument mapping is deferred.
- **Unknown tool / bad arguments**: an unknown tool name or arguments that fail
  validation return a structured tool error (validation exit-code mapping) without
  crashing the server.
- **Command failure isolation**: any single tool call that fails (non-zero exit, panic
  in the adopting command, or a command that times out — which surfaces as a non-zero
  exit `3`, since the server imposes no per-call deadline of its own in the MVP) must not
  bring down the server; subsequent calls still succeed.
- **Stream-channel integrity**: a command that itself writes to its output stream
  must have that output captured as the tool result, never interleaved onto the
  server's protocol channel, so the protocol stream stays parseable.
- **Streaming / unbounded results**: a command that emits a streaming (NDJSON) or
  large result has its full output buffered and returned as a single tool result;
  incremental MCP streaming/progress notifications are out of scope for the MVP. This
  boundary is documented; the buffered output must never corrupt the protocol stream.
- **Input-reading commands**: commands that read configuration or input must respect
  the existing bounded-read size cap; an oversized input is a validation error, not an
  out-of-memory condition.
- **Cancellation / shutdown**: client disconnect, context cancellation, or a shutdown
  signal must stop the server cleanly, flush telemetry, and handle in-flight calls
  deterministically.
- **Concurrency**: when a transport allows simultaneous calls (HTTP), concurrent tool
  invocations must be race-free at the server boundary.
- **Version reporting**: the server must report the injected build version, never a
  placeholder such as `dev` or `unknown`.
- **HTTP exposure**: an HTTP endpoint fronted by no authentication is an operator
  responsibility; the server provides secure transport defaults but holds no
  credentials and implements no auth flow.

## Requirements *(mandatory)*

### Functional Requirements

#### Discovery (US1)

- **FR-001**: The system MUST provide a runnable MCP-server surface for an ax-go CLI
  that an MCP client can connect to and drive.
- **FR-002**: The server MUST derive its tool set from the CLI's existing command
  tree using the already-public MCP tool-discovery adapter (`BuildMCPSchema`), with no
  per-command or per-tool authoring required.
- **FR-003**: The server MUST implement the MCP protocol handshake, reporting server
  identity, the injected build version, and supported capabilities.
- **FR-004**: A tool-list request MUST return exactly the non-hidden, non-reserved,
  flag-satisfiable commands the static MCP adapter would emit for the same command tree —
  i.e. `schema.BuildMCPSchema(root)` minus hidden commands, the reserved `__schema` and
  `mcp-server` commands (FR-005), and commands that require positional arguments (research
  D12) — each with the same name, description, and input fields. (The static
  `__schema --as=mcp` adapter does not itself drop the reserved or positional-argument
  commands; the live server layers those exclusions on top. Those two carve-outs are the
  only sanctioned divergence from the static output.)
- **FR-005**: Reserved commands (the schema command and the server-launcher command
  itself), hidden commands, and commands that require positional arguments MUST NOT be
  exposed as callable tools. These four exclusions — hidden, `__schema`, `mcp-server`, and
  positional-argument-requiring commands (the last because the flat MCP argument object
  maps only onto flags; see research D12) — are the ONLY filtering the server applies on
  top of `schema.BuildMCPSchema`; every other command — including the root command and any
  parent/group commands — is projected exactly as the static adapter projects it,
  preserving discovery parity for callable commands (FR-004/SC-002). An adopting CLI that
  does not want a bare root or parent command exposed as a tool marks it hidden (the
  existing `Hidden` mechanism), rather than relying on server-side special-casing.
- **FR-006**: The exposed tool set MUST stay in sync with the CLI automatically —
  adding or changing a command or flag changes the exposed tools with no extra work.

#### Execution (US2)

- **FR-007**: The server MUST handle tool-call requests by dispatching to the named
  command and executing it through the same command tree, mapping the call's arguments
  to that command's flags and arguments.
- **FR-008**: On a successful call, the server MUST return the command's `stdout`
  machine payload as the tool result and MUST NOT mix any non-payload output into the
  result. The full payload is buffered and returned as a single tool result;
  incremental MCP streaming/progress notifications are out of scope for the MVP. The
  payload MUST be returned as the command's exact `stdout` bytes in a single text
  content block (verbatim, no re-parse/re-serialize), preserving byte-identical
  determinism.
- **FR-009**: On a non-zero exit, the server MUST return a structured tool error
  carrying the ax error envelope (`error_code`, `message`, and the exit-code mapping)
  and MUST keep serving subsequent requests.
- **FR-010**: A single tool call's failure (non-zero exit, panic in the adopting
  command, or timeout) MUST NOT terminate the server. The server does NOT impose its own
  per-call deadline in the MVP: a command that times out surfaces it as a non-zero exit
  (network/timeout → exit `3`) and rides the same non-zero-exit fail-soft path (FR-009);
  a panic is recovered at the dispatch boundary (internal_error → exit `1`). Either way
  the server keeps serving subsequent calls.
- **FR-011**: The server MUST honor agent-safety primitives on tool calls: a call MAY
  carry the dry-run input (producing the dry-run envelope with no side effects) and an
  idempotency key, and the resolved idempotency key MUST be surfaced in the result
  envelope, consistent with the CLI's own behavior.
- **FR-012**: An unknown tool name or arguments that fail validation MUST produce a
  structured tool error (validation exit-code mapping) without crashing the server.
- **FR-026**: Every served tool call MUST resolve to machine/JSON (agent) output mode
  regardless of TTY or environment; a human-format request carried in tool arguments
  MUST be normalized to machine mode so a tool result is always the machine payload.

#### Stream separation (US2)

- **FR-013**: MCP protocol messages MUST travel only on the designated protocol
  channel; logs, progress, and diagnostics MUST go to `stderr`. No non-protocol bytes
  may appear on the protocol channel.
- **FR-014**: Output a command writes during a tool call MUST be captured as that
  call's tool result and MUST NOT be written directly onto the server's protocol
  channel, so the protocol stream remains parseable end-to-end.

#### Transport (US1 + US3)

- **FR-015**: The server MUST support a local (stdio) transport and MUST use it as the
  default when no transport is selected.
- **FR-016**: The server MUST support a streamable HTTP transport bound to a
  configurable address, offering the identical tool set and execution behavior as the
  local transport. The default bind address MUST be loopback (`127.0.0.1`) only;
  binding to a non-loopback/public address MUST require an explicit, deliberate operator
  opt-in (fail-closed against accidental network exposure of a command-executing
  server).
- **FR-017**: Transport selection and any bind address MUST be explicit and
  discoverable to the operator.
- **FR-018**: Outbound/transport security MUST use secure defaults; the server MUST
  NOT silently disable transport verification. The server holds no credentials and
  implements no authentication flow; authentication/authorization for an HTTP endpoint
  is the adopting deployment's responsibility.

#### Packaging & scope (cross-cutting)

- **FR-023**: The runnable MCP-server surface MUST be delivered as its own dedicated,
  isolated package. Its runnable form is a Cobra subcommand (`mcp-server`) an adopting CLI
  mounts over its own command tree, so the runnable binary IS the adopting CLI — no
  standalone launcher binary in `cmd/` is created, because a generic launcher would have
  no command tree of its own and would be a behavior-free placeholder the constitution
  forbids (see research D3; a concrete `cmd/` launcher remains a future additive option).
  The surface MUST NOT be absorbed into the root `ax` facade, and it MUST NOT grow the
  root public API surface beyond, at most, a single minimal entry point an adopting CLI
  uses to launch the server over its own command tree.
- **FR-024**: The package MUST stay thin — a bridge that reflects the existing command
  tree and delegates to existing primitives (the `BuildMCPSchema` tool-discovery
  adapter and the established command-execution/lifecycle behaviors). It MUST NOT
  introduce orchestration, a second CLI framework, persistence, an auth flow, or other
  scope reserved to the adopting CLI.

#### Determinism, lifecycle & contract (cross-cutting)

- **FR-019**: Tool-list output for a given command tree MUST be deterministic
  (byte-identical across runs), modulo documented non-deterministic fields, and MUST be
  guarded as a stable-by-contract output.
- **FR-020**: The server MUST shut down cleanly on context cancellation or a shutdown
  signal, flushing telemetry on exit and handling in-flight calls deterministically: on
  shutdown it stops accepting new tool calls, lets in-flight calls observe the canceled
  `context.Context` (so they return promptly rather than being abandoned mid-write), and
  flushes telemetry before `Serve` returns `nil`. No in-flight call is left to corrupt the
  protocol channel during drain.
- **FR-021**: Concurrent tool calls (where the transport permits them) MUST be
  race-free at the server boundary.
- **FR-025**: Each tool call MUST be observable as its own span: the server MUST
  extract W3C trace context from the incoming request's metadata when present (continuing
  the caller's trace) and otherwise start a fresh root trace, and `trace_id`/`span_id`
  MUST appear on log lines emitted while serving that call (Principle VIII).
- **FR-022**: The feature MUST ship documented usage — README guidance and a runnable
  example demonstrating wrapping a CLI as an MCP server and connecting a client — kept
  in sync with the behavior.

### Key Entities *(include if feature involves data)*

- **MCP Server surface**: the runnable wrapper that hosts an ax-go CLI as an MCP
  endpoint; owns the handshake, tool list, tool-call dispatch, transport, and
  lifecycle.
- **MCP Tool**: a one-to-one projection of a single non-hidden, non-reserved,
  flag-satisfiable command (commands requiring positional arguments are excluded — see
  research D12), carrying the command's name, description, and input fields, sourced from
  the existing tool-discovery adapter.
- **Tool Call**: a client request naming a tool plus its arguments (which may include
  the dry-run and idempotency-key safety inputs).
- **Tool Result**: the response to a tool call — either the command's machine payload
  on success or a structured ax error envelope on failure.
- **Transport**: the channel carrying the MCP protocol (local/stdio by default, or
  HTTP), with the protocol channel kept separate from diagnostics.
- **Command tree**: the adopting CLI's set of commands and flags — the single source
  of truth from which tools are derived.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A developer can expose an existing ax-go CLI as a live MCP server whose
  tool list covers all of its non-hidden commands with zero per-command code (only
  enabling the server surface).
- **SC-002**: 100% of the non-hidden, non-reserved, flag-satisfiable commands present in
  the CLI's static `__schema --as=mcp` output are also discoverable through the live
  server's tool list (discovery parity). The live tool list equals that static set minus
  the reserved `__schema` and `mcp-server` commands and minus commands that require
  positional arguments (FR-005, research D12), which the static adapter does not itself
  drop; those two carve-outs are the only permitted difference.
- **SC-003**: An MCP client can complete handshake → tool list → tool call → receive
  the command's machine payload end-to-end over the default transport.
- **SC-004**: A failing command returns a structured error to the client and the
  server remains available for the next call — zero server terminations caused by
  command failures across a representative session.
- **SC-005**: Across a representative session, no non-protocol bytes ever appear on the
  protocol channel (verifiable by feeding the protocol channel into a strict protocol
  parser).
- **SC-006**: Tool-list output is byte-identical across repeated runs for the same
  command tree, modulo documented non-deterministic fields.
- **SC-007**: The identical tool set and tool-call behavior are observable over both
  the local (stdio) and HTTP transports.
- **SC-008**: The feature lives in its own dedicated package and adds at most one
  minimal public entry point to the root `ax` facade; reviewers can confirm the server
  logic is not embedded in the root package and that the package delegates to existing
  primitives rather than reimplementing discovery, execution, or lifecycle.

## Assumptions

- Source inputs: GitHub issue #10. Governing ADR(s): **none**. The issue cites
  ADR-0003 ("`__schema` Output Format"), but ADR-0003's decisions were already
  absorbed into `specs/010-import-isolated-contracts/research.md` and the ADR file was
  retired as feature 010's final task. There is therefore no ADR to absorb or retire
  in this feature; ADR-0003 is referenced for context only.
- This feature builds directly on the already-public `schema.BuildMCPSchema` (delivered
  by feature 010) as the single source of truth for tool discovery; it does not redefine
  the MCP tool shape. The new `mcp` package imports the import-isolated `schema` package
  directly (NOT the root `ax.BuildMCPSchema` re-export), to keep the server off the root
  runtime facade and preserve import isolation (see plan Constraints).
- **Server scope** (confirmed): a full server supporting both tool discovery
  (`tools/list`) and tool execution (`tools/call`), not discovery-only.
- **Transport** (confirmed): both the local (stdio) transport and a streamable HTTP
  transport are in scope; the local transport is the default/primary. HTTP
  authentication and authorization are ceded to the adopting deployment, consistent
  with the constitution's boundary that ax-go holds no credentials and implements no
  auth flow.
- **Safety posture** (confirmed): all non-hidden commands are callable; the server
  forwards the existing agent-safety primitives (dry-run, idempotency key) rather than
  inventing a new permission model, and faithfully maps exit codes to MCP tool errors.
  The server does not introduce a per-command allowlist; marking destructive commands
  remains the adopting CLI's responsibility.
- The server reuses the established `Execute()` lifecycle behaviors (idempotency,
  telemetry flush-on-exit) where applicable, so MCP-served commands behave like directly
  invoked ones — except that mode resolution is overridden to always machine/JSON mode
  for served calls (FR-026), since the MCP consumer is always an agent and there is no
  TTY.
- **Structure & thinness** (per maintainer direction): the feature is delivered as its
  own dedicated, thin package — a bridge, not a framework. It reflects the existing
  command tree and delegates to existing primitives; it does not bloat the root `ax`
  facade. Whether the package is public (`mcp/`-style, requiring an apidiff allowlist
  update in `internal/cmd/apidiff-verdict` and the `check-packages` guard) or an
  `internal/` implementation behind a minimal public entry point plus a `cmd/` binary is
  a planning decision deferred to `/speckit-plan`; either way the public surface grows by
  at most one minimal entry point.
- The exact MCP protocol revision targeted is pinned during planning; the spec assumes
  a current, widely supported revision.
- The constitution (supreme governance) governs scope: `mcp-server` is "a node an
  external orchestrator can compose; it is not itself an orchestrator." Orchestration,
  auth/identity, persistence, and domain logic remain out of scope and are delegated to
  the adopting CLI and the agent runtime.
