# Feature Specification: Per-Flag MCP Exclusion (`mcp.ExcludeFlag`)

**Feature Branch**: `036-mcp-flag-exclusion`

**Created**: 2026-11-17

**Status**: Draft

**Input**: GitHub issue rshade/ax-go#332. `mcp.Exclude` removes an entire command from MCP, and hidden flags still appear in MCP input schemas. Consumer `github.com/rshade/tsacl` has `tsacl test --watch`, which runs indefinitely; over MCP's request/response protocol a `tools/call` passing `watch: true` blocks forever and holds the dispatcher lock in `internal/mcpserver/dispatch.go`. Excluding `test` would remove the primary tool, and hiding `--watch` does not remove it from `tools/list` or `tools/call`. Provide `mcp.ExcludeFlag(cmd *cobra.Command, name string)` so a CLI author can keep a command as an MCP tool while omitting specific incompatible flags from MCP input schemas (`tools/list` and `__schema --as=mcp`) and rejecting them with a validation error (exit `2`) on `tools/call`, while keeping the flag visible in `--help` and `__schema --as=ax`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - CLI author excludes an incompatible flag from MCP tool schemas while keeping the command and CLI flag intact (Priority: P1)

A CLI author exposes a command such as `test` over `mcp-server`, but one of its flags (such as `--watch` or `--interactive`) never terminates or cannot work over request/response. The author calls `mcp.ExcludeFlag(testCmd, "watch")`. When an agent inspects `<cli> __schema --as=mcp` or calls `tools/list` on `<cli> mcp-server`, the `test` tool is advertised with all of its other flags, while `watch` is absent from both `inputSchema.properties` and `inputSchema.required`. When a human or CLI agent runs `<cli> test --help`, `<cli> __schema --as=ax`, or `<cli> test --watch` directly on the CLI, `--watch` remains present and works as before.

**Why this priority**: This directly solves the adopter defect reported in #332 without dropping the entire command from MCP or maintaining a separate command tree for `mcp-server`.

**Independent Test**: Create a runnable command with `--watch` and `--filter` flags, call `mcp.ExcludeFlag(cmd, "watch")`, and verify that `watch` is absent from `__schema --as=mcp` and live `tools/list`, `filter` remains present, and `--watch` remains in `--help` and `__schema --as=ax` with the exclusion annotation set on the pflag.

**Acceptance Scenarios**:

1. **Given** a runnable command with flags `--watch` and `--filter` where `mcp.ExcludeFlag(cmd, "watch")` has been called, **When** `__schema --as=mcp` or live `mcp-server` `tools/list` is inspected, **Then** the tool's `inputSchema.properties` contains `filter` and omits `watch`.
2. **Given** a flag marked both required (`MarkFlagRequired`) and excluded (`mcp.ExcludeFlag`), **When** `__schema --as=mcp` or live `tools/list` is inspected, **Then** the flag is absent from both `inputSchema.properties` and `inputSchema.required`.
3. **Given** a command with an excluded flag, **When** `<cli> <cmd> --help` or `<cli> __schema --as=ax` is executed, **Then** the excluded flag still appears normally in help output and in the AX schema flag list, and its pflag `Annotations` map carries `"github.com/rshade/ax-go/mcp/exclude": ["true"]`.
4. **Given** a `nil` command, an empty flag name, or a flag name that does not exist on the command, **When** `mcp.ExcludeFlag(cmd, name)` is called, **Then** it is a safe no-op and does not panic or mutate unrelated state; when called on a flag with existing pflag annotations, those existing annotations are preserved.

---

### User Story 2 - MCP dispatcher rejects excluded flags on `tools/call` before executing the command (Priority: P1)

An MCP client invokes `tools/call` on a tool and supplies an argument whose name was excluded with `mcp.ExcludeFlag` (for example, `{"watch": true}`). Even though the client bypassed the advertised `inputSchema`, the MCP server's argument builder rejects `watch` as an unknown argument for that tool before executing the command: it returns `CallToolResult{IsError: true}` with a single `TextContent` item containing the `validation_error` `ax.Error` envelope (exit code `2`), and the command's `Run`/`RunE` function never runs. Calling the same tool without `watch` succeeds normally.

**Why this priority**: Schema omission alone does not prevent a misbehaving or hallucinating MCP client from sending `"watch": true` in `tools/call`. Rejecting the argument before execution prevents the command from running and blocking the serialized dispatcher lock.

**Independent Test**: Mount a command with an excluded `--watch` flag and a call counter in `RunE`. Call the tool via `tools/call` with `{"watch": true}` and verify `IsError: true`, `len(Content) == 1`, `error_code == "validation_error"`, and call counter `0`. Call the tool again without `watch` and verify it succeeds and increments the counter to `1`.

**Acceptance Scenarios**:

1. **Given** a command with `mcp.ExcludeFlag(cmd, "watch")`, **When** an MCP client calls `tools/call` with `{"watch": true}` (or any value, including `null`), **Then** the response has `IsError: true`, a single `TextContent` item carrying an `ax.Error` envelope with `error_code: "validation_error"` (exit `2`), and the command's `RunE` is never invoked.
2. **Given** a command with `mcp.ExcludeFlag(cmd, "watch")`, **When** an MCP client calls `tools/call` without `"watch"`, **Then** the command executes normally with `--watch` at its default value.

---

### User Story 3 - Persistent flag inheritance, local flag shadowing, and fail-closed annotation semantics stay consistent across static and live MCP surfaces (Priority: P2)

A CLI author excludes a persistent flag on a parent or root command (`mcp.ExcludeFlag(root, "watch")`) or shadows an inherited flag on a child command. Both `__schema --as=mcp` and live `mcp-server` apply the exact same inheritance and shadowing rules, and any hand-edited annotation with a non-canonical value fails closed (stays included) so a typo cannot silently drop a flag from the contract.

**Why this priority**: Parity between static `__schema --as=mcp` and live `mcp-server` is a core architectural invariant (Constitution Principle III).

**Independent Test**: Build a parent/child command tree with persistent and shadowed flags, apply `mcp.ExcludeFlag`, and verify identical `inputSchema` properties and `required` lists across `__schema --as=mcp` and live `tools/list`, plus consistent `tools/call` acceptance/rejection.

**Acceptance Scenarios**:

1. **Given** a persistent flag on a parent command marked with `mcp.ExcludeFlag(parent, "watch")`, **When** a child command inherits `--watch`, **Then** `watch` is omitted from the child tool's `inputSchema` in both `__schema --as=mcp` and `tools/list` and is rejected on `tools/call` for the child tool.
2. **Given** an unexcluded persistent flag on a parent command and a local flag of the same name on a child command marked with `mcp.ExcludeFlag(child, "watch")`, **When** the child tool is projected and dispatched, **Then** `watch` is omitted from the child tool's `inputSchema` (the shadowed inherited flag does not leak through) and rejected on `tools/call` for the child, while remaining accepted on the parent tool.
3. **Given** a flag whose `"github.com/rshade/ax-go/mcp/exclude"` pflag annotation is set to anything other than `[]string{"true"}` (such as `[]string{"TRUE"}`, `[]string{"1"}`, `[]string{""}`, `[]string{}`, or `[]string{"true", "false"}`), **When** the tool schema is built or dispatched, **Then** the flag is treated as not excluded.

---

### Edge Cases

- **Nil command or unknown flag name**: `mcp.ExcludeFlag(nil, "watch")`, `mcp.ExcludeFlag(cmd, "")`, and `mcp.ExcludeFlag(cmd, "no-such-flag")` are silent no-ops, matching `mcp.Exclude(nil)` and the issue #332 specification.
- **Flag lookup order**: `mcp.ExcludeFlag(cmd, name)` looks up `name` in `cmd.Flags()` first, then `cmd.PersistentFlags()`, matching `schema.DeclareFlagEnum` and `schema.DeclareFlagExample`, and considers only flags `cmd` itself declares.
- **Inherited-only name**: `mcp.ExcludeFlag(child, "watch")` where `watch` is a parent's persistent flag is a no-op, before and after Cobra merges the parent's flags into the child. Marking the shared `*pflag.Flag` would otherwise exclude it from the parent and every sibling.
- **Reserved agent-safety flags**: `format`, `dry-run`, `yes`, `idempotency-key`, and `strict` are never excluded. `mcp.ExcludeFlag` ignores them, and a hand-set annotation has no effect, so MCP clients can always dry-run, approve, retry under a stable key, and opt into strict warnings.
- **Required and excluded**: A flag marked both `cobra.MarkFlagRequired` and excluded can never be supplied over MCP, so every `tools/call` on the tool fails with a `validation_error` (exit `2`) naming the flag, and `Run`/`RunE` never executes.
- **Preserving existing flag annotations**: If a flag already carries `cobra.BashCompOneRequiredFlag` or `schema` example annotations, calling `mcp.ExcludeFlag` preserves those entries in `flag.Annotations` while setting `ExcludeAnnotationKey`.
- **Local shadow of an excluded persistent flag**: If a parent command marks a persistent flag excluded (`mcp.ExcludeFlag(parent, "watch")`) and a child defines its own local `--watch` flag without excluding it, the child's local flag wins for the child tool (consistent with existing local-overrides-inherited precedence in `collectFlags` and `acceptedFlags`).
- **Local excluded flag shadowing an unexcluded persistent flag**: If a parent has an unexcluded persistent `--watch` flag and a child defines a local `--watch` flag and marks it excluded (`mcp.ExcludeFlag(child, "watch")`), `seen["watch"]` is recorded when visiting the local flag so the shadowed inherited flag does not leak into the child's schema or `acceptedFlags`.
- **Existing goldens**: Baseline golden files (`testdata/schema_ax.golden.json`, `testdata/schema_mcp.golden.json`, `testdata/mcp_tools_list.golden.json`) use fixtures that do not exclude flags and remain byte-identical.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The public `mcp` package MUST export `func ExcludeFlag(cmd *cobra.Command, name string)`, which looks up `name` in `cmd.Flags()` and then `cmd.PersistentFlags()` and sets the pflag annotation `"github.com/rshade/ax-go/mcp/exclude"` (`internalmcp.ExcludeAnnotationKey`) to `[]string{"true"}`.
- **FR-002**: `mcp.ExcludeFlag` MUST be a safe no-op when `cmd` is `nil`, `name` is empty, or `name` is not found in `cmd.Flags()` or `cmd.PersistentFlags()`, and MUST preserve any existing entries in `flag.Annotations`.
- **FR-003**: `internal/mcp` MUST provide `IsFlagExcluded(flag *pflag.Flag) bool` and `MarkFlagExcluded(cmd *cobra.Command, name string)`. `IsFlagExcluded` MUST fail closed, returning `true` if and only if `flag != nil` and `flag.Annotations[ExcludeAnnotationKey]` has length `1` with element `"true"`.
- **FR-004**: `internal/mcp` tool input-schema construction (`collectFlags`, `flagProperties`, `requiredFlags`) MUST omit any flag for which `IsFlagExcluded(flag)` is `true`, so excluded flags never appear in `inputSchema.properties` or `inputSchema.required` in either `__schema --as=mcp` or live `mcp-server` `tools/list`.
- **FR-005**: When a command defines a local flag that shadows an inherited persistent flag of the same name, `collectFlags` and `acceptedFlags` MUST evaluate exclusion on the winning local flag and mark the flag name as seen before checking exclusion, so an excluded local flag never falls back to the shadowed inherited flag.
- **FR-006**: In `internal/mcpserver`, `acceptedFlags` (used by `buildArgs`) MUST omit any flag for which `internalmcp.IsFlagExcluded(flag)` is `true`. Consequently, a `tools/call` supplying an excluded flag name MUST fail in `buildArgs` with the `validation_error` `ax.Error` envelope (exit code `2`, `IsError: true`, single `TextContent` item) before command execution (`Run`/`RunE` never runs).
- **FR-007**: Excluded flags MUST remain unchanged in `--help` output, direct CLI execution, and `__schema --as=ax` (`schema.BuildSchema`), and the annotation MUST be inspectable on the flag's `Annotations` map.
- **FR-008**: Root `ax` and all import-isolated packages (`contract`, `config`, `schema`, `id`, `logging`) MUST gain zero new exported symbols. The only new public symbol is `mcp.ExcludeFlag`, and `make surface-check` (`internal/cmd/surfacecheck/baseline.json`) MUST be updated intentionally.
- **FR-009**: Package documentation (`mcp/doc.go`, `mcp/exclude.go`), runnable example coverage (`ExampleExcludeFlag` in `mcp/example_test.go`), repository documentation (`README.md`), and the docs site (`docs/src/content/docs/guides/expose-schema.md`) MUST document `mcp.ExcludeFlag`.

### Key Entities

- **Flag Exclusion Annotation**: The pflag annotation entry `"github.com/rshade/ax-go/mcp/exclude": []string{"true"}` attached to a `*pflag.Flag.Annotations` map by `mcp.ExcludeFlag`.
- **Accepted MCP Tool Flags**: The set of local and inherited flags on a callable command after shadowing resolution and `IsFlagExcluded` filtering, shared by static/live `inputSchema` generation and live `tools/call` argument validation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a command with an excluded flag, 100% of static (`__schema --as=mcp`) and live (`tools/list`) tool schemas omit the excluded flag from `inputSchema.properties` and `inputSchema.required` while retaining all non-excluded flags in deterministic order.
- **SC-002**: 100% of `tools/call` requests supplying an excluded flag return `IsError: true` with a single `TextContent` item decoding to a `validation_error` `ax.Error` envelope (exit code `2`), and the target command's `RunE` invocation count remains `0`.
- **SC-003**: 100% of excluded flags remain present in `<cmd> --help` and `__schema --as=ax` output and remain usable during direct CLI execution.
- **SC-004**: All repository quality gates (`make validate`, `make test`, `golangci-lint`, `make cover-check`, `make doc-coverage`, `make surface-check`, `make dead-check`, `make slop-check`, `make size-check`, `govulncheck`) pass across the 4-tag build matrix.

## Assumptions

- Source inputs: GitHub issue #332. No frozen ADR in `docs/adr/` governs this feature (`0004-trace-id-format.md` and `0008-cli-framework-cobra.md` are unrelated).
- Clarification was not required: issue #332 explicitly designates `mcp.ExcludeFlag(cmd *cobra.Command, name string)` as the preferred solution, specifies its exact signature, doc contract, pflag annotation mechanism, `inputSchema` omission, `buildArgs` validation error (exit `2`) on `tools/call`, preservation in `--help` and `__schema --as=ax`, and nil/unknown-name no-op behavior. Because `mcp.ExcludeFlag` solves both discovery (`tools/list` and `__schema --as=mcp`) and execution (`tools/call` rejection before `RunE`), the complementary context-inspection alternative mentioned in #332 is unnecessary and out of scope.
- Timeouts for long-running tools are out of scope and tracked separately by #129.
