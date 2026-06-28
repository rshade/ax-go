---

description: "Task list for ax-go mcp-server runnable wrapper"
---

# Tasks: ax-go mcp-server runnable wrapper

**Input**: Design documents from `/specs/011-mcp-server-runtime/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: INCLUDED. Constitution Principle VII (Test-First Discipline) is
NON-NEGOTIABLE for this repo — every behavior lands a failing test first.

**Organization**: Tasks are grouped by user story (US1 discovery, US2 execution,
US3 HTTP transport) so each is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3 (setup, foundational, and polish tasks carry no story label)
- Every task names exact file paths.

## Governing ADR(s)

**N/A** — Issue #10 cites ADR-0003, but it was absorbed into
`specs/010-import-isolated-contracts/research.md` and retired in feature 010. No
ADR-retirement task is included (see research.md "Decision Records Absorbed" = N/A).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Bring in the protocol dependency and create the package skeletons.

- [X] T001 Add the MCP Go SDK dependency: add `github.com/modelcontextprotocol/go-sdk` (pin a tagged v1.x release) to `go.mod`, run `go mod tidy`, and verify the version in `specs/011-mcp-server-runtime/research.md` (D1/D11) matches; justify the dependency in the eventual PR description (Principle X).
- [X] T002 [P] Create package skeletons with doc comments: `mcp/doc.go` (public package doc) and `internal/mcpserver/doc.go` (internal engine doc).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Public option types, the public-package governance guard, and import-isolation — all required before any user story.

**⚠️ CRITICAL**: No user story work begins until this phase is complete.

- [X] T003 Implement public option types in `mcp/options.go`: `Option`, `Transport` (`TransportStdio` default, `TransportHTTP`), `WithTransport`, `WithHTTPAddr`, `WithAllowNonLoopback`, `WithVersion`, and the unexported `options` struct with loopback defaults (data-model.md → ServerOptions; contracts/public-api.md).
- [X] T004 Register the new public package in the apidiff gate: add `"github.com/rshade/ax-go/mcp"` to `allowedPackages` in `internal/cmd/apidiff-verdict/main.go` (keep the slice ordering) and update the fixtures/assertions in `internal/cmd/apidiff-verdict/main_test.go` so `check-packages` passes (Principle XI; contracts/public-api.md "Stability / apidiff gate"). Depends on T003.
- [X] T005 [P] Write failing import-isolation test `mcp/import_isolation_test.go` asserting the `mcp` package's import graph stays within the intended set, and confirm `contract`/`config`/`schema`/`id` do not import `mcp` (extend their `*/import_isolation_test.go` only if needed) (contracts/public-api.md C-API-6/7).

**Checkpoint**: Public surface types exist, the apidiff allowlist recognizes `mcp`, and isolation is guarded — user stories can begin.

---

## Phase 3: User Story 1 - Run a CLI as a live MCP server and discover its tools (Priority: P1) 🎯 MVP

**Goal**: A live server that completes `initialize` and answers `tools/list` for the command tree (parity with `__schema --as=mcp`), over stdio, with no per-tool work.

**Independent Test**: Drive an in-memory client↔server over stdio through `initialize` + `tools/list`; assert the tool set equals `schema.BuildMCPSchema(root)` minus hidden/reserved commands and matches the golden file byte-for-byte.

### Tests for User Story 1 (write first, ensure they FAIL)

- [X] T006 [P] [US1] Failing golden test in `internal/mcpserver/server_test.go`: build a fixed Cobra tree, assert `tools/list` equals `testdata/mcp_tools_list.golden.json` and excludes hidden commands plus reserved `__schema`/`mcp-server` — and ONLY those (the fixture intentionally retains the root and any parent/group commands exactly as `schema.BuildMCPSchema` projects them, preserving parity; an adopter suppresses a bare root/parent via `Hidden`) (FR-004/005/006, SC-002/006; contracts/mcp-server.md C-2/3/4).
- [X] T007 [P] [US1] Failing integration test in `mcp/server_test.go`: in-memory client↔server over stdio completes `initialize` (server name + injected version) and `tools/list`, then asserts clean shutdown on context cancellation — `Serve` returns `nil` and telemetry is flushed (FR-003/FR-020; contracts/mcp-server.md C-1/C-19).

### Implementation for User Story 1

- [X] T008 [US1] Implement tool discovery in `internal/mcpserver/server.go`: derive the tool list from `schema.BuildMCPSchema(root)`, excluding hidden commands and reserved `__schema`/`mcp-server` (D8, FR-004/005/006).
- [X] T009 [US1] Implement SDK server construction + `initialize` in `internal/mcpserver/server.go`: `mcp.NewServer` with implementation name = root name and version = injected build version (never `dev`/`unknown`); register discovered tools for `tools/list` (FR-003; contracts/mcp-server.md C-1). Include a test asserting the version contract: `WithVersion` is reflected in `initialize`, and an empty/placeholder (`dev`/`unknown`) version is rejected fail-closed at startup (validation error, exit 2).
- [X] T010 [US1] Implement the stdio transport + lifecycle in `internal/mcpserver/transport.go`: `&mcp.StdioTransport{}` run loop, telemetry start/flush, and clean shutdown on context cancellation/signal (FR-015/020; contracts/mcp-server.md C-14/19).
- [X] T011 [US1] Wire the public entry points in `mcp/server.go` (`Serve(ctx, root, opts...)`, stdio path) and `mcp/command.go` (`NewCommand(root, opts...)` returning the `mcp-server` Cobra command, excluded from its own tool list) (contracts/public-api.md C-API-1/2/5).
- [X] T012 [P] [US1] Add verified `ExampleServe` and `ExampleNewCommand` in `mcp/example_test.go` (primary-API doc coverage; `make doc-coverage`).
- [X] T013 [US1] Generate and commit `testdata/mcp_tools_list.golden.json`; make the T006 golden test pass.

**Checkpoint**: `mcp.Serve`/`mcp.NewCommand` run a stdio server that discovers all non-hidden commands — independently demoable MVP.

---

## Phase 4: User Story 2 - Invoke a command through the server and get its machine payload (Priority: P2)

**Goal**: `tools/call` executes the named command in an isolated, machine-mode invocation, returns the verbatim `stdout` payload on success and the `ax.Error` envelope on failure, honors agent-safety primitives, preserves stream separation, and keeps serving after failures.

**Independent Test**: With the server running, call a tool with valid args and assert the result equals the command's `stdout` payload; call a failing command and assert an `IsError` result carrying the `ax.Error` envelope while the server still answers the next call.

### Tests for User Story 2 (write first, ensure they FAIL)

- [X] T014 [P] [US2] Failing table-driven test in `internal/mcpserver/dispatch_test.go`: argument→flag mapping, machine-mode forcing (FR-026), unknown-tool and invalid-argument validation errors (FR-012; contracts/mcp-server.md C-5/8/11).
- [X] T015 [P] [US2] Failing integration test in `mcp/server_test.go`: `tools/call` success returns verbatim `stdout` text content; a non-zero-exit command returns `IsError` with the `ax.Error` envelope; the server keeps serving after the failure (FR-008/009/010; contracts/mcp-server.md C-6/7/9).
- [X] T016 [US2] Failing test in `internal/mcpserver/dispatch_test.go` for agent-safety passthrough (`dry-run` → `dry_run:true`, resolved `idempotency-key` surfaced) and stream separation (no non-protocol bytes on the protocol channel) (FR-011/013/014, SC-005; contracts/mcp-server.md C-10/12/13). NOT `[P]`: shares `internal/mcpserver/dispatch_test.go` with T014, so author it after/with T014 (it may still run in parallel with T015 and T017, which are different files).
- [X] T017 [P] [US2] Failing fuzz tests in `internal/mcpserver/fuzz_test.go`: argument-object decoding and W3C trace-context extraction (parser surfaces; Principle VII).

### Implementation for User Story 2

- [X] T018 [US2] Implement isolated dispatch in `internal/mcpserver/dispatch.go`: per-call argument vector, output buffers, and `context.Context`; force machine/JSON mode; map MCP arguments → flags; execute via the Cobra root capturing `stdout` (D4, FR-007/026; contracts/mcp-server.md C-5/11).
- [X] T019 [US2] Implement result/error mapping in `internal/mcpserver/dispatch.go`: success → single text content of verbatim `stdout` bytes; non-zero exit → `CallToolResult{IsError:true}` carrying the `ax.Error` envelope; recover panics → internal_error (exit 1) (FR-008/009/010/012; contracts/mcp-server.md C-6/7/8/9).
- [X] T020 [US2] Implement agent-safety passthrough in `internal/mcpserver/dispatch.go`: accept `dry-run`/`idempotency-key` arguments and surface the resolved idempotency key in the result envelope (FR-011; contracts/mcp-server.md C-10).
- [X] T021 [US2] Enforce stream separation in `internal/mcpserver/dispatch.go`/`server.go`: capture command `stdout` to the result buffer, route command `stderr` and server logs to `stderr`, keep the protocol channel protocol-only (FR-013/014, SC-005; contracts/mcp-server.md C-12/13).
- [X] T022 [US2] Implement per-call observability in `internal/mcpserver/trace.go`: extract W3C trace context from the request when present (else fresh root trace), one span per call, `trace_id`/`span_id` on log lines emitted while serving (FR-025; contracts/mcp-server.md C-18).
- [X] T023 [US2] Register the `tools/call` handler with the SDK server in `internal/mcpserver/server.go` so calls route to the dispatch path; make T014–T017 pass.

**Checkpoint**: Discovery + execution both work over stdio; failures are structured and non-fatal.

---

## Phase 5: User Story 3 - Serve the same tools over an HTTP transport (Priority: P3)

**Goal**: The identical tool set and execution behavior over a streamable HTTP transport, loopback by default (fail-closed against accidental public exposure), race-free under concurrency.

**Independent Test**: Start the server in HTTP mode on a loopback test address; assert `initialize`/`tools/list`/`tools/call` behave identically to stdio; assert a non-loopback bind without opt-in fails closed; assert concurrent calls are isolated under `-race`.

### Tests for User Story 3 (write first, ensure they FAIL)

- [X] T024 [P] [US3] Failing test in `internal/mcpserver/transport_test.go`: HTTP transport parity with stdio (initialize/list/call); loopback-default bind; non-loopback bind without `WithAllowNonLoopback` fails closed with a validation error (exit 2) (FR-016/017/018, SC-007; contracts/mcp-server.md C-15/16/17).
- [X] T025 [P] [US3] Failing concurrency test in `internal/mcpserver/concurrency_test.go` (run with `-race`): simultaneous HTTP `tools/call` invocations are isolated and race-free (FR-021; data-model INV-5); and a shutdown-during-in-flight-calls case asserting in-flight calls drain deterministically (observe canceled context, return promptly, no protocol-channel corruption) and `Serve` returns `nil` (FR-020; contracts/mcp-server.md C-19/C-20).

### Implementation for User Story 3

- [X] T026 [US3] Implement the HTTP transport in `internal/mcpserver/transport.go`: SDK `StreamableHTTPHandler`, loopback-default bind, `WithAllowNonLoopback` enforcement, secure defaults (no TLS-verify disabling), no credentials held (FR-016/018; contracts/mcp-server.md C-15/17).
- [X] T027 [US3] Wire the HTTP path into the public surface: add `--transport`, `--addr`, and `--allow-non-loopback` flags to the `mcp-server` command in `mcp/command.go` and resolve them through the options in `mcp/server.go` (FR-016/017; quickstart.md §3).
- [X] T028 [US3] Guarantee dispatch is race-free under concurrent HTTP (no shared mutable command/flag state across calls); make T025 pass (FR-021).

**Checkpoint**: stdio and HTTP expose identical behavior; HTTP is safe-by-default.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Adopter integration, docs, and the full quality gate.

- [X] T029 [P] Mount `mcp.NewCommand(root)` in `examples/integration/main.go`, add a server smoke test (`examples/integration/main_test.go`), and note `integration mcp-server` usage in `examples/integration/README.md` (FR-022; Constitution Development Workflow).
- [X] T030 [P] Document `mcp-server` in `README.md`: stdio + HTTP usage, loopback safety, and in/out-of-scope (no auth flow, no incremental streaming, no standalone launcher); run markdownlint (FR-022; quickstart.md).
- [X] T031 [P] Ensure every exported `mcp` symbol carries a contract-style doc comment (godoclint `require-doc`) and confirm `make doc-coverage` passes for the primary API.
- [X] T032 Run the full gate clean: `gofmt`, `go vet ./...`, `golangci-lint run`, `go test -race ./...`, `make doc-coverage`, `make cover-check` (new `mcp` and `internal/mcpserver` packages meet the 25% per-package floor; raise/override in `internal/cmd/covercheck/main.go` only if calibration requires), and `go run ./internal/cmd/apidiff-verdict check-packages`.
- [X] T033 Validate `specs/011-mcp-server-runtime/quickstart.md` end-to-end against the built `examples/integration` binary (initialize → tools/list → tools/call over stdio and loopback HTTP).

> No ADR-retirement task: this feature is governed by no ADR (see "Governing ADR(s)" above).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup; BLOCKS all user stories. T004 depends on T003.
- **User Stories (Phase 3–5)**: All depend on Foundational. US2 depends on US1's server core (`internal/mcpserver/server.go`); US3 depends on US1's server core and reuses US2's dispatch. They are best done in priority order (P1 → P2 → P3) because they extend shared files, though each is independently testable once its phase completes.
- **Polish (Phase 6)**: Depends on the user stories targeted for the release.

### Within Each User Story

- Tests (the `### Tests` block) are written FIRST and must FAIL before implementation.
- Discovery/server core before dispatch; dispatch before transport variants.
- Story complete (its checkpoint) before moving to the next priority.

### Parallel Opportunities

- T002 (after T001) runs alongside later setup.
- T005 [P] runs alongside T003/T004 (different files).
- US1 tests T006/T007 [P] together; US2 tests run T014, T015, and T017 [P] together, with T016 sequenced after T014 (both edit `internal/mcpserver/dispatch_test.go`); US3 tests T024/T025 [P] together.
- Polish T029/T030/T031 [P] together.
- Different developers can take US2 and US3 in parallel once US1's server core lands, coordinating on `internal/mcpserver/server.go`.

---

## Parallel Example: User Story 2

```bash
# Launch the different-file US2 tests together (write first, must fail):
Task: "Table-driven dispatch test in internal/mcpserver/dispatch_test.go (T014)"
Task: "Integration call success/failure test in mcp/server_test.go (T015)"
Task: "Fuzz tests in internal/mcpserver/fuzz_test.go (T017)"
# Then, in the same file as T014 (sequential, NOT parallel with it):
Task: "Agent-safety + stream-separation test in internal/mcpserver/dispatch_test.go (T016)"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1 (Setup), Phase 2 (Foundational), then Phase 3 (US1).
2. **STOP and VALIDATE**: a stdio server that discovers all tools (initialize + tools/list, golden-guarded).
3. Demo: connect any MCP client and list `mycli`'s tools.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → live discovery over stdio (MVP).
3. US2 → executable tool calls with structured errors + safety + tracing.
4. US3 → HTTP transport parity, safe-by-default.
5. Polish → adopter example, docs, full gate.

---

## Notes

- [P] = different files, no incomplete dependencies.
- Verify each `### Tests` block FAILS for the right reason before implementing (Principle VII).
- `go test -race ./...` is required, not optional — concurrency is core to US3.
- The apidiff allowlist update (T004) is mandatory governance: skipping it fails CI.
- Commit after each task or logical group; ship as a pre-v1.0 minor (`feat:`).
