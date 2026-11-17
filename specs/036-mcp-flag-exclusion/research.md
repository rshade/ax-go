# Research: Per-Flag MCP Exclusion (`mcp.ExcludeFlag`)

No governing ADR in `docs/adr/`; nothing to absorb or retire.

## R-001: Per-flag exclusion (`mcp.ExcludeFlag`) vs. context dispatch marker

- **Decision**: Implement `mcp.ExcludeFlag(cmd *cobra.Command, name string)` as the sole public mechanism for issue #332, without adding a separate `mcp.FromContext` / `ax.DispatchedByMCP` context helper.
- **Rationale**:
  - Issue #332 explicitly designates `mcp.ExcludeFlag` as the **Preferred** solution and marks the context option as conditional ("If the context option is chosen").
  - `mcp.ExcludeFlag` solves both halves of the problem from a single declaration on the command tree: (1) it omits the incompatible flag from `tools/list` and `__schema --as=mcp` so agents never attempt to pass it, and (2) it rejects the flag in `internal/mcpserver/dispatch.go` (`buildArgs`) with a `validation_error` (exit `2`) before the command's `RunE` executes.
  - A context-only check inside `RunE` would still advertise `--watch` in `tools/list` and `__schema --as=mcp`, inviting agents to call it and fail at runtime.
  - Keeping the API to `mcp.ExcludeFlag` satisfies Constitution Principle X (Dependency & Surface Minimalism) and keeps root `ax` untouched (FR-008).
- **Alternatives considered**:
  - Adding `mcp.FromContext(ctx) bool` or `ax.DispatchedByMCP(ctx) bool` instead of `ExcludeFlag`: Rejected because the flag would still be advertised in `tools/list` and `__schema --as=mcp`.
  - Respecting `pflag.Flag.Hidden` to exclude flags from MCP: Rejected in the issue problem statement because hiding a flag also removes it from `<cli> <cmd> --help` for human CLI users, whereas `--watch` is a primary human-facing CLI flag.

## R-002: Annotation key, lookup order, and fail-closed semantics

- **Decision**: Reuse `internalmcp.ExcludeAnnotationKey` (`"github.com/rshade/ax-go/mcp/exclude"`) on `pflag.Flag.Annotations` with value `[]string{"true"}` (`[]string{excludeAnnotationValue}`). Look up `name` in `cmd.Flags().Lookup(name)` first, then `cmd.PersistentFlags().Lookup(name)`. `IsFlagExcluded(flag)` returns `true` if and only if `flag != nil && len(flag.Annotations[ExcludeAnnotationKey]) == 1 && flag.Annotations[ExcludeAnnotationKey][0] == "true"`.
- **Rationale**:
  - Reusing `"github.com/rshade/ax-go/mcp/exclude"` matches `mcp.Exclude` on `cobra.Command.Annotations`, giving adopters a single canonical annotation key across commands and flags.
  - Looking up `cmd.Flags()` and then `cmd.PersistentFlags()` matches `internal/schema.lookupFlag` (used by `schema.DeclareFlagEnum` and `schema.DeclareFlagExample`), so local and persistent flags behave identically across `schema` and `mcp`.
  - Fail-closed single-element comparison (`len == 1 && vals[0] == "true"`) matches `internal/schema.declaredExample` and `internal/mcp.IsExcluded`, ensuring a hand-edited typo never silently drops a flag from the MCP contract.

## R-003: Interaction with flag inheritance, local shadowing, and required flags

- **Decision**: In both `internal/mcp.collectFlags` and `internal/mcpserver.acceptedFlags`, record `seen[flag.Name] = struct{}{}` *before* checking `IsFlagExcluded(flag)`.
- **Rationale**:
  - Both functions visit `cmd.NonInheritedFlags()` first and `cmd.InheritedFlags()` second.
  - Recording `seen[flag.Name]` before the `IsFlagExcluded(flag)` early return guarantees that if a child command defines a local flag shadowing a parent's persistent flag and excludes that local flag (`mcp.ExcludeFlag(child, "watch")`), the parent's unexcluded persistent flag of the same name is still recognized as shadowed and does not leak into the child tool's schema or `acceptedFlags`.
  - Conversely, if a parent excludes a persistent flag (`mcp.ExcludeFlag(parent, "watch")`), the same `*pflag.Flag` pointer is visited via `child.InheritedFlags()` on every descendant and excluded automatically unless shadowed by an unexcluded local flag on the child.
  - Because `requiredFlags(cmd)` in `internal/mcp/mcp.go` iterates over `collectFlags(cmd)`, and `missingRequiredFlag(allowed, setFlags)` in `internal/mcpserver/dispatch.go` iterates over `acceptedFlags(target)`, an excluded flag that was also marked required for CLI use (`cobra.MarkFlagRequired`) is automatically omitted from `inputSchema.required` and does not block `tools/call` when omitted.
