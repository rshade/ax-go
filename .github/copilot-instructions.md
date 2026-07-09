# Copilot instructions for ax-go

`ax-go` (module `github.com/rshade/ax-go`, package `ax`) is the Agentic Experience
foundation for Go CLIs: it makes Go command-line tools predictable for LLM agents while
staying ergonomic for humans. It targets **Go 1.26.5**.

## Source of truth (read before flagging a "divergence")

Authority runs **constitution → feature spec → agent docs → code**:

1. `.specify/memory/constitution.md` is supreme and governs all behavior.
2. The active feature's `specs/<feature>/` directory (`spec.md`, `research.md`,
   `data-model.md`, `contracts/`) defines intended behavior. **The spec, not a value
   used only in a test or example, defines the contract.** Do not infer a documented
   default or rule from a literal that appears solely in `*_test.go` or an `Example`.
3. `AGENTS.md` (imported by `CLAUDE.md`/`GEMINI.md`) holds repo-wide engineering rules.
4. Behavior is pinned by **executable** `ExampleXxx` functions (with `// Output:`) and
   golden files that `go test` runs in CI. Before claiming code "won't compile" or
   "breaks an example", run the suite (below) — if it passes, the claim is wrong.

## Go version facts (avoid stale-Go false positives)

This is modern Go (1.26.5). The following are valid and must not be flagged as errors:

- `for i := range n` where `n` is an integer ranges `i` from `0` to `n-1` (Go 1.22+).
- Generics, the `min`/`max`/`clear` builtins, and `errors.Join` are available.
- `ListenConfig.Listen` on a numeric loopback address does not fail merely because the
  passed context is already canceled; a canceled context is treated as clean shutdown.

## Core AX mandates (the machine-facing contract)

- **Stream separation is absolute.** `stdout` carries only the final machine payload;
  logs, progress, diagnostics, and error envelopes go to `stderr`. Never write
  non-payload bytes to `stdout`.
- **Exit codes are deterministic:** `0` success, `1` unknown/internal, `2`
  validation/bad input, `3` network/timeout, `4` auth/permission.
- **Output is deterministic.** Same input → byte-identical `stdout`, except documented
  non-deterministic fields (timestamps, `trace_id`, auto-generated `idempotency_key`).
  Use structs (not maps) for envelopes; RFC 3339 UTC timestamps; never `float64` for IDs
  or money.
- **Errors** use the `ax.Error` / `contract.NewError` envelope, emitted to `stderr`.
- **Agent-safety primitives:** `--idempotency-key` (auto UUID v4 when absent, surfaced in
  the envelope), `--dry-run` (same envelope, `dry_run: true`, no side effects), and mode
  resolution by precedence `--format` → `AGENT_MODE` → TTY detection.
- Every CLI exposes `__schema`; `__schema --as=mcp` emits MCP-tool output.

## Go conventions enforced in this repo

- `context.Context` is the **first parameter** of any function doing I/O, outbound calls,
  goroutines, or cancelable work.
- Wrap errors with `%w` (never `%s`/`%v`); `errors.Is`/`errors.As` must work.
- **No `panic` in library code** — return errors. No mutable package-level state and no
  `init()` that mutates globals, reads files/env, or makes network calls.
- Avoid `any`/`interface{}`; prefer a concrete or tightly scoped type (comment when
  genuinely needed). `defer Close()` every `io.Closer`. Functional options for
  constructors with several knobs.
- Secure defaults only: never set `InsecureSkipVerify: true`; cap unbounded reads
  (Hujson config defaults to 1 MiB); never log PII, secrets, or tokens, and never build
  log messages from un-sanitized user strings (use zerolog field methods).
- Reads accept Hujson; **writes emit strict, minified JSON** (Hujson cannot marshal
  comments). NDJSON for streaming/unbounded output.
- `internal/` is private and toolchain-enforced. The public surface is the root package
  `ax` plus the contract packages `contract`, `config`, `schema`, and `id`; only those
  are gated by API-diff (`internal/` is exempt).

## How to validate (run before raising correctness concerns)

```bash
go test -race ./...        # make test — race detector is required
make validate              # gofmt -s, go mod tidy -diff, go vet ./...
make lint                  # golangci-lint v2.12.2 + markdownlint + actionlint
make doc-coverage          # ExampleXxx coverage on the primary API
make cover-check           # per-package + repo-wide coverage floors
make ci                    # test + validate + lint + doc-coverage
```

`CHANGELOG.md` is owned by release-please from Conventional Commits — never hand-edit it.
Capture user-facing changes in the commit message (`feat:`, `fix:`, `feat!:`).

`ROADMAP.md` is maintained by the `/roadmap` sync workflow and reconciled to GitHub
issue/label state out-of-band, not in feature PRs. Do not flag scope, status, or
"documentation drift" in it, and do not suggest edits to it in review.
