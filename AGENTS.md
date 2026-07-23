# Repository Guidelines

## Project Overview

`ax-go` is the Agentic Experience foundation for Go CLI tools in the
`rshade` portfolio. Its purpose is to make Go CLIs predictable for LLM agents
and still ergonomic for human engineers.

The module is `github.com/rshade/ax-go`, the package name should be `ax`, and
the project currently targets Go `1.26.5`. The canonical source of truth for
behavior and public API decisions is the constitution at
`.specify/memory/constitution.md`. The ADRs in `docs/adr/` are a FROZEN legacy
decision log being retired through the Spec Kit feature workflow; where an ADR
conflicts with the constitution, the constitution wins.

## Repository Layout

- `go.mod` declares the module and Go version.
- `README.md` explains the project mission, standards, and roadmap.
- `.specify/memory/constitution.md` is the canonical, supreme governance
  document; `docs/adr/`, this file, `CONTEXT.md`, `GEMINI.md`, and `CLAUDE.md`
  are derived and reconciled to it.
- `docs/adr/` contains a FROZEN legacy log of Architecture Decision Records. Do
  not create or edit ADRs. When a Spec Kit feature is governed by one, absorb its
  decisions into the feature's `research.md` and delete it as the final task.
- `internal/` contains private implementation packages behind the public
  package `ax` and the narrow public contract packages. Do not move code into
  additional public subpackages without a Spec Kit feature or constitution
  amendment.
- `contract/`, `config/`, `schema/`, and `id/` are approved public contract
  packages for thin consumers. They must remain import-isolated from the root
  runtime facade, telemetry exporters/SDK setup, logger/Loki, HTTP
  instrumentation, and gRPC runtime adapters.
- `logging/` is an approved public package and the second import-isolated
  surface: the trace-correlated zerolog logger over `internal/logcore`, exactly
  as `mcp` sits over `internal/mcpserver` — except that unlike `mcp`, `logging`
  **is** import-isolated. Its forbidden set differs from the four contract
  packages above: `zerolog` and the OTel trace **API** are required here (they
  appear in `Logger`'s method set and provide trace correlation), while
  `net/http` and `crypto/tls` are forbidden. That is why
  `internal/testutil/imports.go` carries a per-surface rule set
  (`ForbiddenLoggingImports`) rather than one shared list. A logging-only
  consumer links 103 packages and ~2.26 MB stripped, against 410 packages and
  ~12.0 MB through root `ax`. Loki direct push and `Execute` stay root-only,
  because both need dependencies the isolation exists to exclude.
- `mcp/` is an approved public package: the thin MCP server runtime surface
  (`mcp.Serve`, `mcp.NewCommand`) over `internal/mcpserver`, which confines
  the MCP Go SDK and all protocol mechanics. It belongs to the apidiff-gated
  public surface but is not an import-isolated contract package; the contract
  packages must never import it (enforced by `mcp/import_isolation_test.go`).
- `testdata/` contains golden fixtures for stable public JSON contracts.
- `cmd/` is reserved for runnable support binaries and does not exist yet.
  The MCP server runtime is not a `cmd/` launcher: spec 011 shipped
  `mcp.NewCommand`, the reserved `mcp-server` subcommand an adopting CLI
  mounts itself. Do not create placeholder commands before behavior exists.
- Do not add `pkg/` or `src/`; public package `ax` lives at the module root
  to preserve the `github.com/rshade/ax-go` import path.
- `examples/integration/` contains the runnable integration command for
  exercising the public API from a real Cobra CLI. Keep it current when
  public behavior or examples change.
- `AGENTS.md` is the canonical agent instruction file. `GEMINI.md` and
  `CLAUDE.md` should import this file rather than duplicate guidance.

## Core AX Mandates

- Stream separation is non-negotiable:
  - `stdout` is reserved for the final machine payload.
  - `stderr` is used for logs, progress, diagnostics, and structured error
    envelopes.
- Exit codes are deterministic:
  - `0`: success
  - `1`: unknown/internal error
  - `2`: validation/bad input
  - `3`: network/timeout
  - `4`: authentication/permission
- Every CLI built on ax-go must expose `__schema`, emitting a structured JSON
  description of command tree, flags, types, and examples. A companion
  `__schema --as=mcp` adapter emits MCP-tool-compatible output, and the `mcp`
  package's reserved `mcp-server` subcommand (`mcp.NewCommand`) runs the same
  command tree as a live MCP server with no per-tool work.
- The `non_deterministic_fields` enumeration in `__schema` output is the
  authoritative source of truth for fields an agent may safely ignore when
  diffing two runs. Agents must not infer or maintain a separate mask list.
- Input accepts Hujson for human convenience on **reads only** — writes emit
  strict JSON (Hujson cannot Marshal comments). To mutate an existing Hujson
  file while preserving user formatting, use the AST `Patch` path
  preserved in `specs/001-bound-config-reads/research.md`. Output emits strict,
  minified JSON for bounded payloads and NDJSON for streaming or unbounded
  result sets (absorbed into specs/006-output-determinism-harness/research.md).
- All commands must support agent-safety primitives:
  - `--idempotency-key`, auto-generating UUID v4 when absent and surfacing the
    key in the output envelope.
  - `--dry-run`, producing the same envelope with `dry_run: true` and no side
    effects.
  - output mode resolution through `--format`, `AGENT_MODE`, and TTY detection.
- Errors use the standard `ax.Error` envelope and are emitted to `stderr`.
- **Output is deterministic.** Given the same inputs, two runs of the same
  command produce byte-identical `stdout` payloads, modulo fields documented
  as non-deterministic (timestamps, `trace_id`, auto-generated
  `idempotency_key`). Use structs (not maps) for any envelope shape; emit
  timestamps as RFC 3339 UTC; never use bare `float64` for IDs or money
  values (precision loss). Agents compare outputs across runs —
  non-determinism silently breaks their flows.

## Accepted Architecture

- Agent mode precedence is `--format` flag, then `AGENT_MODE`, then TTY
  detection. Carry the resolved mode in `context.Context`.
- Cobra is the CLI framework. `ax.Execute()` wraps Cobra execution to add mode
  resolution and OpenTelemetry flush-on-exit behavior.
- Contexts must propagate W3C Trace Context IDs by default through the
  OpenTelemetry SDK.
- Grafana Loki is the log aggregation target; Tempo, Jaeger, and
  Honeycomb-compatible systems are valid trace backends through OTel.
  Default log shipping is decoupled (`stderr` → Promtail/Alloy DaemonSet);
  direct push is opt-in via `AX_LOKI_URL` (Constitution Principle VIII;
  see `specs/007-loki-direct-push/research.md`). Enforce label
  cardinality discipline at the logger API level: `environment`,
  `application`, `level`, `host`, and `version` are labels
  (low-cardinality, indexed); `trace_id`, `span_id`, `user_id`, durations,
  and resource IDs are payload (never promoted to labels).
- Use OpenTelemetry trace/span IDs for observability. Use **UUID v7** (via
  `github.com/google/uuid`) for resource and entity IDs, and **UUID v4**
  (same library) for auto-generated idempotency keys. Never mix observability
  IDs with resource/entity IDs.
- Use `github.com/rs/zerolog` for structured logging. The canonical
  constructor is `ax.NewLogger(ctx)`, returning an `ax.Logger` (initially
  backed by `*zerolog.Logger`) with trace correlation wired in.
  `logging.NewLogger(ctx)` is an identity-preserving alias of that same
  constructor exposed by the import-isolated surface — one implementation, one
  backend, one trace-correlation hook, reachable by two names (constitution
  `1.2.1`). It is not a second logger, and adding one remains forbidden by
  Principle VI.
- The `ax.Logger` interface exists ONLY as a migration seam for a future
  superseding decision. Do not introduce parallel-pluggable logger backends, an
  `ax.WithLogger(...)`-style runtime selection API, or a second concrete
  logger implementation (Constitution Principle VI).
- Stability and deprecation are governed by Constitution Principle XI
  (**Stability & SemVer**) and Principle XII (**Deprecation Lifecycle**).
  Pre-v1.0 (`0.x`): a `0.x.PATCH` release is bug-fixes-only and always safe to
  take; a `0.MINOR.0` bump MAY break (Go API surface OR machine-payload shapes
  like `ax.Error` / `__schema`, which are additive-tolerant); breaking changes
  ride the minor digit and never auto-promote to `1.0.0`. Deprecate an exported
  symbol with a `//Deprecated:` doc-comment paragraph carrying a migration note,
  let it ship in ≥1 published `0.MINOR.0` release (`staticcheck SA1019` flags
  call sites — already enabled), then remove it. See the constitution principles
  for the full policy.

## Development Workflow

1. Start by checking `git status --short` and reading the constitution
   (`.specify/memory/constitution.md`) plus any governing (frozen) ADRs.
2. Keep changes scoped to the requested behavior and the constitution-defined
   contract.
3. Prefer idiomatic, small Go packages. Avoid speculative abstractions.
4. Keep `README.md` and `examples/integration/` current with public API,
   command behavior, flags, output shapes, and roadmap changes. If a change
   affects how users should run or understand ax-go, update the docs and the
   integration example in the same change.
5. Run `gofmt` on Go changes.
6. Run `go test -race ./...` before handing work back. The race detector
   is REQUIRED, not optional — concurrent code is ubiquitous (OTel
   exporters, Loki push, ZeroLog hooks, idempotency).
7. Run `go vet ./...`, `golangci-lint run`, and `make doc-coverage`
   (ExampleXxx coverage on the primary API). All must be clean.
8. Use `testing.B` with `-benchmem` for allocation or hot-path performance
   claims. Do not assert numeric performance targets without a benchmark.
9. Run `make surface-check` (`internal/cmd/surfacecheck`) — the exported-surface
   gate across all build configurations and platforms. See **Build
   Configurations** below.

### Build Configurations (tagged toolchain)

Two negative build constraints, `ax_no_otlp` and `ax_no_grpc`, let a consumer
decline OTLP export and `ax.GRPCDial`. Both default off, so the default build is
what every existing consumer already has.

**Code behind a build constraint is invisible to the default toolchain.** Neither
`go vet ./...` nor `go test -race ./...` passes tags, and `golangci-lint` accepts
only one tag set per run. A green default run does NOT cover the declined
configurations — pass tags explicitly:

```bash
go build   -tags=ax_no_grpc,ax_no_otlp ./...
go test    -race -tags=ax_no_grpc,ax_no_otlp ./...
go vet     -tags=ax_no_grpc,ax_no_otlp ./...
golangci-lint run --build-tags=ax_no_grpc,ax_no_otlp
```

`make test`, `make validate`, and `make lint` iterate all four combinations
(`BUILD_TAG_MATRIX`); CI runs them as matrix jobs. The four configurations are
exhaustive: default, `ax_no_grpc`, `ax_no_otlp`, and both.

Rules when touching gated code:

- Tag names are stable public contract. Renaming one is a breaking change under
  Principle XI.
- Parity assertions (trace extraction, root span, log correlation, machine
  payloads) belong in **untagged** files so they run under every configuration.
  A parity claim verified only by default proves nothing.
- The declined build must never gain an exported symbol. `grpc_disabled.go` and
  `otlp_disabled.go` declare none, and must not reference any type from the
  declined dependency trees — doing so would pull the trees back in.
- `ax.GRPCDial` is the only public identifier whose presence may vary.

### Exported Surface Gate

`internal/cmd/surfacecheck` is the single surface gate. It is documented in
full under [Public Surface Gate](#public-surface-gate); the short version is
that it scans the seven public packages across 4 build-tag configurations ×
6 `GOOS`/`GOARCH` profiles = 24 loads, and diffs the result against both
`internal/cmd/surfacecheck/baseline.json` and the permanent audit. The load
count is 24 regardless of package count: a load is one (configuration, profile)
combination, and every requested package is loaded within it.

```bash
make surface-check    # verify
make surface-update   # regenerate after an INTENTIONAL API change
git diff internal/cmd/surfacecheck/baseline.json   # review every line
```

The tag combinations, profile list, and public package list are hardcoded Go
constants in `inventory.go`, so a matrix change is a reviewable commit
auditable via `git blame`. The public package list is duplicated from
`apidiff-verdict`'s `allowedPackages()` and guarded by a test that parses the
original and compares.

### Changelog & Releases

- **Never hand-edit or create `CHANGELOG.md`.** It is strictly managed by
  release-please from Conventional Commit history. Manual edits will be
  overwritten and create release-note conflicts. This overrides any general
  "always update the changelog" guidance from global agent instructions.
- Capture user-facing changes in the commit message (Conventional Commits:
  `feat:`, `fix:`, `feat!:` / `BREAKING CHANGE:`), not in the changelog file.
- The release flow (versioning, tags, `CHANGELOG.md`) is owned by
  release-please via `release-please-config.json` and
  `.release-please-manifest.json`. Do not duplicate it manually.
- When ax-go's minimum Go version changes or a new minor/major is released,
  update the compatibility matrix in `README.md` → `## Compatibility`. See
  [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full update process.

## Testing-First Discipline

**Tests land before implementation.** For every new behavior or
exported function, write the test that asserts the contract first and
verify it fails for the right reason. The implementation then makes it
pass without weakening the assertion. Bug fixes start with a failing
regression test.

Test forms and when to use each:

- **Table-driven tests** are the default for any function with more
  than one input shape. Standard form: `[]struct{}` cases, a `for`
  loop, `t.Run(tc.name, ...)`. Skip table-driven only when a single
  happy path is genuinely all that exists.
- **`ExampleXxx` functions** are how agents learn the API — they
  compile, run, and appear in godoc, the highest-leverage doc artifact
  Go offers. Coverage is two-tier:
  - *Required and gated:* a verified `ExampleXxx` on the **primary API
    surface** (constructors, the core exported types, and top-level
    entry points). `make doc-coverage` (`internal/cmd/doccover`)
    enforces this in CI, ratcheted through `baseline.txt` so it can
    never silently regress.
  - *Encouraged, not gated:* examples for other exported symbols where
    they add clarity. `WithX` functional options are demonstrated
    **inside** a parent example, not gated individually.
  Doc-comment *presence* is a separate, stricter rule and stays at
  100%: every exported symbol MUST carry a doc comment, enforced by
  `golangci-lint` (`godoclint`'s `require-doc`).
- **Golden-file tests** for `__schema` output, the `ax.Error` envelope
  JSON shape, and any output that is stable-by-contract. Schema
  stability is part of the public API.
- **Fuzz tests** (`func FuzzXxx(f *testing.F)`) for every parser
  surface: Hujson input, idempotency-key validation, error-envelope
  round-trip, `TRACEPARENT` extraction.
- **`testing.B` benchmarks with `-benchmem`** for any allocation or
  hot-path performance claim. Do not assert numeric targets without a
  benchmark.

Test surfaces every change must exercise (where applicable):

- stdout/stderr separation — nothing leaks to `stdout` that isn't the
  documented payload
- exit-code mapping
- `ax.Error` envelope shape (golden file)
- `__schema` output stability (golden file)
- agent-mode precedence (flag > env > TTY-detect)
- Hujson input parsing AND strict JSON output (round-trip + the
  read-Hujson/write-JSON asymmetry)
- `--dry-run` side-effect suppression
- idempotency-key generation, propagation, and envelope surfacing
- OTel trace correlation in logs (`trace_id` and `span_id` on every
  line when a span is active)
- Output determinism (same input → byte-identical envelope modulo
  documented non-deterministic fields)

## Coverage Policy

Test coverage is gated in CI by `internal/cmd/covercheck`, which enforces both
per-package and repo-wide floors against the `coverage.out` profile. The gate is
authoritative; Codecov status checks are advisory. Floors are hardcoded Go
constants in `internal/cmd/covercheck/main.go`, so every floor change is a
reviewable commit auditable via `git blame`.

### Floors

| Scope | Floor | Aspirational Target |
|-------|-------|---------------------|
| Repo-wide (aggregate) | 78% | 85% |
| Per-package default | 25% | 80% |

Per-package overrides (the original six calibrated to the 2026-06-16 baseline;
`internal/cli`, `internal/mcp`, and `internal/schema` calibrated on 2026-07-17
when they enrolled, ~2pp below their measured coverage):

| Package | Floor |
|---------|-------|
| `github.com/rshade/ax-go` | 85% |
| `github.com/rshade/ax-go/examples/integration` | 85% |
| `github.com/rshade/ax-go/examples/logging` | 98% |
| `github.com/rshade/ax-go/examples/rootlogging` | 98% |
| `github.com/rshade/ax-go/internal/cmd/sizecheck` | 86% |
| `github.com/rshade/ax-go/internal/logcore` | 96.5% |
| `github.com/rshade/ax-go/logging` | 98% |
| `github.com/rshade/ax-go/internal/cli` | 98% |
| `github.com/rshade/ax-go/internal/cmd/benchcheck` | 80% |
| `github.com/rshade/ax-go/internal/cmd/doccover` | 45% |
| `github.com/rshade/ax-go/internal/cmd/surfacecheck` | 80% |
| `github.com/rshade/ax-go/internal/config` | 65% |
| `github.com/rshade/ax-go/internal/mcp` | 96.9% |
| `github.com/rshade/ax-go/internal/schema` | 93% |
| `github.com/rshade/ax-go/internal/telemetry` | 60% |
| `github.com/rshade/ax-go/internal/testutil` | 25% |

Any package without an explicit override (including newly added packages and
`internal/cmd/covercheck` itself) faces the 25% per-package default.

### Excluded from Per-Package Floor Enforcement

The exclusion set is empty: every package in the module faces the per-package
floor gate. The three originally excluded packages (`internal/cli`,
`internal/mcp`, and `internal/schema`) were enrolled with tests and explicit
floors on 2026-07-17; retiring their 0% contribution to the aggregate is what
allowed the repo-wide floor to rise from 70% to 78%.

### Local Verification

Run the exact same check CI runs:

```bash
make cover-check
```

Or step by step:

```bash
go test -race -coverprofile=coverage.out -covermode=atomic ./...
go run ./internal/cmd/covercheck -coverage coverage.out
```

`covercheck` exits `0` when all floors are met, `1` on a floor violation (naming
each offending package with its actual%, floor%, and shortfall on stderr), and
`2` on bad input (missing or malformed coverage file).

### Raising a Floor

1. Improve coverage in the target package.
2. Edit the `perPackage` map in the `defaultFloorConfig()` function in `internal/cmd/covercheck/main.go`.
3. Verify locally with `make cover-check`.
4. The commit message records why the floor was raised (floor changes are
   auditable via `git blame`).

### Escalation Path

Floors escalate in 5pp increments as tests are added. All packages are now
under per-package enforcement, so the remaining path to the 80% per-package
and 85% repo-wide aspirational targets is raising individual package floors —
and the repo-wide floor with them — as coverage improves.

## Performance Regression Budget

Performance is gated in CI by `internal/cmd/benchcheck`, which compares a
fresh current-worktree `-count=10` benchmark run against a same-host
benchmark run from the PR's base commit using
`golang.org/x/perf/benchstat`'s statistical comparison. The gate is
authoritative. Both benchmark runs use `-cpu=1` so benchmark names do not
depend on host core count (`BenchmarkFoo-1` on every machine), and no
committed benchmark output bakes in one developer laptop or one GitHub
runner generation. Budget thresholds are hardcoded Go constants in
`internal/cmd/benchcheck/main.go`, so every threshold change is a reviewable
commit auditable via `git blame`.

### Tracked Benchmarks

| Benchmark | Hot path |
|-----------|----------|
| `BenchmarkLoggerEmit/*`, `BenchmarkLoggerTracingHook/*`, `BenchmarkLoggerFieldShapes/*`, `BenchmarkLogger/*`, `BenchmarkLoggerDisabledLevel` | Logger emit path |
| `BenchmarkParseConfigBoundedRead/*`, `BenchmarkParseConfigDefaultCapRead` | Hujson/config parse path |
| `BenchmarkBuildCommand` | `__schema` reflection path |
| `BenchmarkWriteError` | Error envelope marshal path |

A benchmark added on the current branch but absent from `BENCH_BASE_REF` is
absent from the comparison until it lands on the base branch — this is not a
failure condition.

Conversely, a benchmark present in `BENCH_BASE_REF` that is **absent from
the current run** (its package failed to build, it panicked, or it was
renamed/deleted) is always a hard failure — `benchcheck` detects this case
explicitly rather than letting it silently vanish from the comparison, since
`benchstat` itself only compares rows present on both sides and would
otherwise drop a missing benchmark without any signal.

### Budget

| Metric | Budget | Notes |
|--------|--------|-------|
| `ns/op` | 5% increase | Counted only when `benchstat` marks the delta statistically significant (α=0.05) |
| `allocs/op` | +1 increase | Absolute, not percentage — most tracked benchmarks report 0 allocs/op, where a percentage threshold is undefined |

A benchmark failing either budget independently fails the check; a
benchmark can fail on time, allocations, or both, and the failure message
identifies each independently.

### Local Verification (Benchmarks)

Run the exact same check CI runs:

```bash
make bench-check
```

`make bench-check` creates a temporary git worktree for `BENCH_BASE_REF`,
runs `go test -run='^$' -bench=. -cpu=1 -count=10 -benchmem ./...` there,
runs the same command in the current worktree, and compares the two files.
CI sets `BENCH_BASE_REF` to the pull request base SHA. Locally, it defaults
to `git merge-base HEAD origin/main`, then `HEAD~1` if `origin/main` is not
available; override it when needed:

```bash
BENCH_BASE_REF=HEAD~1 make bench-check
```

Or step by step:

```bash
BENCH_BASE_REF="${BENCH_BASE_REF:-$(git merge-base HEAD origin/main 2>/dev/null || git rev-parse --verify --quiet HEAD~1)}"
git worktree add --detach /tmp/ax-go-bench-base "$BENCH_BASE_REF"
(cd /tmp/ax-go-bench-base && go test -run='^$' -bench=. -cpu=1 -count=10 -benchmem ./... > /tmp/bench-base.txt)
go test -run='^$' -bench=. -cpu=1 -count=10 -benchmem ./... > /tmp/bench-current.txt
go run ./internal/cmd/benchcheck -baseline /tmp/bench-base.txt -current /tmp/bench-current.txt
git worktree remove --force /tmp/ax-go-bench-base
```

`benchcheck` exits `0` when every tracked benchmark is within budget, `1` on
a budget violation (naming each offending benchmark, the exceeded metric,
and the measured delta on stderr), and `2` on bad input (missing or
malformed baseline/current file).

### Adjusting the Budget

1. Edit `maxNsOpRegressionPercent` and/or `maxAllocsOpIncrease` in
   `internal/cmd/benchcheck/main.go`.
2. Verify locally with `make bench-check`.
3. The commit message records why the budget changed (budget changes are
   auditable via `git blame`).

### Accepting Trade-Offs

There is no committed benchmark baseline to update. A reviewed,
intentional performance trade-off is handled as a policy decision: either
keep the code within the existing budget or change the budget constants in
`internal/cmd/benchcheck/main.go` with the rationale in the commit message.
After the trade-off lands on the base branch, future PRs compare against
that branch state on their own runner.

## Public Surface Gate

The public API surface is gated in CI by `internal/cmd/surfacecheck`, which
inventories the complete compiler-visible surface (declarations, direct and
promoted fields, complete interface method sets, value and pointer method
sets, alias-attributed members, and reachable hidden concrete types) of all
seven public packages, across 4 build-tag configurations × 6 supported
GOOS/GOARCH profiles = 24 loads, and compares it against two committed
artifacts:

- `internal/cmd/surfacecheck/baseline.json`: the current approved canonical
  feature IDs, signatures, and the configurations and profiles each feature is
  present in (schema version 2; the version 1 root-only shape is described in
  `specs/015-internalize-helpers/contracts/baseline-schema.md`).
- `specs/015-internalize-helpers/public-surface-audit.json`: the permanent,
  never-delete decision record classifying every feature as `supported` or
  `implementation-leak` with a lifecycle state (schema in
  `specs/015-internalize-helpers/contracts/audit-schema.md`). The audit is
  scoped to the **root package**, whose bare feature IDs it joins on; the
  other five public packages are gated by the baseline alone.

Run it from the module root:

```bash
make surface-check                          # verify
make surface-update                         # regenerate the baseline
go run ./internal/cmd/surfacecheck
git diff internal/cmd/surfacecheck/baseline.json   # review every line
```

From a nested directory:

```bash
make -C "$(git rev-parse --show-toplevel)" surface-check
```

### Stream and Exit Contract

A pass writes one minified JSON object to stdout and nothing to stderr and
exits `0`. Every failure writes nothing to stdout and exactly one minified
`ax.Error` envelope to stderr:

| Error code | Exit | Meaning |
|------------|------|---------|
| `surface_drift` | `2` | Source, configurations, profiles, baseline, audit state, or a required `Deprecated:` paragraph disagree. |
| `invalid_surface_artifact` | `2` | Missing, malformed, oversized, unsorted, duplicate, or schema-invalid baseline/audit; invalid flags. |
| `surface_permission` | `4` | Permission denial reading artifacts or executing tooling. |
| `surface_internal` | `1` | Unexpected internal failure. |

### Presence and Divergence

Presence is a **recorded fact, not an invariant**: a build constraint removing
an identifier is the whole point of the configuration axis, and a
platform-specific declaration is legitimate too. Each baseline entry therefore
carries a `configurations` and a `profiles` presence set, written as the `"all"`
sentinel or an explicit sorted list. An unreviewed change in *where* a feature
exists is `presence-changed` drift, so the gate still fails closed.

Signature, by contrast, stays a hard invariant: one feature has exactly one
signature. A feature rendering differently between two combinations is
`signature-divergent` and fails closed with no inventory, because there is no
canonical signature to record.

Presence is stored as the product of the two sets. `-update` verifies that
factorisation is exact and fails with `presence-unfactored` rather than
recording a pattern lossily — a lossy record would let the gate later accept a
surface nobody reviewed.

`ax.GRPCDial` is currently the only feature whose presence is not `"all"`.

### Change Protocol

- An intentional live-surface change updates `baseline.json` in the same
  reviewed PR; an intentional addition also appends a retained audit record
  (see `specs/015-internalize-helpers/quickstart.md`). Bootstrap candidates
  come from `go run ./internal/cmd/surfacecheck -list` and `-audit-seed`;
  both modes are read-only and the seed is invalid until manually classified.
- Deprecations retain the baseline entry and transition the audit row
  `live → deprecated`; removal is a follow-up breaking feature that
  transitions `deprecated → removable → removed` only after a published
  `0.MINOR.0` carried the notice (Constitution Principle XII).
- Never hand-tune the gate into silence: an `added` drift means the surface
  change is unreviewed, not that the baseline should be regenerated blindly.

`internal/cmd/surfacecheck` is an internal command package with an explicit
80% per-package coverage floor in
`internal/cmd/covercheck/main.go`.

## Binary Size Gate

The import-isolated `logging` surface exists to make a logging-only consumer
small, and a guarantee nothing measures is a guarantee that decays. Binary size
is gated in CI by `internal/cmd/sizecheck`, which builds two committed probe
programs with production flags (`-trimpath -ldflags="-s -w"`) and enforces two
independent budgets. Thresholds are hardcoded Go constants in
`internal/cmd/sizecheck/main.go`, so every change is a reviewable commit
auditable via `git blame`.

### The two probes

| Program | Role |
|---------|------|
| `examples/logging` | imports **only** `github.com/rshade/ax-go/logging`; the measured subject |
| `examples/rootlogging` | byte-for-byte the same program on root `ax`; the ratio denominator |

They differ by one import and one call. Keep them diff-clean against each
other — anything more makes the ratio measure something other than the import
boundary. They are committed rather than synthesised at measurement time so the
comparison builds against this repository's own `go.mod`: no network, no
`replace` stanza, and the difference stays reviewable in `git diff`.

### The two budgets, and why they are NOT adjusted the same way

| Constant | Value | Adjustable? |
|----------|-------|-------------|
| `maxIsolatedBinaryBytes` | 3,000,000 | **Yes**, for a reviewed reason |
| `minReductionPercent` | 75% | **No** — lowering it is a spec change |

The **absolute ceiling** drifts with the toolchain, so it carries deliberate
headroom (measured 2,261,257 bytes on Go 1.26.5) and may be raised on the
coverage/benchmark protocol: confirm the increase is understood — a jump almost
always means a new transitive dependency, so check `go list -deps` first — edit
the constant, verify, and record the reason in the commit message.

The **reduction ratio** is toolchain-independent: both probes are built by the
same compiler in the same run, so a newer Go moves them together and can never
explain a ratio breach. A breach means the isolated surface gained weight the
root facade did not, which is the exact regression this feature exists to
prevent. Lowering it re-negotiates the headline claim — treat it as a spec
change and update SC-002 in the same commit.

Never move either constant to silence a failure whose cause you have not
identified.

### Stream and exit contract

A pass writes one minified JSON object to stdout — including both measured sizes
and the computed reduction — and nothing to stderr, exiting `0`. Every failure
writes nothing to stdout and exactly one minified `ax.Error` envelope to stderr.
Each failure carries its own code, because a maintainer resolves each
differently — a probe that did not compile, a breached ceiling, and a breached
ratio are never collapsed into one:

| Error code | Exit | Meaning |
|------------|------|---------|
| `size_build_failed` | `2` | A probe did not compile; the budget was NOT checked. |
| `size_ceiling_exceeded` | `2` | The isolated binary breached the absolute ceiling. |
| `size_reduction_insufficient` | `2` | The isolated binary is not sufficiently smaller than the root build. |
| `invalid_size_artifact` | `2` | Invalid flags or unexpected positional arguments. |
| `size_permission` | `4` | Permission denied executing the Go toolchain. |
| `size_internal` | `1` | Unexpected internal failure. |

### Local verification (size)

```bash
make size-check
```

## Type Relocation Is Not a Breaking Change

`go-apidiff` keys a type's identity on its **declaring package**, so moving an
exported type into another package and leaving an identity-preserving alias
behind (`type Error = contract.Error`) is reported as an incompatible change —
even though every consumer compiles unchanged.

This repository has already shipped that refactor once. The v0.1.0 → v0.2.0
release moved `Error`, `Mode`, `Envelope`, `Schema`, and the config/schema option
types into the import-isolated public packages, released as a plain `feat:`, and
was a no-op for adopters. Running `go-apidiff` across that tag boundary today
reports **37 findings of this shape** — including entries whose "before" and
"after" renderings are textually identical. The apidiff gate landed afterwards
(PR #82) and had never been reconciled against that precedent.

`internal/cmd/apidiff-verdict` therefore classifies these findings and excludes
them from the merge gate. The rule is deliberately narrow — a finding is excused
only when:

- the two renderings are **textually identical**, or
- a bare type name gained a declaring package **inside this module** while
  keeping its name, allowing the established prefix convention
  (`ParseConfigOption` → `config.Option`, `LoggerOption` → `logcore.Option`).

Removals, renames, signature changes, member-level changes, and relocations to
another module all stay breaking. Excused findings are printed in their own
**"Type relocations (not gated)"** report section — never silently dropped — and
structural changes to the relocated types remain gated by `surface-check`, which
inventories every field and interface method across all 24 loads. The two gates
are complementary: apidiff for semantic compatibility, surfacecheck for exact
structural surface.

The classifier's acceptance test is the release history: it must rule the shipped
v0.1.0 → v0.2.0 diff non-breaking, and `TestClassifierMatchesShippedReleaseHistory`
asserts exactly that.

## Documentation Discipline (Contracts, Not Narration)

Coding agents read doc comments as much as humans do, and an agent
working in the package learns the contract from them. Write doc comments
as contracts, not narration.

- **Document the contract, not the code.** State inputs, outputs, the
  error a function returns and the **exit code** it maps to, invariants,
  units, and fail-closed semantics. Mirror the stable `error_code` style
  the specs use (e.g. FR-007 / SC-005 of the bounded-config feature).
- **Never write "what" comments that restate the code.** A comment that
  narrates the line above it (`// increment i`) is noise on a good day
  and a lie after the next refactor. Comment the **why**: rationale,
  constraints, the non-obvious.
- **Prefer verified docs over prose; treat drift as a defect.** An
  `ExampleXxx` compiles and (with `// Output:`) is executed by
  `go test`, so it cannot silently drift; a golden file breaks on
  change; a type pins a shape. When a fact can be pinned by an example,
  a golden test, or a type, prefer that over a sentence.
- **Presence is gated; quality is on you.** `godoclint`'s `require-doc`
  gates that every exported symbol HAS a doc comment. It cannot tell a
  contract from narration — that is the reviewer's job.

## Go Discipline

Conventions LLM agents frequently slip on. Every implementation
follows them.

- **`context.Context` first parameter** on every function that does
  I/O, makes outbound calls, runs goroutines, or could be canceled.
  No "convenience" overloads without context.
- **Wrap errors with `%w`**, never `%s` or `%v`.
  `fmt.Errorf("loading config: %w", err)` preserves the chain;
  `fmt.Errorf("%s", err)` destroys it. `errors.Is` and `errors.As`
  MUST work against every error returned from this library.
- **No `panic` in library code.** Return errors. `panic` is reserved
  for programmer-error invariants that can never legitimately occur —
  never for user input, network failures, or missing resources.
- **Avoid `any` / `interface{}`.** Prefer a concrete type or a
  tightly scoped interface. When `any` is genuinely needed (decoding
  a flexible shape, for example), comment why at the use site.
- **No mutable package-level state.** No global maps, slices, or
  counters. No `init()` that mutates globals, reads files, makes
  network calls, or reads the environment for runtime behavior.
  Configuration enters via constructors.
- **Minimal dependencies.** Every new dep is maintenance cost and a
  future migration target. Before adding one: confirm the stdlib
  doesn't cover it, the existing dep set doesn't, and the added
  function is non-trivial to write inline. Justify the addition in
  the PR description.
- **`defer Close()` for every `io.Closer`.** Never let one leak. If
  `Close` returns an error that matters, log it via the canonical
  logger; never silently discard.
- **Functional options** for constructors with more than 2-3 config
  knobs: `ax.NewLogger(ctx, ax.WithLevel(...), ax.WithSink(...))`.
  Avoids "config struct vs. positional arg" churn as the API grows.
- **`internal/` packages** for anything consumers shouldn't import.
  The Go toolchain enforces it; use it freely.
- **Version injection at build.** Binaries built from ax-go-based
  CLIs inject a real version into `__schema`'s `version` field via
  `-ldflags "-X ..."`. Never ship `dev` or `unknown` to production
  agents.
- **Public API diffing in CI.** The `API Diff` workflow
  (`.github/workflows/apidiff.yml`) runs `go-apidiff` on every PR and scopes
  the result to the public surface — the root package `ax` plus the public
  packages `config`, `contract`, `id`, `logging`, `mcp`, and `schema`.
  `internal/` is exempt
  (Constitution Principle XI; the toolchain blocks external import). An
  incompatible change to that surface **fails CI** unless the PR carries the
  `breaking-change-approved` label. Applying the label acknowledges the break
  is intentional and MUST be paired with a `feat!:` / `BREAKING CHANGE:`
  commit so release-please rides the break on the minor digit (pre-v1.0,
  `0.MINOR.0` MAY break). The public-package allowlist is the single source of
  truth in `internal/cmd/apidiff-verdict`; adding a public package requires
  updating it (a `check-packages` guard fails CI otherwise). The breaking-change
  definition itself lives in Constitution Principle XI, which supersedes the
  never-written ADR-0013 that originally gated this work.

## Guardrails For Agents

- Never write logs, warnings, progress, or prompts to `stdout`.
- Never relax machine-readable output guarantees for human convenience.
- Never use trace IDs as resource IDs or resource IDs as trace IDs.
- **Never log PII, secrets, tokens, or credentials** — not in Loki labels,
  not in JSON payload. The cardinality split (Constitution Principle VIII)
  governs index
  performance; the no-PII rule is privacy/security and applies to all log
  output regardless.
- **Never compose log messages from un-sanitized user-controlled strings.**
  Control characters (newlines, ANSI escapes) in user input can forge log
  entries. Use ZeroLog's field methods (`.Str`, `.Int`, etc.) which escape
  correctly; never `.Msg(fmt.Sprintf(...userInput...))`.
- **Never skip TLS verification** on outbound HTTP/gRPC calls. The `ax`
  helpers (`ax.HTTPClient`, `ax.GRPCDial`) ship secure defaults; do not
  pass `InsecureSkipVerify: true` or equivalents.
- **Never read unbounded user input.** Hujson config files MUST be
  size-capped at the read boundary (default 1 MiB, configurable). An
  oversized config is a validation error, not an OOM.
- Public API or runtime-behavior changes are specified through the Spec Kit
  feature workflow (GitHub issue → spec → plan → tasks), NOT new ADRs — ADRs are
  frozen. Record the decision in the feature's `research.md` and, when
  cross-cutting, amend the constitution. Even a renamed exported identifier is a
  breaking change.
- Keep `GEMINI.md` and `CLAUDE.md` as thin imports of this file so all
  agent guidance stays synchronized.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
at specs/015-internalize-helpers/plan.md
<!-- SPECKIT END -->
