# Feature Specification: MCP Prompts and Static Resources in the Schema Contract

**Feature Branch**: `028-mcp-prompts-resources`

**Created**: 2026-10-07

**Status**: Draft

**Input**: Tracking issue rshade/ax-go#138 (Phase 1, contract-first). An agent operating an ax-go CLI with no filesystem context can discover the CLI's tools through `__schema` and `mcp-server`, but it has no server-vended answer to "how do I drive this tool" (MCP *prompts*: workflow templates) or "show me that again" (MCP *resources*: addressable, stable reference context). Today the schema contract and the MCP adapter declare tools only. This feature adds additive, static declarations of prompts and resources to the command tree, projected into `__schema` and `__schema --as=mcp`. Registering them on the live `mcp-server` (Phase 2) is trigger-gated and out of scope.

## Clarifications

### Session 2026-10-07

- Q: Is the prompt template text part of the Phase 1 contract? → A: Yes. A prompt declares static template text with a fixed `{{argument}}` placeholder syntax, validated at declaration time against the declared arguments, so Phase 2 only registers what is already pinned.
- Q: Does resource text content appear in `__schema` output? → A: No, metadata only. Phase 1 resources carry URI, name, title, description, and MIME type, with no content field at all. Content and `resources/read` arrive in Phase 2 as an additive field. An unprojected content field would be contract that no golden could observe.
- Q: What happens when the same prompt name or resource URI is declared on two different commands? → A: `__schema` (both formats) fails closed: it writes a `validation_error` envelope to `stderr` naming the duplicate and both command paths, exits `2`, and writes nothing to `stdout`. The non-erroring builders keep the first declaration in walk order, so their output stays deterministic.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Adopter declares a workflow prompt on a command (Priority: P1)

A CLI author attaches a named workflow template to a command in their Cobra tree, for example "triage-cost-spike: run `report --since=<window>`, then `explain` on the top line item". The template has a title, a description, declared arguments, and static template text. Running `<cli> __schema` shows the prompt on the command it was declared on. Running `<cli> __schema --as=mcp` lists it in a top-level `prompts` array in MCP prompt shape.

**Why this priority**: Prompts answer "how do I use it", which is the core gap for an agent with no repository on disk. Prompts alone are a viable MVP.

**Independent Test**: Declare one prompt with two arguments on a group command and one on a leaf. Confirm that both appear in `__schema` on their declaring commands and in `__schema --as=mcp` under `prompts`, in walk order, golden-pinned.

**Acceptance Scenarios**:

1. **Given** a command with one declared prompt, **When** `__schema` runs, **Then** that command's schema node carries a `prompts` list containing the prompt's name, title, description, arguments, and template exactly as declared.
2. **Given** prompts declared on a non-runnable group command and on a runnable leaf, **When** `__schema --as=mcp` runs, **Then** the top-level `prompts` array lists both, ordered by command-tree walk order and then by declaration order.
3. **Given** a prompt declared with an invalid shape (empty name, a name outside the MCP name charset, duplicate argument names, or template placeholders referencing an undeclared argument), **When** the author declares it, **Then** the declaration is rejected with a descriptive error and nothing is recorded on the command.

---

### User Story 2 - Adopter declares a static, read-only resource (Priority: P2)

A CLI author advertises addressable reference context on a command, for example `cli://finfocus/docs/pricing-model` with MIME type `text/markdown`. Phase 1 declares the resource's identity and metadata only. Its content is served by Phase 2's `resources/read`. Running `<cli> __schema` shows it on its command. Running `<cli> __schema --as=mcp` lists it in a top-level `resources` array in MCP `resources/list` shape.

**Why this priority**: Resources answer "show me that again" for stable reference material. They are less central than prompts, and they need the stricter static-only guarantees of User Story 3.

**Independent Test**: Declare a resource on the root. Confirm that it appears in both projections with its URI, name, title, description, and MIME type, golden-pinned.

**Acceptance Scenarios**:

1. **Given** a command with one declared resource, **When** `__schema` runs, **Then** that command's schema node carries a `resources` list with the resource's URI, name, title, description, and MIME type, and no content field.
2. **Given** resources declared on several commands, **When** `__schema --as=mcp` runs, **Then** the top-level `resources` array lists them all in walk order, then declaration order.
3. **Given** a resource declaration with an empty name, a URI that is not absolute (no scheme), or a URI containing whitespace or control characters, **When** the author declares it, **Then** it is rejected with a descriptive error and nothing is recorded.

---

### User Story 3 - Resources are provably static and read-only (Priority: P2)

A maintainer reviewing a future change needs a guarantee that resources can never become a channel for run-record or live state. That state is the TRACK leg, which structured logs on `stderr` serve. Persisting run state is permanently out of bounds under Constitution Principle VI.

**Why this priority**: The constitutional narrowing is what makes resources safe to add at all. Without an enforced guarantee, the first "resource that shows the last run" breaks the library's no-persisted-state boundary.

**Independent Test**: A test fails if the resource or prompt declaration types gain any field capable of carrying live behavior, such as a function, channel, pointer, map, or interface. A second test mutates the caller's declaration value (including its argument slice) after declaring it and confirms the projection is unchanged.

**Acceptance Scenarios**:

1. **Given** the published declaration types, **When** their field shapes are inspected recursively, **Then** every field is a string, bool, or slice of such value structs. No function, channel, pointer, map, or interface field exists.
2. **Given** a prompt or resource whose declaration value (including the prompt's argument slice) the caller mutates after declaring it, **When** `__schema` runs twice, **Then** both runs emit the declaration exactly as it was at declaration time, byte-identical.

---

### User Story 4 - Trees that declare nothing are unchanged (Priority: P1)

Every existing adopter upgrades and sees no difference.

**Why this priority**: Constitution Principle XI makes `__schema` output public API. The change must be strictly additive.

**Independent Test**: The existing declaration-free goldens (`testdata/schema_ax`, `testdata/schema_mcp`, `testdata/mcp_tools_list`) all pass with no regeneration.

**Acceptance Scenarios**:

1. **Given** a tree with no declarations, **When** `__schema` and `__schema --as=mcp` run, **Then** stdout is byte-identical to the pre-feature output. No empty `prompts` or `resources` keys appear.
2. **Given** a tree with declarations, **When** the live `mcp-server` initializes, **Then** it advertises exactly the same capabilities and tool list as before. No prompts or resources capability is advertised in this phase.

### Edge Cases

The **declaration tree** is the root command (always, even when hidden) plus
every command not under a hidden child: exactly the commands the ax-native
`__schema` tree contains. Both projections and both tree checks walk it.

- **Declarations on a hidden command, or anywhere in a hidden subtree**: they are pruned from both projections, matching how hidden commands are pruned from `__schema` today.
- **Declarations on a command excluded with `mcp.Exclude`, or on a non-runnable group**: they are still projected in both outputs. Exclusion governs tools only, and group or root commands are natural homes for CLI-wide prompts. The MCP aggregation therefore walks the whole declaration tree, not only callable ones.
- **Several declarations on one command**: they accumulate in declaration order, and existing annotations on the command are preserved.
- **Declaration on a nil command**: it returns an error and has no effect.
- **The same prompt name, or the same resource URI, declared twice anywhere in the declaration tree** (including twice on one command): `__schema` in both formats fails closed with a `validation_error` envelope on `stderr` naming the duplicate and the command path(s), exits `2`, and writes nothing to `stdout`. The non-erroring builders keep the first declaration in walk order. A duplicate on the same command is rejected immediately at declaration time. A duplicate across commands can only be detected once the tree is walked.
- **The same name used once by a prompt and once as a resource's name**: allowed. Prompts are keyed by name and resources by URI, in separate namespaces.
- **A hand-written annotation value that is not a valid encoded declaration** (bypassing the declaration function): it is never projected, the non-erroring builders omit it, a later declaration on that command is refused with `corrupt_annotation`, and `__schema` fails with `validation_error` (exit `2`) naming the command (FR-004b).
- **A hidden root command**: its own declarations are projected, because `__schema` never prunes the root it is given; only declarations under hidden children are dropped.
- **Template text or descriptions containing newlines or non-ASCII text**: they are emitted with standard strict-JSON escaping. Any text field that is not valid UTF-8 is rejected at declaration.
- **Template placeholders**: `{{name}}` must name a declared argument, or the declaration is rejected. A declared argument that the template never references is allowed, because it may shape the workflow without being substituted. Text that merely contains braces outside the exact `{{name}}` form is literal.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The public `schema` package MUST export a prompt declaration type carrying: name (required), title, description, an ordered list of arguments (each with a required name, a title, a description, and a required flag, mirroring MCP's prompt argument), and required, non-empty template text. The text may contain `{{name}}` placeholders, each naming a declared argument. The placeholder syntax is part of the contract.
- **FR-002**: The public `schema` package MUST export a resource declaration type carrying exactly: URI (required, absolute with a scheme), name (required), title, description, and MIME type. Phase 1 has no content field. Content is a Phase 2 additive field served through `resources/read`.
- **FR-003**: The public `schema` package MUST export one function to declare a prompt on a command and one to declare a resource on a command. Each validates its input. On invalid input or a nil command, it returns the standard error envelope type (usable with `errors.As`) with the stable error code `invalid_schema_declaration`, mapped to exit `2`. No sentinel variable is introduced, which matches the repository's envelope-first error convention. Neither panics. On success, each appends the declaration to the command's namespaced Cobra annotations (`github.com/rshade/ax-go/schema/...`, matching spec 015's key style) and preserves all other annotations.
- **FR-004**: Prompt and argument names MUST match the MCP name charset `^[a-zA-Z0-9_.-]+$`. Argument names MUST be unique within a prompt. A prompt name or resource URI already declared on the same command MUST be rejected. Resource URIs MUST be at most 2048 bytes, MUST parse as absolute URIs with a non-empty scheme, and MUST contain no whitespace or control characters. A resource MIME type MUST contain no control characters; spaces are allowed, so parameterised types such as `text/plain; charset=utf-8` are valid. A URI that has a scheme but does not parse is reported as `malformed`, distinct from `not_absolute` (no scheme). All text fields MUST be valid UTF-8.
- **FR-004a**: `__schema` in both formats MUST fail closed when the declaration tree declares the same prompt name twice or the same resource URI twice. It writes one `validation_error` envelope to `stderr` (exit `2`) whose context names the duplicate key and the declaring command paths, and writes nothing to `stdout`. Every projected entry counts, so a same-command duplicate that bypassed the declaration functions (a hand-written annotation) reports that command's path twice. Any builder that cannot return an error MUST keep the first declaration in walk order.
- **FR-004b**: A declaration function MUST refuse (reason `corrupt_annotation`, command unchanged) to rewrite an existing declaration annotation that does not decode or holds an invalid entry, so a write never silently discards prior content. `__schema` in both formats MUST fail closed with a `validation_error` envelope (exit `2`, empty `stdout`) naming the command whose declaration annotation does not project cleanly.
- **FR-005**: Declarations MUST be read back only through the same command-tree walk that builds `__schema`. There is one source of truth, and neither projection keeps a separate registry.
- **FR-006**: The ax-native `CommandSchema` MUST gain `prompts` and `resources` fields, both omitted when empty, that list the declarations made on that command in declaration order.
- **FR-007**: The MCP adapter `MCPSchema` MUST gain top-level `prompts` and `resources` arrays, both omitted when empty. They aggregate declarations from the root (always, even when hidden) and every command not under a hidden child, the exact pruning rule of the ax-native tree (reserved commands such as `__schema` are therefore included), in pre-order walk order (a command, then its children in Cobra order), then declaration order. Field names follow MCP's camelCase (`mimeType`).
- **FR-008**: The declaration types MUST be composed only of strings, bools, and slices of value structs built from those. Declarations MUST be captured by value at declaration time, so a later mutation of the caller's value or slices never changes the projection. A test MUST fail if any function, channel, pointer, map, or interface field is introduced (User Story 3).
- **FR-009**: For a tree with no declarations, `__schema` and `__schema --as=mcp` output MUST be byte-identical to the pre-feature output. The existing goldens MUST pass without regeneration.
- **FR-010**: Output MUST be deterministic: the same tree emits byte-identical `__schema` output across runs. New golden files MUST pin the ax-native and MCP shapes of a tree with declarations.
- **FR-011**: The live `mcp-server` MUST NOT register prompts or resources, and MUST NOT advertise prompts or resources capabilities. Its `initialize` capabilities and its `tools/list` output (`mcp_tools_list` golden) MUST be unchanged.
- **FR-012**: Root `ax` MUST re-export the new types and declaration functions as identity-preserving aliases and wrappers, matching its existing full re-export of the `schema` surface. `make surface-check` inventories are updated intentionally, and the root-package audit gains `supported` rows for the new root features.
- **FR-013**: The `schema` package MUST remain import-isolated: no new module dependencies and no imports from its forbidden set.
- **FR-014**: `README.md`, `examples/integration/`, and the docs site MUST document the declaration API and show one prompt and one resource. The integration example's goldens change only additively.

### Key Entities

- **Prompt declaration**: A server-vended workflow template. It has a name, optional title and description, ordered arguments, and template text with `{{name}}` placeholders. It is attached to the command it describes.
- **Prompt argument**: A named, optionally required input to a prompt, with a title and a description.
- **Resource declaration**: Addressable, static, read-only reference context. In Phase 1 it has a URI, name, optional title, description, and MIME type. Content arrives in Phase 2.
- **Declaration annotation**: The namespaced Cobra annotation holding a command's encoded declarations. It is the single source of truth read by the schema walk.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the existing declaration-free goldens (`testdata/schema_ax`, `testdata/schema_mcp`, `testdata/mcp_tools_list`) pass unchanged after the feature lands. The integration example's goldens change only additively, because FR-014 adds declarations to that CLI.
- **SC-002**: A fixture tree with prompts and resources on a root, a non-runnable group, an excluded leaf, and a hidden subtree projects exactly the expected declarations: everything except the hidden subtree's. It is golden-pinned in both formats.
- **SC-003**: Every invalid-declaration case in the validation table (at least one per rule in FR-001, FR-002, and FR-004, plus the nil command) is rejected, and the command's annotations are unchanged afterward.
- **SC-003a**: A tree declaring a duplicate prompt name, and separately one declaring a duplicate resource URI, across two commands makes `__schema` and `__schema --as=mcp` exit `2` with a golden-pinned `validation_error` envelope and empty `stdout`.
- **SC-003b**: A tree carrying a hand-written declaration annotation that does not decode cleanly makes `__schema` exit `2` with a golden-pinned `validation_error` envelope (`reason` `corrupt_annotation`) and empty `stdout`, and a declaration on that command is refused without rewriting the annotation.
- **SC-004**: Ten consecutive `__schema` runs over the declaration fixture produce byte-identical stdout.
- **SC-005**: The module gains zero new dependencies, and the `schema` package's import-isolation test passes.
- **SC-006**: The live server's capabilities and tool list are byte-identical before and after for a tree carrying declarations.

## Assumptions

- Source inputs: GitHub issue #138. No ADR governs this feature.
- Phase 2, registering declarations on the live `mcp-server` through the MCP SDK's prompt and resource APIs, is trigger-gated and unscheduled. The trigger is the first adopter hand-rolling a prompt template or static resource. Phase 2 will be its own Spec Kit feature.
- MCP sampling is out of scope because it violates output determinism. Resource templates (parameterized URIs) are out of scope because they imply computed content. Resource content of any kind (text or blob) is deferred to Phase 2, by clarification.
- Prompt arguments are declared explicitly. They are not auto-derived from the command's flags, which would couple a workflow template to one command's flag set.
- The change is additive under Constitution Principle XI (new optional output fields and new exported symbols). It ships as `feat(schema)`, and `breaking-change-approved` is not applied.
- Distinct from #67 (enriching tool descriptions with module metadata), and adjacent to #28 (per-flag schema semantics) and #16 (non-deterministic field enumeration).
