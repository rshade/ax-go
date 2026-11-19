# Implementation Plan: Per-Flag MCP Exclusion (`mcp.ExcludeFlag`)

**Branch**: `036-mcp-flag-exclusion` | **Date**: 2026-11-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/036-mcp-flag-exclusion/spec.md`

## Summary

Add `mcp.ExcludeFlag(cmd *cobra.Command, name string)` so a CLI author can keep a command as a callable MCP tool while omitting a flag that cannot work over request/response (such as `--watch` in `github.com/rshade/tsacl`). `mcp.ExcludeFlag` delegates to `internal/mcp.MarkFlagExcluded(cmd, name)`, which looks up `name` in `cmd.Flags()` and then `cmd.PersistentFlags()` and sets the pflag annotation `"github.com/rshade/ax-go/mcp/exclude"` (`internalmcp.ExcludeAnnotationKey`) to `[]string{"true"}`. `internal/mcp.IsFlagExcluded(flag)` fails closed (`len(vals) == 1 && vals[0] == "true"`). Both `internal/mcp.collectFlags` (which feeds static `__schema --as=mcp` and live `mcp-server` `tools/list` `inputSchema.properties` and `inputSchema.required`) and `internal/mcpserver.acceptedFlags` (which feeds `buildArgs` and `missingRequiredFlag` during `tools/call`) record the flag name as seen and then skip any flag for which `IsFlagExcluded(flag)` is `true`. As a result, excluded flags vanish from MCP tool schemas, any `tools/call` supplying an excluded flag is rejected in `buildArgs` with `validation_error` (exit `2`) before the command runs, and the flag stays untouched in `--help`, direct CLI execution, and `__schema --as=ax`.

## Technical Context

**Language/Version**: Go, at the version pinned in `mise.toml` (`go.mod` must match)

**Primary Dependencies**: `github.com/spf13/cobra` (existing), `github.com/spf13/pflag` (existing), `github.com/modelcontextprotocol/go-sdk` v1.8.0 (existing). Zero new dependencies.

**Storage**: N/A. Flag annotations live on the in-memory `*pflag.Flag.Annotations` map on the Cobra command tree (Constitution Principle VI).

**Testing**: `go test -race` across the 4-tag matrix (`make test`), unit tests in `internal/mcp/exclude_test.go`, `internal/mcp/input_schema_test.go`, `internal/mcpserver/dispatch_test.go`, and end-to-end static/live parity, `tools/call` rejection, and `--help` / `__schema --as=ax` preservation tests in `mcp/exclude_test.go` plus `ExampleExcludeFlag` in `mcp/example_test.go`.

**Target Platform**: All six `surfacecheck` `GOOS`/`GOARCH` profiles; no platform-specific code.

**Project Type**: Go library (public facade in `mcp`, shared schema builder in `internal/mcp`, live MCP engine in `internal/mcpserver`).

**Performance Goals**: None asserted. `IsFlagExcluded` is a single map lookup on `flag.Annotations` during schema reflection and tool dispatch (`flag.Annotations` is `nil` for unannotated flags, so the check is zero-allocation).

**Constraints**: Root `ax` and the import-isolated packages (`contract`, `config`, `schema`, `id`, `logging`) gain zero new exported symbols (FR-008); `mcp/import_isolation_test.go` stays green; static `__schema --as=mcp` and live `mcp-server` stay in lock-step; all existing golden files remain byte-identical.

**Scale/Scope**: One new exported function `mcp.ExcludeFlag` in `mcp/exclude.go`, two helpers (`IsFlagExcluded`, `MarkFlagExcluded`) and a filter in `internal/mcp/mcp.go`, a matching filter in `internal/mcpserver/dispatch.go`, surface inventory update (`internal/cmd/surfacecheck/baseline.json`), tests, runnable example, and documentation updates across `mcp/doc.go`, `README.md`, and `docs/src/content/docs/guides/expose-schema.md`.

**Governing ADR(s)**: N/A. Neither `docs/adr/0004-trace-id-format.md` nor `docs/adr/0008-cli-framework-cobra.md` governs per-flag MCP exclusion.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Basis |
| --- | --- | --- |
| I. Stream Separation | Pass | Rejected excluded-flag calls return `CallToolResult{IsError: true}` over the MCP protocol channel; no diagnostic leaks to `stdout`. |
| II. Deterministic Output & Exit Codes | Pass | Rejected excluded-flag calls produce a deterministic `validation_error` `ax.Error` envelope with exit code `2`. Schema property and `required` ordering remain deterministic. |
| III. `__schema` Discoverability | Pass | `internal/mcp.collectFlags` backs both `__schema --as=mcp` and `mcp-server` `tools/list`, keeping both surfaces identical. `__schema --as=ax` is untouched. |
| IV. Agent-Safety Primitives | Pass | `--dry-run`, `--yes`, `--format`, and `--idempotency-key` passthrough are untouched. |
| V. Asymmetric JSON I/O | N/A | Config read/patch paths are untouched. |
| VI. Library, Not Application | Pass | Annotation is stored on the caller's `*pflag.Flag` instance; no package-level mutable state. |
| VII. Test-First Discipline | Pass | Failing unit, integration, and example tests are written before implementation; `ExampleExcludeFlag` covers the new public entry point. |
| VIII. Observability & ID Discipline | Pass | Trace context propagation into `validation_error` envelopes is preserved. |
| IX. Security & Resource Safety | Pass | Excluded flags are rejected in `buildArgs` before `d.execute`, so a blocking flag like `--watch` can never run or hold the dispatcher lock. |
| X. Idiomatic Go & Dependency Minimalism | Pass | Zero new dependencies; reuses `ExcludeAnnotationKey` (`"github.com/rshade/ax-go/mcp/exclude"`). |
| XI. Stability & SemVer | Pass | Purely additive public symbol (`mcp.ExcludeFlag`) in package `mcp`; existing schemas for commands that do not call `mcp.ExcludeFlag` are unchanged. Rides a `feat(mcp):` minor bump. |
| XII. Deprecation Lifecycle | N/A | Nothing deprecated or removed. |

**ADR absorption gate (Constitution §Governance)**: Governing ADR(s) is N/A; no ADR retirement task applies.

**Post-design re-check**: Pass. Only `mcp.ExcludeFlag` is added to the public API surface (`mcp` package); `internal/cmd/surfacecheck/baseline.json` is updated intentionally via `make surface-update`.

## Project Structure

### Documentation (this feature)

```text
specs/036-mcp-flag-exclusion/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── mcp-flag-exclusion.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks command)
```

### Source Code (repository root)

```text
mcp/exclude.go                                 # ExcludeFlag(cmd *cobra.Command, name string)
mcp/exclude_test.go                            # static/live parity, tools/call rejection, --help and --as=ax preservation, nil/unknown no-op
mcp/example_test.go                            # ExampleExcludeFlag
mcp/doc.go                                     # package doc update for ExcludeFlag
internal/mcp/mcp.go                            # IsFlagExcluded, MarkFlagExcluded, collectFlags exclusion filter
internal/mcp/exclude_test.go                   # unit tests for IsFlagExcluded and MarkFlagExcluded
internal/mcp/input_schema_test.go              # unit tests for excluded local, required, persistent, and shadowed flags in inputSchema
internal/mcpserver/dispatch.go                 # acceptedFlags exclusion filter (imports internal/mcp)
internal/mcpserver/dispatch_test.go            # dispatcher unit tests for excluded flag rejection and excluded required flag allowance
internal/cmd/surfacecheck/baseline.json        # updated exported surface inventory for mcp.ExcludeFlag
README.md                                      # MCP discovery & execution documentation update
docs/src/content/docs/guides/expose-schema.md  # Excluding commands and flags guide section update
```

**Structure Decision**: `mcp.ExcludeFlag` lives alongside `mcp.Exclude` in `mcp/exclude.go`, delegating to `internal/mcp` for annotation storage and schema filtering and `internal/mcpserver` for dispatch filtering.

## Complexity Tracking

No constitution violations to justify.
