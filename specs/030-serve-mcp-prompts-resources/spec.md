# Feature Specification: Serve Declared MCP Prompts, Resources, and Instructions on the Live Server

**Feature Branch**: `030-serve-mcp-prompts-resources`

**Created**: 2026-10-07

**Status**: Draft

**Input**: GitHub issue rshade/ax-go#270, Phase 2 of #138 (Phase 1 is `specs/028-mcp-prompts-resources/`). v0.8.0 lets a CLI author declare prompts and static resources, and `__schema --as=mcp` projects them. The live `mcp-server` still registers tools only, so a declared prompt or resource is visible in the schema and invisible to every MCP client. The trigger spec 028 set for this phase has fired: `rshade/go-decide#22` wants its `decide` skill available the moment `go-decide mcp-server` starts. This feature serves declared prompts and resources on the live server, adds static content to the resource declaration, and adds a server-instructions option so the `initialize` handshake tells an agent where to look.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An agent reads a protocol document the CLI ships (Priority: P1)

A CLI author declares a resource whose static content is a long reference document (go-decide's `decide` skill is 174 lines, 6.6 KB of Markdown) and gives the server a short instructions string saying when to read it. An agent connects to `<cli> mcp-server`, sees the instructions at connect, reads the resource on its own, and learns how to drive the CLI's tools without the repository on disk.

**Why this priority**: This is the whole point of #270. Instructions are the pointer and resource content is the body; neither works alone. Without it, go-decide hand-rolls a tool that returns protocol text, which costs two schema version bumps and recreates the drift between live and static surfaces.

**Independent Test**: Declare one resource with content and set instructions that name it. Connect an in-memory MCP client: `initialize` carries the instructions, `resources/list` lists the resource, and `resources/read` returns exactly the declared content with the declared MIME type.

**Acceptance Scenarios**:

1. **Given** a CLI with a declared resource carrying content, **When** a client calls `resources/list`, **Then** it receives the same URIs, names, titles, descriptions, and MIME types as `__schema --as=mcp`'s `resources`, in the same order.
2. **Given** the same CLI, **When** a client calls `resources/read` with a declared URI, **Then** it receives the declared content and MIME type, byte for byte, on every call.
3. **Given** the same CLI, **When** a client calls `resources/read` with a URI that was not declared, **Then** it receives a protocol error and no content.
4. **Given** instructions set on the server, **When** a client sends `initialize`, **Then** the result's `instructions` field equals the configured text.

---

### User Story 2 - A user invokes a declared prompt from their MCP client (Priority: P1)

A CLI author declares a short workflow prompt with arguments, for example `decide` taking a `question`. In an MCP client such as Claude Code the prompt appears as a user-invoked entry (`/mcp__<server>__<prompt>`). The user supplies arguments and the client receives the rendered template as one user message.

**Why this priority**: Prompts are the user's manual entry point to a workflow. They are not something the model fetches, so they are a separate surface from resources and need their own correctness guarantees.

**Independent Test**: Declare a prompt with a required and an optional argument and a template referencing both plus literal brace text. Over an in-memory client, `prompts/list` matches the schema, and `prompts/get` returns the template with the placeholders substituted and the other braces untouched.

**Acceptance Scenarios**:

1. **Given** a CLI with declared prompts, **When** a client calls `prompts/list` over stdio or HTTP, **Then** it receives the same names, titles, descriptions, and arguments as `__schema --as=mcp`'s `prompts`, in the same order.
2. **Given** a declared prompt, **When** a client calls `prompts/get` with valid arguments, **Then** it receives a single user-role text message in which every declared `{{name}}` is replaced by that argument's value and all other brace text is unchanged.
3. **Given** a declared prompt, **When** a client calls `prompts/get` with a required argument missing, an argument the prompt does not declare, or an unknown prompt name, **Then** it receives an invalid-params protocol error and no rendered text, never a partial render.
4. **Given** an optional argument that is absent, **When** the prompt renders, **Then** its placeholder renders as empty text.
5. **Given** the same prompt and the same arguments, **When** `prompts/get` is called repeatedly, **Then** the responses are byte-identical.

---

### User Story 3 - Trees and servers that declare nothing are unchanged (Priority: P1)

Every existing adopter upgrades and sees no difference in what their server advertises.

**Why this priority**: The handshake and capability set are machine contract. Advertising a capability that has nothing behind it breaks clients that probe it, and any change here is invisible to the default test suite.

**Independent Test**: For a tree with no prompts, no resources, and no instructions, the `initialize` result and capability set are byte-identical to v0.8.0, and the existing `__schema` and MCP goldens pass without regeneration.

**Acceptance Scenarios**:

1. **Given** a CLI with no declared prompts, no declared resources, and no instructions, **When** a client sends `initialize`, **Then** the result is byte-identical to v0.8.0 and advertises neither the prompts nor the resources capability.
2. **Given** a CLI with prompts but no resources (or the reverse), **When** a client sends `initialize`, **Then** only the capability with something behind it is advertised.
3. **Given** declared resource content, **When** `__schema` and `__schema --as=mcp` run, **Then** their output is byte-identical to the output without content: content is never projected.

---

### User Story 4 - Server instructions are validated and fail closed (Priority: P2)

An adopting CLI passes a short instructions string to the server through an option. A malformed or oversized string is a configuration mistake the author must learn about at startup, not a string silently truncated or passed through to every client.

**Why this priority**: Instructions go into every connecting agent's context, so an unbounded or malformed string is a resource-safety problem (Principle IX), but the happy path works without validation.

**Independent Test**: Start the server with invalid UTF-8 and with text over the cap. Each fails at startup with exit `2` and an `ax.Error` envelope on `stderr`, with nothing on `stdout`.

**Acceptance Scenarios**:

1. **Given** instructions that are valid UTF-8 and within the cap, **When** the server starts, **Then** it starts normally.
2. **Given** instructions that are not valid UTF-8, **When** the server starts, **Then** it fails closed with exit `2` and writes nothing to `stdout`.
3. **Given** instructions over the size cap, **When** the server starts, **Then** it fails closed with exit `2` and writes nothing to `stdout`.

---

### User Story 5 - The feature works in a real client (Priority: P2)

A maintainer confirms the behavior end to end in Claude Code rather than trusting an in-memory SDK client alone.

**Why this priority**: The issue's stated reason to build this is how each surface reaches the model, which only a real client can show. It gates merging but cannot run in CI.

**Independent Test**: Add the integration example to Claude Code with `claude mcp add`, start a new session, and record the Claude Code version and the steps in the pull request.

**Acceptance Scenarios**:

1. **Given** the integration example added to Claude Code, **When** a new session starts, **Then** the server's instructions appear in the agent's context.
2. **Given** the same session, **When** the user types the prompt's slash command, **Then** it renders with its arguments.
3. **Given** the same session, **When** the user `@`-mentions the resource, **Then** it attaches.
4. **Given** only the instructions, **When** the model decides it needs the document, **Then** it reads the resource through `ReadMcpResource` with no `@` mention.

---

### Edge Cases

- **Hidden commands**: prompts and resources declared on a hidden command or in a hidden subtree are not served, exactly as `__schema` prunes them. The served set is the same aggregation `BuildMCPSchema` uses, so the two cannot diverge.
- **Commands excluded from tools**: a prompt or resource declared on a command excluded with `mcp.Exclude`, or on a non-runnable group, is still served, as in spec 028.
- **Duplicates**: a duplicate prompt name or resource URI across the tree already fails closed in `__schema` (spec 028). The live server applies the same rule at startup and does not pick one silently.
- **Resource without content**: a resource declared without content (valid under spec 028) is served by `resources/list` but `resources/read` returns an empty body with the declared MIME type. Whether a content-less resource should instead be omitted from the live server is settled in planning.
- **Brace text in templates**: text such as `{x}` or `{{ name }}` or `{{unknown}}` is literal at render time, exactly as spec 028 validates it.
- **Argument values containing placeholder syntax**: an argument value that itself contains `{{other}}` is inserted verbatim and never re-expanded, so rendering is a single pass.
- **Unicode and newlines**: argument values and templates with non-ASCII text or newlines render byte-exactly.
- **Oversized prompt arguments**: an argument value over the size bound is rejected with invalid-params rather than rendered.
- **Live-state requests**: a resource can never carry per-call or computed content. Content is fixed at declaration time and never a callback.
- **Protocol channel**: the instructions string, rendered prompts, and resource bodies travel only in the protocol channel. They are never written to `stdout` outside it, and may appear only in debug logs on `stderr`.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The live server MUST register every prompt from the same aggregation `schema.BuildMCPSchema` uses (the root plus every command not under a hidden child, deduplicated the same way), so `prompts/list` and `__schema --as=mcp`'s `prompts` list the same entries in the same order.
- **FR-002**: The live server MUST register prompts only when at least one exists, so a CLI with no prompts advertises exactly the capabilities it does today.
- **FR-003**: `prompts/get` MUST return exactly one user-role text message in which each `{{name}}` that names a declared argument is replaced by that argument's value. All other brace text MUST remain literal. Substitution MUST be a single pass: values are never re-scanned.
- **FR-004**: `prompts/get` MUST reject, with an invalid-params protocol error and before rendering, a missing required argument, an argument the prompt does not declare, and an unknown prompt name. It MUST never return a partial render. This matches the strictness `tools/call` applies to unknown arguments.
- **FR-005**: An absent optional argument MUST render as empty text. The declaration documentation MUST say so.
- **FR-006**: Rendering MUST be deterministic: the same template and arguments produce identical bytes (Principle II).
- **FR-007**: The resource declaration MUST gain an additive static content field carrying text. It is set at declaration time, captured by value, and is never a callback, function, or reference to live state (Principle VI). Whether binary (blob) content is also supported is decided in planning; the default is text only.
- **FR-008**: The live server MUST register every declared resource from the same aggregation, only when at least one exists. `resources/list` MUST match `__schema --as=mcp`'s `resources`, and `resources/read` MUST return the declared content with the declared MIME type. An unknown URI MUST be a protocol error.
- **FR-009**: Resource content MUST NOT appear in `__schema` or `__schema --as=mcp`. Both keep projecting resource metadata only, and all existing goldens MUST remain byte-identical.
- **FR-010**: The `mcp` package MUST expose a `WithInstructions` option whose text is delivered in the `initialize` result's `instructions` field. When the option is not used, the field MUST be absent exactly as it is in v0.8.0.
- **FR-011**: Instructions MUST be validated at startup: they MUST be valid UTF-8 and within a size cap. A violation MUST fail closed with exit `2` and an `ax.Error` envelope on `stderr`, like `WithVersion`. Instructions are a runtime option and are not part of `__schema`.
- **FR-012**: Size caps MUST bound prompt templates, resource content, and instructions (Principle IX), with room for a skill-sized resource (at least the 6.6 KB go-decide document). A declaration over a cap MUST be rejected at declaration time with the stable `invalid_schema_declaration` code.
- **FR-013**: Prompt arguments supplied to `prompts/get` MUST be bounded in size, and an oversized value MUST be rejected with invalid-params.
- **FR-014**: Nothing in this feature MAY write to `stdout` outside the protocol channel (Principle I), persist state, or run a template. The server renders text and never executes it (Principle VI, Additional Constraints).
- **FR-015**: The change MUST be additive: new exported API and one new field on the resource declaration, no change to the `__schema` shape, and no existing golden regenerated. It is a `0.MINOR` bump (Principle XI).
- **FR-016**: Root `ax` MUST re-export any new public symbol as the existing identity-preserving alias or wrapper convention requires, and the exported-surface baseline and root audit are updated intentionally.
- **FR-017**: Documentation MUST be updated. The statement "Phase 1 is a declaration only: the live mcp-server does not serve prompts yet" is removed from `mcp/doc.go` and the schema declaration docs. The declaration docs gain the guidance to keep templates short and put long reference text in resource content. `README.md` and `examples/integration/` show a declared prompt, a resource with content, and instructions.
- **FR-018**: The declared prompts and resources MUST be discoverable and servable identically over every transport the server supports (stdio and HTTP).
- **FR-019**: The live server MUST fail closed at startup, before registering anything, when the declaration tree holds a duplicate prompt name, a duplicate resource URI, or a corrupt declaration annotation, with the same `validation_error` envelope (exit `2`, empty `stdout`) that `__schema` emits for it.

### Key Entities

- **Prompt declaration**: Unchanged from spec 028. Now also served: listed by `prompts/list` and rendered by `prompts/get`.
- **Resource declaration**: Spec 028's URI, name, title, description, and MIME type, plus a new static content field. Content is observable only through `resources/read`.
- **Server instructions**: A short runtime string supplied through an `mcp` option and delivered at `initialize`. It points the agent at resources and tools. It is not part of the schema contract.
- **Rendered prompt**: The single user-role message `prompts/get` returns for a prompt and its arguments.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the existing `__schema`, `__schema --as=mcp`, and MCP `tools/list` goldens pass unchanged after the feature lands.
- **SC-002**: For a CLI with no prompts, no resources, and no instructions, the `initialize` result and capability set are byte-identical to v0.8.0.
- **SC-003**: For a fixture tree with declarations on a root, a non-runnable group, an excluded leaf, and a hidden subtree, `prompts/list` and `resources/list` equal the `__schema --as=mcp` projection exactly, on both stdio and HTTP.
- **SC-004**: Ten consecutive `prompts/get` calls with identical arguments, and ten `resources/read` calls for one URI, return byte-identical results.
- **SC-005**: Every `prompts/get` error case (missing required argument, undeclared argument, unknown prompt, oversized value) returns a protocol error with no rendered text; none returns a partial render.
- **SC-006**: A resource of at least 6.6 KB can be declared, listed, and read back byte-for-byte.
- **SC-007**: In a real Claude Code session, an agent given only the instructions reads the declared resource without an `@` mention, recorded with the client version and steps in the pull request.
- **SC-008**: The module gains zero new dependencies, and the `schema` package's import-isolation test still passes.
- **SC-009**: `make` gates pass: lint, race tests across the build-tag matrix, fuzz smoke, goldens, and deadcode.

## Assumptions

- Source inputs: GitHub issue #270 and spec 028 (`specs/028-mcp-prompts-resources/`). No ADR governs this feature.
- The MCP Go SDK version already in use provides prompt registration, resource registration, and a server instructions option, so no dependency change is expected.
- Resource content is text only in this feature. Blob content is a later additive field if an adopter needs it.
- Dynamic resource content of any kind (callbacks, computed or per-call content, run records, live state) stays permanently out of scope under Principle VI.
- Out of scope: prompt-argument completion, resource templates, sampling, `listChanged` notifications, per-command elicitation (#137), and any orchestration. The prompt and resource set is static for the life of the server.
- Exact cap values for templates, resource content, instructions, and prompt argument values are chosen in planning. They must admit the 6.6 KB go-decide document with comfortable headroom.
- The behavior of a declared resource that has no content (served empty or omitted) is decided in planning.
- Shipped as `feat(mcp)` with a `feat(schema)` component; `breaking-change-approved` is not applied. Adjacent to #137 and #67.
