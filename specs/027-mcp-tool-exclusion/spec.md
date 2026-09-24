# Feature Specification: MCP Tool Exclusion

**Feature Branch**: `027-mcp-tool-exclusion`

**Created**: 2026-09-23

**Status**: Draft

**Input**: Tracking issue rshade/ax-go#253. Adopter report rshade/finfocus#1509: `finfocus __schema --as=mcp` advertises 37 tools. They include the runnable root (which launches an interactive TUI), Cobra's auto-added `help`, six pure group commands that only print usage, and `analyzer serve`, a long-running Pulumi gRPC handshake. Because live dispatch is serialized (`internal/mcpserver/dispatch.go`), one call to a blocking command stalls every later `tools/call`. The only lever today is `cobra.Command.Hidden`, which `WalkCallableCommands` treats as "prune the whole subtree". An adopter therefore can't remove a root or group command without losing every tool beneath it, and can't remove a user-facing command from MCP without also hiding it from `--help`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Non-callable commands never become tools (Priority: P1)

An agent connects to an ax-go CLI's MCP server and calls `tools/list`. Every tool it sees does something when called. It never sees Cobra's `help` command, or a group command whose only behavior is printing usage, because calling those returns prose rather than a machine payload and wastes the agent's tool budget.

**Why this priority**: Every adopter with nested commands hits this with zero configuration. It is the default-correctness fix.

**Independent Test**: Build a tree with root → group (no `Run`/`RunE`) → leaf, plus Cobra's default help command. Confirm that `__schema --as=mcp` and live `tools/list` list the leaf (and the root only if it is runnable), and never `help` or the group.

**Acceptance Scenarios**:

1. **Given** a group command with neither `Run` nor `RunE` and a runnable child, **When** tools are listed, **Then** the child is listed and the group is not.
2. **Given** a CLI on which Cobra has initialized its default `help` command, **When** tools are listed, **Then** no tool named `<root>-help` appears.
3. **Given** a non-runnable root with runnable children, **When** tools are listed, **Then** the root is absent and all runnable descendants are present.

---

### User Story 2 - Adopter excludes one command without losing its children (Priority: P1)

A CLI author marks a specific command as not-an-MCP-tool, for example because it is interactive, long-running, or a TUI entry point. That command drops out of MCP. Its descendants remain tools, and it stays fully visible in `--help` and in `__schema --as=ax`.

**Why this priority**: This is the capability adopters are missing today. `Hidden` is the only workaround, and it is wrong on two counts: it prunes subtrees and it hides help.

**Independent Test**: Mark a runnable root and a runnable leaf as excluded. Confirm that both are absent from `__schema --as=mcp` and live `tools/list`, that the root's other children are still present, and that `--help` still shows both commands.

**Acceptance Scenarios**:

1. **Given** a runnable root marked excluded with runnable children, **When** tools are listed, **Then** the root is absent and every child is present.
2. **Given** an excluded leaf, **When** an MCP client sends `tools/call` for that leaf's derived tool name, **Then** the server returns the same `unknown tool` error as for any never-registered name, and the command does not execute.
3. **Given** an excluded command, **When** a user runs `<cli> --help` or `<cli> __schema --as=ax`, **Then** the command appears normally, the AX schema command tree keeps its unchanged shape, and the exclusion annotation can be inspected on the command's `cobra.Command.Annotations`.

---

### User Story 3 - Static and live surfaces cannot drift (Priority: P2)

A maintainer relies on `__schema --as=mcp` (static) to predict exactly what `mcp-server` (live) will serve.

**Why this priority**: Parity is an existing invariant (spec 011). The new rules must not break it.

**Independent Test**: For the same fixture tree, the static adapter's tool names equal the live server's `tools/list` names, in the same order.

**Acceptance Scenarios**:

1. **Given** any tree (without positional-argument commands) mixing hidden subtrees, reserved commands, non-runnable groups, `help`, and annotated exclusions, **When** both surfaces are generated, **Then** their tool-name lists are identical.

### Edge Cases

- A command is both `Hidden` and annotated as excluded: `Hidden` wins (subtree pruned), and the result is identical to `Hidden` alone.
- An excluded command whose children are also excluded: each is evaluated independently, and only unannotated runnable descendants are listed.
- An adopter defines its own command literally named `help` with a `RunE`: it is treated as reserved like Cobra's default and excluded. This is documented. A CLI that needs a callable help tool should rename it.
- The annotation is set to a value other than the canonical "true": it is treated as not excluded. Only the exact canonical value counts, so a typo can't silently exclude a command.
- A tree whose only runnable commands are all excluded: `tools/list` returns an explicit empty list, not an error.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `WalkCallableCommands` MUST skip, without pruning descendants, any command that is not runnable (`!cmd.Runnable()`).
- **FR-002**: `WalkCallableCommands` MUST treat a command named `help` as reserved, skipping that node and its subtree, in addition to the existing `__schema`, `mcp-server`, and `completion`.
- **FR-003**: The public `mcp` package MUST export a function that marks a single command as excluded from MCP, for example `mcp.Exclude(cmd *cobra.Command)`. It is implemented as a namespaced Cobra annotation (`github.com/rshade/ax-go/mcp/exclude` = `"true"`), matching the key style of spec 015's annotations. It MUST be nil-safe and MUST preserve any existing annotations.
- **FR-004**: `WalkCallableCommands` MUST skip, without pruning descendants, any command carrying that annotation with the canonical value.
- **FR-005**: `Hidden` semantics MUST be unchanged: a hidden command still prunes its whole subtree.
- **FR-006**: The new skip rules (FR-001, FR-002, FR-004) MUST NOT introduce any divergence between the static `__schema --as=mcp` adapter and the live `mcp-server` `tools/list`: for every tree without positional-argument commands, both MUST produce identical tool-name lists in the same order. Exclusion MUST be expressed only on the command tree (annotation or structural rule), never as a server-only option, so both paths observe it. The pre-existing live-only filter that drops commands whose `Args` validator rejects zero arguments (`internal/mcpserver.requiresPositionalArgs`) is unchanged and out of scope.
- **FR-007**: An excluded or skipped command MUST NOT be dispatchable via `tools/call`. It is never registered, so a call on its derived name fails exactly as a call on any never-registered name does: over the wire the MCP SDK rejects it with its `unknown tool` error before dispatch, and the dispatcher's own unknown-tool guard returns the `validation_error` envelope if it is reached directly. The command's `RunE` never runs.
- **FR-008**: Excluded commands MUST remain in `--help` output and in the `__schema --as=ax` command tree, whose shape is unchanged. The exclusion annotation MUST be inspectable on the command's `cobra.Command.Annotations`. The public `schema.CommandSchema` has no annotations field, and surfacing exclusion in `__schema --as=ax` (for example an additive `mcp_excluded` field) is a separate machine-contract change deferred to its own issue. This deviates from #253's "with the annotation shown" acceptance criterion.
- **FR-009**: Root `ax` MUST gain no new exported symbols. The new symbol lives in the `mcp` package, and `make surface-check` inventories are updated intentionally.
- **FR-010**: The `mcp-server` command's `Example` text MUST use the adopting CLI's root name rather than the literal `mycli` (for example, derived from `root.Name()` at `NewCommand` time), so adopters' `--help` is correct without overriding it.

### Key Entities

- **Exclusion annotation**: The namespaced Cobra annotation key and canonical value. The single source of truth read by the shared walk.
- **Callable command**: A command that is not hidden, not reserved, runnable, and not annotated excluded. This is exactly the set projected as MCP tools.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For the finfocus-shaped fixture (runnable root marked excluded, six groups, `help`, one blocking leaf marked excluded), the tool count drops from the pre-change count by exactly the number of groups, `help`, the root, and the excluded leaf, with no runnable unannotated command lost.
- **SC-002**: Static and live tool-name lists match for 100% of the test fixtures (none of which contain positional-argument commands; see FR-006).
- **SC-003**: `tools/call` on every excluded or skipped name returns the unknown-tool validation error, and the command's `RunE` is never invoked (asserted with a call counter).
- **SC-004**: All existing MCP, schema, and golden tests pass. Any golden that changes does so only because groups or `help` disappeared, and each such change is justified in the commit message as an intentional behavior change.

## Assumptions

- Dropping non-runnable groups and `help` from the tool list is a behavior change, not a break: tools that returned only usage prose had no machine contract to rely on. It ships as `feat(mcp)` with a release note, not as a major version.
- Issue #237 (a subtree `tools/call` re-executes the server) is a separate dispatch-path defect and is out of scope. It doesn't touch the walk.
- Per-tool MCP `annotations` (`readOnlyHint`/`destructiveHint`) are out of scope and tracked by #122.
