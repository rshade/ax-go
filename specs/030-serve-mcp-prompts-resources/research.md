# Research: Serve Declared MCP Prompts, Resources, and Instructions

No governing ADR; nothing to absorb or retire.

## R-001: Registration source

- **Decision**: Register from `schema.CollectDeclarations(root)` (non-erroring,
  first-wins, walk order), the aggregation `BuildMCPSchema` uses.
- **Rationale**: One source of truth, mirroring `discoverTools`. Parity with
  `__schema --as=mcp` is then structural, not asserted.
- **Alternatives**: A second walk in `mcpserver` (rejected: the drift
  `discoverTools` exists to prevent).
- **Duplicates**: `__schema` fails closed on a cross-tree duplicate. The live
  server must not silently keep one, so `Serve` calls `FindDuplicate` and
  `FindCorrupt` at startup and returns the same `validation_error` (exit `2`)
  before registering anything. A server that starts is guaranteed parity.

## R-002: Capability advertising

- **Decision**: Call `AddPrompt` / `AddResource` only when the collected slice
  is non-empty; the SDK derives the `prompts` / `resources` capability from
  registrations.
- **Rationale**: Keeps a no-declaration CLI byte-identical (FR-002, SC-002). The
  SDK v1.8.0 `AddPrompt` / `AddResource` emit list-changed notifications, which
  are inert before a session exists. A test asserts the exact capability set
  for four cases (none, prompts only, resources only, both).
- **Note**: The issue cites SDK v1.7.0; `go.mod` requires v1.8.0. API is the
  same. `AddResource` panics on an unparseable URI; declaration-time validation
  already rejects those, so no new guard is needed.

## R-003: Rendering

- **Decision**: Add `RenderTemplate(template string, values map[string]string)
  string` in `internal/schema`, implemented on the same scanner as
  `templatePlaceholders`, so a span is a placeholder in rendering exactly when
  it is one in validation. Single pass; substituted values are never rescanned.
- **Rationale**: Two scanners would eventually disagree on `{{{x}}}` style
  edge cases that spec 028 already pinned. Single pass prevents argument values
  from injecting placeholders.
- **Alternatives**: `strings.NewReplacer` (rejected: wrong semantics for
  literal brace text and it would substitute inside adjacent braces).
- **Absent optional argument**: renders as empty text (FR-005), documented on
  `schema.Prompt`.

## R-004: Size caps

- **Decision**: Template 64 KiB, resource content 1 MiB, instructions 8 KiB,
  prompt argument value 64 KiB. Constants in `internal/schema` and
  `internal/mcpserver`, in the style of `maxResourceURIBytes`.
- **Rationale**: go-decide's skill is 6.6 KB, so every cap clears it by 10x or
  more. Instructions land in every connecting agent's system prompt, so they
  get the tightest cap. Declaration-time caps reject with the existing
  `invalid_schema_declaration` / `too_long` reason; the instructions cap fails
  at startup with `validation_error` exit `2`.
- **Alternatives**: No cap on templates (rejected: Principle IX). An aggregate
  tree cap (deferred; per-item caps bound the problem).
- **Compatibility**: A template over 64 KiB was valid in v0.8.0 and would now
  be rejected. Accepted as a fail-closed hardening, recorded in the commit.

## R-005: Resource content type and projection

- **Decision**: `internal/schema.Resource` gains `Content string
  `json:"content,omitempty"``, stored in the annotation. The public
  `schema.Resource` gains `Content string `json:"-"``, so no `__schema`
  projection can emit it. The explicit-literal `toMCPResource` replaces the
  type conversion, which would otherwise break (field sets differ).
  `fromInternalResource` copies `Content` so `CommandSchema.Resources` carries
  it in memory but never in JSON.
- **Rationale**: Existing goldens stay byte-identical (FR-009) with no new
  projection type. Public `DeclareResource` takes content directly.
- **Blob**: Not supported (text only). Adding `Blob` later is additive.
- **Alternatives**: A separate `DeclareResourceContent` function (rejected: two
  calls to keep consistent, content could be declared for an undeclared URI).
  `Content` in `__schema` (rejected: bloats discovery, issue statement).
- **Empty content**: A resource declared without content is still listed
  (parity) and `resources/read` returns an empty text body with the declared
  MIME type. Documented on the declaration.
- **Type-shape guard**: The existing reflection test already allows string
  fields, so `Content` passes; the test also gets a case asserting that
  `Content` serializes to nothing.

## R-006: Argument handling in `prompts/get`

- **Decision**: Validate before rendering: unknown prompt name, an argument not
  declared, a missing required argument, or a value over the cap returns an
  invalid-params error (SDK `jsonrpc2` invalid-params code, as `tools/call`
  does for unknown flags). Message names the argument, never echoes the value.
- **Rationale**: Matches `dispatch.go`'s strictness; no partial render.
- **Unknown prompt name**: The SDK returns its own invalid-params error when no
  prompt is registered under that name, so the handler is never reached.

## R-007: Instructions

- **Decision**: `mcp.WithInstructions(text)` sets `options.instructions`;
  `Config.Instructions` carries it; `newMCPServer` passes
  `&sdk.ServerOptions{Instructions: ...}` (the current `nil` becomes the
  options pointer only when set, keeping the unset path identical).
  Validation (UTF-8, cap) happens in `Serve` beside `validateVersion` with
  `validation_error` / exit `2`.
- **Logging**: A debug-level `stderr` log may record the instructions' length,
  not the text, to avoid echoing agent-facing content.

## R-008: Verification surfaces

- Unit: render table (substitution, literal braces, absent optional, unicode,
  no re-expansion), argument validation, cap validation, `Content`
  not projected.
- Parity: registered set equals `BuildMCPSchema` prompts and resources for a
  fixture with root, group, excluded leaf, hidden subtree.
- Integration: in-memory SDK client over stdio and HTTP.
- Golden: `prompts/get`, `resources/read`, and `initialize` (with and without
  instructions), new files only.
- Fuzz: `prompts/get` arguments and `RenderTemplate`.
- Manual: Claude Code session, recorded in the PR (SC-007).
