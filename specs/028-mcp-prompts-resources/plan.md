# Implementation Plan: MCP Prompts and Static Resources in the Schema Contract

**Branch**: `028-mcp-prompts-resources` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/028-mcp-prompts-resources/spec.md`

## Summary

Add two declaration functions, `schema.DeclarePrompt` and
`schema.DeclareResource`, that validate and store static prompt and resource
metadata as namespaced Cobra annotations on the command tree. The existing
`__schema` walk projects the declarations additively: `CommandSchema.prompts`
and `CommandSchema.resources` in the ax-native output, and top-level
`MCPSchema.prompts` and `MCPSchema.resources` in the `--as=mcp` output. Every
new collection is `omitempty`, so trees without declarations are
byte-identical. `__schema` fails closed (exit `2`) on a cross-tree duplicate.
Root `ax` re-exports everything. The live `mcp-server` is untouched (Phase 2 is
trigger-gated).

## Technical Context

**Language/Version**: Go 1.27.1 (pinned in `mise.toml`; `go.mod` matches)

**Primary Dependencies**: `github.com/spf13/cobra` (annotations), stdlib
`encoding/json`, `net/url`, `strings`, `unicode/utf8`. No new module
dependencies.

**Storage**: N/A. Declarations are in-memory Cobra annotations, and nothing is
persisted (Constitution VI).

**Testing**: `go test -race` (four-tag matrix via `make test`), golden files,
table-driven validation tests, a reflection type-shape test, and a determinism
loop.

**Target Platform**: All six surfacecheck GOOS/GOARCH profiles. The code has no
platform-specific parts.

**Project Type**: Go library (public contract package `schema`, root facade
`ax`).

**Performance Goals**: No regression on tracked benchmarks. `BuildCommand` is
unchanged (research R9).

**Constraints**: `schema` stays import-isolated, with no `net/http` and no
runtime facade. The public surface is additive only. Exported symbols do not
vary by build configuration. `mcp_tools_list.golden.json` and the existing
schema goldens stay byte-identical.

**Scale/Scope**: Around 8 new exported identifiers in `schema` plus root
aliases; around 400 lines of production code.

**Governing ADR(s)**: N/A

## Constitution Check

| Principle | Status |
| --- | --- |
| I Stream separation | `__schema` payload on stdout; duplicate failure envelope on stderr only, stdout empty. ✅ |
| II Determinism and exit codes | Pre-order walk plus declaration order; structs not maps for payloads; duplicate → `validation_error` exit `2`; invalid declaration → `invalid_schema_declaration` exit `2`. ✅ |
| III `__schema` discoverability | Additive fields in both formats, golden-pinned. ✅ |
| V Asymmetric JSON I/O | Strict minified JSON via `contract.WriteJSON`. ✅ |
| VI Library, not application | No persisted state; resources are static metadata only; annotation keys are `const`; the placeholder scanner is a hand-written loop, not a package-level `*regexp.Regexp`. ✅ |
| VII Test-first | Tests are written before implementation in every task group. ✅ |
| IX Security and resource safety | URI length cap; UTF-8 validation; control-character rejection; no unbounded input. ✅ |
| X Idiomatic Go and minimal deps | Stdlib only; errors as `*contract.Error`; no panic. ✅ |
| XI Stability and SemVer | Additive `omitempty` fields and new symbols; `feat(schema)`; no `breaking-change-approved`. ✅ |

Post-design re-check: no violations, and Complexity Tracking is empty.

## Design

### `internal/schema/declarations.go` (new)

- Internal mirror types `Prompt`, `PromptArgument`, and `Resource`, carrying
  the ax-native JSON tags. These are the annotation encoding.
- `const promptsAnnotationKey`, `resourcesAnnotationKey`.
- `ValidatePrompt(Prompt) *Violation` and `ValidateResource(Resource) *Violation`,
  where `Violation{Field, Reason string}` (research R3). They include a
  hand-written placeholder scanner.
- `AddPrompt(cmd, Prompt) *Violation` and `AddResource(cmd, Resource) *Violation`:
  nil check, validate, decode the existing list, same-command duplicate check,
  append, encode, write. They allocate `Annotations` only on success.
- `Prompts(annotations) []Prompt` and `Resources(annotations) []Resource`:
  decode, then re-validate each entry and drop invalid ones (fail closed).
- `WalkDeclarationCommands(root, visit)`: pre-order over the declaration tree
  (the root, always, plus every command not under a hidden child),
  exactly `BuildCommand`'s pruning, with reserved commands included
  (research R6). It is shared by MCP aggregation and `FindDuplicate`, and it
  matches the recursion of `convertCommandSchema`, so both formats see the
  same set.
- `FindDuplicate(root *cobra.Command) *Conflict`: checks prompts first, then
  resources.
- `FindCorrupt(root *cobra.Command) *Conflict`: the first command whose
  declaration annotation does not project cleanly (FR-004b, research R11).
  `NewSchemaCommand.RunE` checks it before `FindDuplicate`.

### `internal/mcp/mcp.go`

- `Schema` gains `Prompts []internalschema.Prompt` and
  `Resources []internalschema.Resource`. `Build` aggregates them over
  `WalkDeclarationCommands`, keeping the first declaration of each name or URI.

### `schema/schema.go` (public)

- New types `Prompt`, `PromptArgument`, `Resource`, `MCPPrompt`,
  `MCPPromptArgument`, `MCPResource`; new fields on `CommandSchema` and
  `MCPSchema` (data-model.md).
- `DeclarePrompt` and `DeclareResource` convert to the internal types, call
  `Add*`, and map a `Violation` to
  `contract.NewError(context.Background(), "invalid_schema_declaration", msg, WithErrorExitCode(ExitValidation), WithErrorContext(field, reason))`.
- `convertCommandSchema` fills `Prompts` and `Resources`, first-wins across the
  tree to match `BuildMCPSchema`. A `seen` set is threaded through the
  recursive conversion.
- `NewSchemaCommand.RunE` calls `internalschema.FindDuplicate(root)` before
  building either format and returns the `validation_error` envelope (research
  R4).

### Root `ax` (`schema.go`)

- Aliases and wrappers per research R8.

### Tests

- `internal/schema/declarations_test.go`: validation table (every reason),
  annotation preservation, failure leaves annotations untouched, decode
  fail-closed, `FindDuplicate` cases (prompt, resource, hidden-pruned, the same
  name across the prompt and resource namespaces is allowed).
- `schema/declarations_test.go`: public error is `*contract.Error` with the
  code and exit `2`; type-shape reflection test (no func, chan, pointer, map,
  or interface, recursively); capture-by-value mutation test; projection
  fixture (root, non-runnable group, `mcp.Exclude`d leaf, hidden subtree)
  golden in both formats; duplicate `__schema` exits `2` with a golden stderr
  envelope (trace masked) and empty stdout; determinism loop × 10; existing
  goldens unchanged.
- `internal/mcp`: aggregation order and first-wins dedup.
- `internal/mcpserver`: for a declaring tree, `initialize` capabilities carry
  no prompts or resources, and the `tools/list` golden is unchanged.
- `schema/example_test.go`: `ExampleDeclarePrompt`, `ExampleDeclareResource`
  with `// Output:`.

### Docs and example

- `examples/integration/main.go`: one prompt and one resource on the root,
  with the integration goldens updated additively.
- `README.md` `__schema` section; a "Declare prompts and resources" section in
  `docs/src/content/docs/guides/expose-schema.md`; a `schema/doc.go`
  paragraph.
- Never edit `CHANGELOG.md`.

### Gates

- Run `make surface-update`, then review every `baseline.json` line, then
  append audit rows (`-audit-seed`, hand-classified `supported`/`live`).
- Covercheck floors: `internal/schema` 93% and `internal/mcp` 96.9% must hold.

## Project Structure

### Documentation (this feature)

```text
specs/028-mcp-prompts-resources/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/schema-prompts-resources.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (touched)

```text
internal/schema/declarations.go        # NEW
internal/schema/declarations_test.go   # NEW
internal/mcp/mcp.go                    # Schema.Prompts/Resources, Build aggregation
internal/mcp/*_test.go                 # aggregation tests
internal/mcpserver/server_test.go      # capabilities unchanged
schema/schema.go                       # types, fields, Declare*, RunE duplicate check
schema/declarations_test.go            # NEW
schema/example_test.go                 # examples
schema/doc.go                          # docs
schema.go                              # root aliases/wrappers
testdata/schema_ax_declarations.golden.json         # NEW
testdata/schema_mcp_declarations.golden.json        # NEW
testdata/schema_duplicate_declaration.golden.json   # NEW
examples/integration/{main.go,testdata/schema_*.golden.json}
internal/cmd/surfacecheck/baseline.json
specs/023-internalize-helpers/public-surface-audit.json
README.md
docs/src/content/docs/guides/expose-schema.md
```

**Structure Decision**: The existing layout is unchanged. Contract types live in
public `schema/`, mechanics in `internal/schema` and `internal/mcp`, and the
root facade only aliases.

## Risks

- **Shared-file collision**: `baseline.json`, the audit JSON, and the
  integration goldens. No other claim is live (checked 2026-10-07).
- **Audit rows for promoted fields**: the root audit enumerates fields promoted
  through aliases, so `CommandSchema.Prompts` and similar need rows. The seed
  makes that mechanical.
- **Coverage floors**: `internal/schema`'s 93% floor needs full validation
  branch coverage. The table test covers every reason.

## Complexity Tracking

No violations.
