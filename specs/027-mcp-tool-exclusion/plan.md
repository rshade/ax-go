# Implementation Plan: MCP Tool Exclusion

**Branch**: `027-mcp-tool-exclusion` | **Date**: 2026-09-23 | **Spec**: [spec.md](spec.md)

## Summary

Tighten the single shared projection, `internal/mcp.WalkCallableCommands`, which already feeds both `__schema --as=mcp` and the live `mcp-server`. Two structural rules are added (skip non-runnable nodes; treat `help` as reserved), plus one opt-in, node-only exclusion annotation exposed through `mcp.Exclude`. Because every rule lives on the command tree and in the one shared walk, static/live parity holds by construction. The `mcp-server` example text is also derived from the adopter's root name.

## Technical Context

**Language/Version**: Go (version pinned in `mise.toml`; `go.mod` must match)

**Primary Dependencies**: `github.com/spf13/cobra` (`Command.Runnable`, `Annotations`), existing `internal/mcp`, `internal/mcpserver`, public `mcp`

**Testing**: `go test`; `make test`, `make validate`, `make lint` (all four build-tag combinations); `make surface-check`; `make doc-coverage`; `make cover-check`

**Constraints**: No new exported symbols in root `ax` (FR-009). The `mcp` package stays a thin facade (doc.go). MCP SDK types stay confined to `internal/mcpserver`.

## Constitution Check

- Machine contract determinism: the walk order is unchanged (depth-first, Cobra order), so tool order stays deterministic.
- Fail-closed: a non-canonical annotation value means *not excluded* (a typo can't silently remove a tool). An excluded name hits the existing unknown-tool validation path.
- Import isolation: the annotation key constant lives in `internal/mcp` and is written by the public `mcp.Exclude`. No new dependency edges. `import_isolation_test.go` must stay green.
- Stability policy: additive public API (`mcp.Exclude`). The behavior change (groups and `help` removed from tool lists) is documented as `feat(mcp)`.

## Design

### Walk rules (`internal/mcp/mcp.go`)

```go
func WalkCallableCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
    if cmd.Hidden || isReservedCommand(cmd.Name()) {
        return // prune subtree (unchanged semantics; "help" now reserved)
    }
    if cmd.Runnable() && !IsExcluded(cmd) {
        visit(cmd) // node-only skip otherwise; children still walked
    }
    for _, child := range cmd.Commands() {
        WalkCallableCommands(child, visit)
    }
}
```

- `helpCommandName = "help"` is added to the reserved constants and `isReservedCommand`.
- `ExcludeAnnotationKey = "github.com/rshade/ax-go/mcp/exclude"` and `excludeAnnotationValue = "true"`.
- `IsExcluded(cmd)` returns true only for the exact canonical value.
- `MarkExcluded(cmd)`: nil-safe, allocates `Annotations` if nil, preserves existing keys.

### Public facade (`mcp/exclude.go`)

- `func Exclude(cmd *cobra.Command)` delegates to `internalmcp.MarkExcluded`. Its doc comment states: node-only, children unaffected, help and `__schema --as=ax` unaffected, applies to both static and live surfaces.
- A runnable `Example` (`example_test.go`) shows excluding a runnable root and a blocking `serve` leaf.

### Dispatch (`internal/mcpserver`)

No change expected. `server.go:145` builds the tool registry from the same walk, so an excluded name is never registered. Over the wire the MCP SDK rejects an unregistered name before dispatch; `dispatch.go:149` is the dispatcher's own guard for a direct call. Test both layers, and prove an excluded command's `RunE` is never invoked.

### Example text (`mcp/command.go`)

Replace the literal `mycli` in `Example` with the root's name, resolved at `NewCommand(root, ...)` via `root.Root().Name()`, falling back to `root.Name()`.

### Docs

- `mcp/doc.go`: document the three skip rules (hidden → subtree; reserved incl. `help` → subtree; non-runnable or `Exclude`d → node only).
- `docs/src/content/docs/guides/expose-schema.md`: replace the "every non-hidden command" sentence and add an "Excluding commands" section.
- README "Running as an MCP server", the `NewCommand` `Long` text, and the `Build`/`WalkCallableCommands`/`discoverTools` doc comments: state the new rules.
- Parity scope: the live-only `requiresPositionalArgs` filter predates this feature and is unchanged; parity fixtures contain no positional-argument commands (FR-006).
- Never edit `CHANGELOG.md` (release-please owns it).

## Project Structure (touched)

```text
internal/mcp/mcp.go            # walk rules, reserved help, annotation helpers
internal/mcp/walk_test.go      # walk table tests
internal/mcp/exclude_test.go   # NEW: annotation helper tests
mcp/exclude.go                 # NEW: public Exclude
mcp/example_test.go            # Example for Exclude
mcp/command.go                 # Example text uses root name
mcp/doc.go                     # rules documented
README.md                      # MCP section: which commands become tools
mcp/exclude_test.go            # NEW: public Exclude, static/live parity, tools/call rejection, help/AX presence
internal/mcpserver/dispatch_test.go  # dispatcher unknown-tool guard for an excluded name
internal/cmd/surfacecheck/...  # surface inventory update (make surface-update)
testdata/ goldens              # only if a golden lists groups/help (justify)
docs/src/content/docs/guides/expose-schema.md  # Excluding commands section
```

## Risks

- **Golden churn**: existing goldens/fixtures may list group or help tools. Update them only where the change is exactly those removals, and call each one out in the commit message.
- **Adopters relying on group tools**: low risk, since they returned usage text only. Covered by the release note.
- **`Runnable()` on a root with `RunE` that only prints help**: stays listed. Adopters use `Exclude` for that; document it.
