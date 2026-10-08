# Research: MCP Prompts and Static Resources in the Schema Contract

No ADR governs this feature, so there is no "Decision Records Absorbed" section.
Every decision below resolves a design question the spec left to planning.

## R1. Where declarations live

- **Decision**: Store declarations as JSON-encoded lists in two namespaced Cobra
  annotations on the declaring command:
  `github.com/rshade/ax-go/schema/prompts` and
  `github.com/rshade/ax-go/schema/resources`.
- **Rationale**: This follows the two existing precedents exactly.
  `schema.WithNonDeterministicFields` writes `github.com/rshade/ax-go/schema/...`
  annotations, and `mcp.Exclude` writes `github.com/rshade/ax-go/mcp/exclude`.
  `internal/schema.BuildCommand` already clones `cmd.Annotations`, so
  declarations ride the same walk that builds `__schema` (FR-005) with zero
  changes to `BuildCommand`. It is also the benchmarked `BenchmarkBuildCommand`
  path, so the hot path is untouched. JSON encoding at declaration time is
  copy-by-value by construction (FR-008).
- **Alternatives considered**: A registry option on `NewSchemaCommand` was
  rejected: it is a second source of truth, it diverges from the tree, and it
  would not be visible to Phase 2's server. A side map keyed by
  `*cobra.Command` was rejected because it is mutable package-level state,
  which Constitution VI forbids.

## R2. Error shape for invalid declarations

- **Decision**: `DeclarePrompt` and `DeclareResource` return `error`. On failure
  the value is a `*contract.Error` (`*ax.Error`) with `error_code`
  `invalid_schema_declaration`, exit code `2`, and a `context` naming the
  offending field. It is built with `context.Background()` because declaration
  happens during tree construction, before any span exists, and performs no I/O.
  So there is no `ctx` parameter (Go Discipline: ctx is for I/O and
  cancellation).
- **Rationale**: The repository has no exported sentinel error variables;
  every library failure is an envelope with a stable code (`config/config.go`).
  An adopter that propagates the error out of `RunE` gets a correct exit `2`
  envelope for free. `errors.As(err, &axErr)` works.
- **Alternatives considered**: An exported `ErrInvalidDeclaration` sentinel was
  rejected because it would be the repo's first mutable exported var, against
  convention. Panicking was rejected (no `panic` in library code). Silently
  ignoring, the `WithNonDeterministicFields` nil behavior, was rejected because a
  dropped contract declaration is a silent failure.

## R3. Validation grammar

- **Decision**:
  - Names (prompt, argument) match `^[a-zA-Z0-9_.-]+$`, the MCP tool-name rule
    already used by `internal/mcp.ToolName`.
  - Argument names are unique within a prompt.
  - A template is required and non-empty. A placeholder is exactly `{{name}}`
    (no inner spaces), with name in the charset above (regex `\{\{([a-zA-Z0-9_.-]+)\}\}`).
    Every placeholder must name a declared argument. Any other brace text,
    including `{{ name }}` with spaces, is literal.
  - A resource URI must be at most 2048 bytes, contain no ASCII whitespace or
    control characters, and parse with `net/url.Parse` with a non-empty
    `Scheme`. A resource name is required.
  - Every string field must be valid UTF-8 (`unicode/utf8.ValidString`).
  - Prompt name and resource URI must be unique **within the declaring
    command**, checked at declaration time.
- **Rationale**: The rules are checkable locally, deterministic, and cheap.
  `net/url` and `unicode/utf8` are stdlib and outside the contract packages'
  forbidden set (`internal/testutil/imports.go` forbids `net/http`,
  `crypto/tls`, zerolog, gRPC, OTel SDK and the root facade, but not
  `net/url`). The 2048-byte cap bounds adopter-supplied input (Constitution IX).
- **Alternatives considered**: Go `text/template` syntax was rejected: it is a
  Turing-ish language whose evaluation would make Phase 2 non-deterministic and
  hard to validate. Requiring that every argument be referenced was rejected
  because an argument may steer the workflow without being substituted.

## R4. Cross-tree duplicate detection (clarification Q3)

- **Decision**: Add an internal checker that walks the non-hidden tree in pre-order, the same
  pruning as `BuildCommand`. Every projected entry counts, so a same-command
  duplicate from a hand-written annotation reports that path twice. It returns the **first** conflict in
  deterministic order: prompts are checked before resources, in walk order.
  `NewSchemaCommand`'s `RunE` runs it before writing either format. On conflict
  it returns `contract.NewError(cmd.Context(), "validation_error", ...)` with
  exit `2`, `context`
  `{"kind":"prompt"|"resource","key":<name or uri>,"commands":[<path1>,<path2>]}`
  and an `actionable_fix` telling the author to rename or remove one
  declaration. `BuildSchema` and `BuildMCPSchema` cannot return an error without
  a breaking signature change, so they keep the first declaration in walk order
  and drop later duplicates.
- **Rationale**: The fail-closed agent path is `__schema` itself. The Go
  builders stay non-breaking (Constitution XI) and deterministic. Reporting one
  conflict per run keeps the envelope small and the golden stable.
- **Alternatives considered**: An exported `schema.ValidateDeclarations(root)`
  was deferred. Adopters can already assert the failure through
  `axtest.Run(... "__schema")`, so a new exported symbol is not needed now.
  Reporting all conflicts at once was rejected as unnecessary churn in the
  envelope.

## R5. Projection shapes

- **Decision**:
  - The ax-native output reuses the declaration types directly:
    `CommandSchema.Prompts []Prompt` (`json:"prompts,omitempty"`) and
    `CommandSchema.Resources []Resource` (`json:"resources,omitempty"`). JSON
    keys are snake_case (`mime_type`).
  - The MCP adapter gets separate types, `MCPPrompt`, `MCPPromptArgument`, and
    `MCPResource`, with MCP camelCase (`mimeType`), following the `MCPTool`
    precedent. `MCPSchema` gains `Prompts []MCPPrompt` (`json:"prompts,omitempty"`)
    and `Resources []MCPResource` (`json:"resources,omitempty"`). `MCPPrompt`
    carries the MCP `prompts/list` fields **plus** an ax extension field
    `template`, the same way `MCPTool` already carries the non-standard
    `nonDeterministicFields`.
- **Rationale**: Separate MCP types decouple the two wire formats, so a future
  ax-native-only field never leaks into the MCP adapter, which is the reason
  `MCPTool` exists apart from `CommandSchema`. Field names match go-sdk v1.8.0's
  `mcp.Prompt`, `mcp.PromptArgument`, and `mcp.Resource` JSON tags, so Phase 2
  can map one-to-one. `omitempty` on every new collection keeps no-declaration
  output byte-identical (FR-009).
- **Alternatives considered**: Reusing `Prompt` inside `MCPSchema` was rejected
  because it couples formats. Omitting `template` from the MCP adapter was
  rejected because it would make the static adapter unable to answer "how do I
  use it", which is the feature's purpose.

## R6. MCP aggregation walk

- **Decision**: `internal/mcp.Build` aggregates prompts and resources from
  every non-hidden command, using exactly `BuildCommand`'s pruning (hidden
  only) rather than `WalkCallableCommands`. Reserved commands are **not**
  pruned.
- **Rationale**: This is the spec's edge-case rule. Exclusion and runnability
  govern tools only, and root or group commands are the natural home for
  CLI-wide prompts. The ax-native tree includes reserved commands (`__schema`,
  `completion`, `help` and `mcp-server` all appear in the integration golden),
  so pruning them only in the MCP walk would let the two formats diverge.
  Sharing one rule makes the formats agree by construction.

## R7. Live server is untouched (FR-011)

- **Decision**: No change to `internal/mcpserver`. A test adds declarations to
  the server fixture and asserts that the `initialize` capabilities have no
  `prompts` or `resources` key, and that `mcp_tools_list.golden.json` is
  unchanged.
- **Rationale**: The go-sdk advertises prompt and resource capabilities only
  when `AddPrompt` or `AddResource` is called, or when they are explicitly
  configured. Proving the absence pins the Phase 2 trigger boundary.

## R8. Root re-exports and gates

- **Decision**: Root `ax` gains the aliases `Prompt`, `PromptArgument`,
  `Resource`, `MCPPrompt`, `MCPPromptArgument`, and `MCPResource`, plus the
  wrappers `DeclarePrompt` and `DeclareResource`, matching its existing
  one-to-one re-export of `schema`. `make surface-update` regenerates
  `baseline.json`, and every new root feature, including the promoted new
  fields on `CommandSchema` and `MCPSchema`, gets a hand-classified `supported`
  audit row with `lifecycle: live`. Run `surfacecheck -audit-seed` to
  enumerate them. Examples are added for `schema.DeclarePrompt` and
  `schema.DeclareResource`. These are encouraged rather than doccover-gated,
  since neither is in `requiredSymbols`.
- **Rationale**: The root alias surface is a stability promise. A partial
  re-export would split adopters between import paths.

## R9. Performance

- **Decision**: No new tracked benchmark. `internal/schema.BuildCommand`
  (tracked) is unchanged. Decoding runs in the public converter only when the
  annotation key is present, so trees without declarations take one map lookup
  per command per key.
- **Rationale**: Per AGENTS.md, no numeric performance claim is made without a
  benchmark, and none is made here.

## R10. Resource content stays out of Phase 1 (clarification Q2)

- **Decision**: Resources carry no content field in Phase 1, not even a capped
  one. Content arrives with Phase 2's `resources/read`, as an additive field.
- **Rationale**: A truncated body is worse than none. An LLM handed the first
  64 KiB of a longer document cannot tell it holds a fragment, so it answers
  from partial material and never learns that it should fetch the real
  resource. An absent field is unambiguous: the resource exists, and its
  content is not available here. The accepted cost is that Phase 1 resources
  are discoverable but not readable, which the docs state outright.
- **Alternatives considered**: Inline text capped at 64 KiB was rejected for
  the truncation hazard above. Inline text with a `truncated` flag was rejected
  because it relies on every agent honoring the flag. A declared but
  unprojected content field was rejected because no golden could observe it.
