# Implementation Plan: Serve Declared MCP Prompts, Resources, and Instructions on the Live Server

**Branch**: `030-serve-mcp-prompts-resources` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/030-serve-mcp-prompts-resources/spec.md`

## Summary

Register the prompts and resources that spec 028 lets a CLI declare on the live
`mcp-server`, using the same aggregation `schema.BuildMCPSchema` uses
(`schema.CollectDeclarations`) so `prompts/list`, `resources/list` and
`__schema --as=mcp` cannot diverge. `prompts/get` renders a template with one
single-pass scan that shares its placeholder grammar with declaration-time
validation. `resources/read` returns static content that the resource
declaration now carries as an additive `Content` text field; the field is
excluded from every `__schema` projection. A new `mcp.WithInstructions` option
flows through `mcpserver.Config` to the SDK's `ServerOptions.Instructions`,
validated at startup like `WithVersion`. Prompts and resources are registered
only when declared, so a CLI that declares nothing keeps a byte-identical
handshake.

## Technical Context

**Language/Version**: Go 1.27.1 (pinned in `mise.toml`; `go.mod` matches)

**Primary Dependencies**: `github.com/modelcontextprotocol/go-sdk` v1.8.0
(already required: `Server.AddPrompt`, `Server.AddResource`,
`ServerOptions.Instructions`), `github.com/spf13/cobra`, stdlib. No new module
dependencies.

**Storage**: N/A. Content is held in Cobra annotations and in memory; nothing
is persisted (Constitution VI).

**Testing**: `go test -race` across the four-tag matrix, table-driven render and
validation tests, in-memory SDK client integration tests over stdio and HTTP,
golden files for `prompts/get`, `resources/read`, and `initialize`, a fuzz test
for render and prompt arguments, a reflection type-shape test extended to the
new field, and a determinism loop.

**Target Platform**: All six surfacecheck GOOS/GOARCH profiles; no
platform-specific code.

**Project Type**: Go library (public packages `schema`, `mcp`; confined engine
`internal/mcpserver`).

**Performance Goals**: None asserted. Registration is a one-time startup walk.
`BenchmarkBuildCommand` (`__schema` reflection) is a tracked hot path and must
stay within budget since `schema` declarations change; run `make bench-check`.

**Constraints**: No new `stdout` writes outside the protocol channel; default
build and all three build-tag configurations behave identically (parity tests
live in untagged files); `schema` and the other contract packages remain
import-isolated; `mcp` stays non-isolated.

**Scale/Scope**: Content cap 1 MiB, template cap 64 KiB, instructions cap
8 KiB, prompt argument value cap 64 KiB (see research.md R-004).

**Governing ADR(s)**: N/A. ADRs are frozen; spec 028's decisions carry forward.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Basis |
| --- | --- | --- |
| I. Stream Separation | Pass | Rendered prompts, resource bodies, and instructions travel only in the protocol channel. Debug logging of instructions goes to `stderr`. A stdout-cleanliness test covers a full prompt/resource session. |
| II. Deterministic Output | Pass | Rendering is a pure single-pass function; list order is the walk order `BuildMCPSchema` uses; goldens and a repeat-call test pin it. |
| III. `__schema` Discoverability | Pass | `__schema` and `--as=mcp` are unchanged; content is unprojected, enforced by `json:"-"` on the public field and existing goldens. |
| IV. Agent-Safety | Pass | The server renders text and never executes it; no mutation, so `--yes`/`--dry-run` do not apply. |
| V. Asymmetric JSON I/O | N/A | No config read path changes. |
| VI. Library, Not Application | Pass | Static content only, captured by value at declaration; no callbacks, no persistence, no orchestration. Dynamic content stays out. |
| VII. Test-First | Pass | Tasks order tests before implementation; fuzz test for render and arguments. |
| VIII. Observability & IDs | Pass | No new IDs or labels. |
| IX. Security & Resource Safety | Pass | New caps on template, content, instructions, and argument values; UTF-8 validated; fail closed. |
| X. Idiomatic Go | Pass | Functional option, no globals, no `any`, no new dependency. |
| XI. Stability & SemVer | Pass, with note | Additive: one exported field, one exported option. `0.MINOR` `feat`. Template and content caps tighten validation for templates larger than 64 KiB, which would have been valid in v0.8.0; the cap is generous and the tightening is called out in the commit body. |
| XII. Deprecation | N/A | Nothing deprecated. |

**ADR absorption gate**: Governing ADR is N/A, so no retirement task applies.

**Post-design re-check**: Pass. The design adds no new package, no new public
subpackage, and no dependency.

## Project Structure

### Documentation (this feature)

```text
specs/030-serve-mcp-prompts-resources/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── mcp-live-server.md
├── checklists/requirements.md
└── tasks.md             # /speckit-tasks output
```

### Source Code (repository root)

```text
internal/schema/declarations.go     # Resource.Content, caps, RenderTemplate sharing the placeholder scanner, TreeDeclarationError (moved from schema)
internal/schema/declarations_test.go
internal/mcpserver/server.go        # Config.Instructions, ServerOptions, register prompts/resources
internal/mcpserver/prompts.go       # new: prompts/get handler (arguments check, render)
internal/mcpserver/resources.go     # new: resources/read handler (static content)
internal/mcpserver/*_test.go        # unit, integration (stdio + HTTP), fuzz, parity
mcp/options.go                      # WithInstructions
mcp/server.go                       # thread the option into Config
mcp/doc.go                          # drop "does not serve prompts yet"
schema/declarations.go              # public Resource.Content (json:"-"), docs, explicit MCPResource conversion
testdata/                           # new goldens: prompts_get, resources_read, initialize_*
examples/integration/               # declared prompt, resource with content, WithInstructions; goldens additive
internal/cmd/surfacecheck/baseline.json   # intentional additions (review every line)
README.md
```

**Structure Decision**: No new package. Server mechanics stay confined to
`internal/mcpserver`, per `AGENTS.md`; the declaration field and its validation
live in `internal/schema` with the public `schema` mirror, so render and
validation share one placeholder grammar.

## Complexity Tracking

No constitution violations to justify.
