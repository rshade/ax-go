# Implementation Plan: Preserve Command Stdout on MCP Non-Zero Exit

**Branch**: `035-mcp-error-stdout` | **Date**: 2026-11-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/035-mcp-error-stdout/spec.md`

## Summary

Preserve a dispatched command's captured `stdout` when it exits non-zero under `mcp-server`. In `internal/mcpserver/dispatch.go`, `dispatcher.errorResult` accepts the captured `stdout` byte slice alongside the error: it always places the marshaled `ax.Error` envelope in `Content[0]`, and when `len(stdout) > 0`, it appends a second `*sdk.TextContent{Text: string(stdout)}` at `Content[1]` with the command's verbatim `stdout` bytes. When `len(stdout) == 0` (including pre-dispatch validation errors and commands that fail without writing to `stdout`), `errorResult` returns the existing single-item result. Panic recovery in `runRecovered` continues to reset `outBuf` so partial `stdout` from a panicking command is discarded. Update the dispatch contract documentation (`internal/mcpserver/doc.go`, `mcp/doc.go`, `mcp/server.go`, `README.md`, `docs/src/content/docs/guides/expose-schema.md`) and pin the two-item error result wire format with a new golden fixture in `testdata/`.

## Technical Context

**Language/Version**: Go 1.27.4 (pinned in `mise.toml`; `go.mod` matches)

**Primary Dependencies**: `github.com/modelcontextprotocol/go-sdk` v1.8.0 (existing), `github.com/spf13/cobra` (existing), stdlib. Zero new dependencies.

**Storage**: N/A. Per-call `bytes.Buffer` in memory only (Constitution Principle VI).

**Testing**: `go test -race` across the 4-tag matrix (`make test`), table-driven and unit tests in `internal/mcpserver/dispatch_test.go`, end-to-end HTTP session test in `mcp/server_test.go`, and a live server golden test in `internal/mcpserver/golden_test.go` pinning `testdata/mcp_tools_call_error_stdout.golden.json`.

**Target Platform**: All six `surfacecheck` `GOOS`/`GOARCH` profiles; no platform-specific code.

**Project Type**: Go library (confined MCP engine in `internal/mcpserver`, public facade in `mcp`).

**Performance Goals**: None asserted. Appending one pointer to a 2-element slice on the non-zero-exit-with-stdout path adds negligible overhead and does not touch any tracked benchmark in `benchcheck`.

**Constraints**: Preserve stream separation (command `stderr` stays on the server's `stderr` writer and never enters `CallToolResult.Content`); preserve `stdout` byte-for-byte without re-parsing or re-serializing; discard partial `stdout` on panic recovery; keep all existing golden files unchanged; zero changes to exported Go identifiers or signatures.

**Scale/Scope**: One internal method signature update (`dispatcher.errorResult`) and its call sites in `internal/mcpserver/dispatch.go`, unit/integration/golden tests, one new golden file, and documentation updates across 5 files.

**Governing ADR(s)**: N/A. Neither `docs/adr/0004-trace-id-format.md` nor `docs/adr/0008-cli-framework-cobra.md` governs MCP tool error result formatting.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Basis |
| --- | --- | --- |
| I. Stream Separation | Pass | Captured `stdout` is returned only inside the MCP `CallToolResult` on the protocol channel; captured `stderr` continues to be forwarded exclusively to `Config.Stderr` and never leaks into `Content[0]` or `Content[1]`. |
| II. Deterministic Output & Exit Codes | Pass | Captured `stdout` bytes are wrapped verbatim (`string(stdout)`) without re-parsing or re-marshaling, preserving exact field order and formatting. |
| III. `__schema` Discoverability | Pass | `__schema` and `__schema --as=mcp` are untouched; all existing schema goldens remain byte-identical. |
| IV. Agent-Safety Primitives | Pass | `--dry-run`, `--yes`, and `--idempotency-key` passthrough are untouched. |
| V. Asymmetric JSON I/O | N/A | Config read/patch paths are untouched. |
| VI. Library, Not Application | Pass | Stateless per-call buffer handling in `internal/mcpserver`; no persistence or globals. |
| VII. Test-First Discipline | Pass | Failing unit, integration, and golden tests are written before modifying `dispatch.go`. |
| VIII. Observability & ID Discipline | Pass | W3C trace context propagation into `ax.Error` (`Content[0]`) and the command's envelope (`Content[1]`) is preserved and verified by the golden test. |
| IX. Security & Resource Safety | Pass | Panic recovery in `runRecovered` still resets `outBuf`, preventing partial/corrupt buffers from leaking on panic. |
| X. Idiomatic Go & Dependency Minimalism | Pass | Zero new dependencies; changes confined to `internal/mcpserver` and docs. |
| XI. Stability & SemVer | Pass | Additive machine-contract enhancement (`Content[0]` is unchanged; `Content[1]` is appended only when `len(stdout) > 0`); zero exported Go API changes (`surface-check` baseline unchanged). Rides a `feat(mcp):` minor bump. |
| XII. Deprecation Lifecycle | N/A | Nothing deprecated or removed. |

**ADR absorption gate (Constitution §Governance)**: Governing ADR(s) is N/A; no ADR retirement task applies.

**Post-design re-check**: Pass. No new packages, exported symbols, or dependencies introduced.

## Project Structure

### Documentation (this feature)

```text
specs/035-mcp-error-stdout/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── mcp-error-stdout.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks command)
```

### Source Code (repository root)

```text
internal/mcpserver/dispatch.go       # errorResult(ctx, err, stdout) appends Content[1] when len(stdout) > 0
internal/mcpserver/dispatch_test.go  # tests for non-zero exit with stdout, empty stdout, panic with partial stdout, stream separation
internal/mcpserver/golden_test.go    # golden test pinning live tools/call error result with stdout
internal/mcpserver/doc.go            # package doc contract update
mcp/server_test.go                   # public Serve end-to-end test for failing command with stdout
mcp/doc.go                           # package doc contract update
mcp/server.go                        # Serve doc comment update
testdata/mcp_tools_call_error_stdout.golden.json # new golden fixture
README.md                            # MCP execution bullet update
docs/src/content/docs/guides/expose-schema.md # docs site guide update
```

**Structure Decision**: All runtime changes live in `internal/mcpserver/dispatch.go`, with tests in `internal/mcpserver/` and `mcp/`, and contract documentation updated in `internal/mcpserver/doc.go`, `mcp/doc.go`, `mcp/server.go`, `README.md`, and `docs/src/content/docs/guides/expose-schema.md`.

## Complexity Tracking

No constitution violations to justify.
