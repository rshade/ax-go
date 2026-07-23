# ax-go

![The ax-go mascot: a teal-cyan robot gopher shouldering an axe](docs/brand/ax-go-logo-256.png)

> Agentic Experience (AX) foundation for Go CLI tools — the "Common DNA" for
> the rshade portfolio.

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-live-blue)](https://rshade.github.io/ax-go/)
[![Coverage](https://codecov.io/gh/rshade/ax-go/branch/main/graph/badge.svg)](https://codecov.io/gh/rshade/ax-go)
[![Cross-Compile](https://github.com/rshade/ax-go/actions/workflows/crosscompile.yml/badge.svg)](https://github.com/rshade/ax-go/actions/workflows/crosscompile.yml)

📖 **Documentation:** <https://rshade.github.io/ax-go/>

> **Status: Released (pre-v1.0, `0.x`).** The current pinnable release is
> **v0.3.0**: `go get github.com/rshade/ax-go@v0.3.0`. Output contracts are
> frozen in code — core primitives such as `ax.Error`, `ax.Execute`,
> `ax.NewLogger`, `ax.ParseConfig`, and `ax.NewEntityID` are covered by
> contract tests, and all public output shapes are pinned by golden fixtures.
> v0.2.0 added the import-isolated contract packages (`contract`, `config`,
> `schema`, `id`) for thin consumers; v0.3.0 adds the public `mcp` package
> (`mcp.Serve`, `mcp.NewCommand`), the `ax.Guard` / `ax.Perform` dry-run
> side-effect guards, and the `retryable` / `retry_after_seconds` error
> recovery fields. Releases are tagged by release-please from the Conventional
> Commit history; see [CHANGELOG.md](CHANGELOG.md) for release history.
>
> **Stability guarantee while in `0.x`:** a **patch** upgrade (`0.x.PATCH`) is
> always safe — patch releases are bug-fixes-only and stay backward-compatible.
> A **minor** bump (`0.MINOR.0`) MAY contain breaking changes (to the Go API or
> to machine-payload shapes like `ax.Error` / `__schema`), and breaking changes
> never auto-promote to `1.0.0`. The full policy — including what counts as a
> breaking change and the deprecation lifecycle — lives in the constitution's
> **Stability & SemVer** and **Deprecation Lifecycle** principles
> ([`.specify/memory/constitution.md`](.specify/memory/constitution.md)).

**Security:** please report suspected vulnerabilities privately through
[GitHub Security Advisories](https://github.com/rshade/ax-go/security/advisories/new).
See [`SECURITY.md`](SECURITY.md) for the full policy. Do not open public
issues for unpatched vulnerabilities.

## Mission

ax-go is the shared foundation that standardizes **Agentic Experience (AX)**
across Go-based CLI tools. Its goal is simple:

> Ensure all Go-based CLI tools are as powerful and predictable for LLM agents
> as they are for human engineers.

Rather than every CLI reinventing how it talks to an autonomous agent — how it
emits data, reports errors, exposes its command tree, and stays safe under
retries — ax-go encodes those conventions once so the whole portfolio shares
the same predictable behavior.

## Public Import Surfaces

Use the root package for full CLI runtime behavior:

```go
import ax "github.com/rshade/ax-go"
```

The root `ax` facade remains the ergonomic surface for complete Cobra CLIs:
`ax.Execute`, telemetry lifecycle, structured logging, `__schema` command
wiring, HTTP/gRPC helpers, and trace-aware envelopes.

Use isolated contract packages for thin consumers that only need stable machine
contracts without root runtime adapters:

```go
import (
    "github.com/rshade/ax-go/config"
    "github.com/rshade/ax-go/contract"
    "github.com/rshade/ax-go/id"
    "github.com/rshade/ax-go/schema"
)
```

- `contract`: exit codes, mode resolution, context metadata, success/error
  envelopes, and strict JSON/NDJSON writers.
- `config`: bounded Hujson reads and comment-preserving RFC 6902 patches.
- `schema`: ax-native and MCP-compatible command schema shapes/builders.
- `id`: UUID v4 idempotency keys and UUID v7 entity/resource IDs.

Use the isolated logging package when you want structured, trace-correlated
logging without the runtime:

```go
import "github.com/rshade/ax-go/logging"
```

- `logging`: the zerolog-backed logger, stream separation, and
  `trace_id`/`span_id` on every line — with no OTel SDK, no OTLP exporter, no
  gRPC, no Cobra, and no `net/http`.

Import-isolation tests keep those public contract packages free of the root
facade, telemetry exporters/SDK setup, logger/Loki, HTTP instrumentation, and
gRPC runtime adapters.

### Choosing a surface

| You need | Import |
|---|---|
| Logging only, smallest binary | `logging` |
| Stable machine contracts only | `contract`, `config`, `schema`, `id` |
| Logging **plus** Loki direct push | root `ax` |
| Logging plus OTel export or `ax.Execute` | root `ax` |

The size difference is the point. A logging-only consumer links 103 packages;
the same program on the root facade links 410:

| Program | Stripped binary |
|---|---|
| imports `logging` | **2,261,257 bytes** |
| imports root `ax` | 12,013,833 bytes |
| reduction | **81.2%** |

Measured on linux/amd64 with Go 1.26.5 and `-trimpath -ldflags="-s -w"`, and
enforced on every PR by `make size-check` against the two committed probe
programs `examples/logging` and `examples/rootlogging`. Absolute sizes drift
with the toolchain; the ratio does not, because both probes move together.

`logging` differs from the four contract packages in what it is allowed to
link: `zerolog` and the OpenTelemetry trace **API** are required there (the
logger's method set names them, and they are what makes trace correlation
work), while `net/http` and `crypto/tls` are forbidden. `net/http` is the
single largest size lever, which is why Loki direct push — which needs it —
stays available only through root `ax`.

Both surfaces name **one** logger. `ax.Logger`, `ax.Labels`, and
`ax.LoggerOption` are identity-preserving aliases of the same declarations
`logging` exposes, so a logger from either is accepted by the other with no
conversion, and an option manufactured by root `ax` — including
`ax.WithLokiFromEnv()` — is accepted by `logging.NewLogger`. There is one
implementation, one backend, and one trace-correlation hook behind both names.

**These packages provide no live tracing.** They link zero gRPC and always
have, and that same import isolation keeps the OpenTelemetry SDK out of them,
so none of them starts a span, extracts `TRACEPARENT` / `TRACESTATE`, or
resolves an active span context. `contract.TraceIDFromContext` and
`contract.SpanIDFromContext` **read back metadata a caller already stored**
with `contract.WithMetadata` — they resolve nothing themselves, and return
`contract.ZeroTraceID` / `contract.ZeroSpanID` when no metadata is present.
Because those zero values are well-formed W3C IDs rather than empty strings, a
context that was never populated produces valid-looking output carrying no
trace at all.

Live tracing is provided **only by the root `ax` package**: `ax.StartTelemetry`
(W3C propagation and `TRACEPARENT` extraction), the recording root span
`ax.Execute` opens around the command, and `ax.NewLogger`'s `trace_id` /
`span_id` log correlation. Thin consumers that need real trace IDs must import
root `ax` and accept the runtime weight the isolated packages exist to avoid —
or keep the root facade and drop the heavy optional pieces at build time, as
described in [Slimming the binary](#slimming-the-binary-optional-otlp-and-grpc).

Use the `mcp` package to run a CLI as a live MCP server (see
[Running as an MCP server](#running-as-an-mcp-server)):

```go
import "github.com/rshade/ax-go/mcp"
```

- `mcp`: a thin entry point (`mcp.Serve`, `mcp.NewCommand`) that exposes an
  ax-go command tree as a live MCP server over the official MCP Go SDK. The SDK
  and all protocol/transport/dispatch mechanics stay behind `internal/mcpserver`.

## Core Standards

These are the non-negotiable mandates every tool built on ax-go must follow.

### The Golden Rule — stream separation

- **`stdout`** is strictly reserved for the final data payload (JSON).
- **`stderr`** carries everything else: logs, progress indicators, and
  structured error envelopes.

This lets an agent pipe `stdout` straight into a JSON parser while humans and
log collectors read `stderr`.

### Deterministic exit codes

| Code | Meaning                       |
| ---- | ----------------------------- |
| `0`  | success                       |
| `1`  | unknown / internal error      |
| `2`  | validation / bad input        |
| `3`  | network / timeout             |
| `4`  | authentication / permission   |

### Determinism is machine-readable trust

Given the same inputs, two runs of the same command produce **byte-identical**
`stdout` (modulo documented non-deterministic fields — timestamps, `trace_id`,
auto-generated `idempotency_key`). This is the machine equivalent of trust:
agents diff outputs across runs to detect drift, so determinism is what lets an
agent safely delegate to an ax-go CLI. It is a stronger guarantee than the AX
literature demands, and it is the project's clearest differentiator.

### Machine discoverability (`__schema`)

Every tool implements a `__schema` command that emits a structured JSON map of
its command tree, flags, types, and examples — so agents can ground themselves
without guessing. The primary format is ax-native JSON, with
`__schema --as=mcp` available as an MCP-compatible adapter. The discoverability
contract is now owned by the `schema` package and the absorbed decisions in
[`specs/010-import-isolated-contracts/research.md`](specs/010-import-isolated-contracts/research.md).

Each command declares an explicit `non_deterministic_fields` list. Commands
that emit `ax.Envelope[T]` register their payload type once; varying payload
fields carry the marking at their definition:

```go
type result struct {
    EntityID string `json:"entity_id" ax:"nondeterministic"`
}

ax.WithNonDeterministicFields[result](cmd)
```

Registration adds the standard `meta.trace_id`, `meta.span_id`, and
`meta.idempotency_key` locators. Tagged payload fields appear as `data.*`
locators, and unregistered commands emit an explicit empty list. The same list
is available as `nonDeterministicFields` in `__schema --as=mcp`; it is the
authoritative mask for deterministic output comparisons.

### Running as an MCP server

The same command tree that powers `__schema --as=mcp` can run as a **live MCP
server** with no per-tool work, via the `mcp` package. Mount the reserved
`mcp-server` subcommand (an explicit opt-in — it executes commands, so it is not
auto-mounted like `__schema`):

```go
import (
    "github.com/rshade/ax-go"
    "github.com/rshade/ax-go/mcp"
)

func main() {
    root := newRootCommand()
    root.AddCommand(mcp.NewCommand(root, mcp.WithVersion(version)))
    os.Exit(ax.Execute(context.Background(), root))
}
```

Run it over **stdio** (the default; the idiomatic MCP subprocess model) or a
**streamable HTTP** transport:

```sh
mycli mcp-server                                            # stdio
mycli mcp-server --transport=http --addr=127.0.0.1:8080     # loopback HTTP
mycli mcp-server --transport=http --addr=0.0.0.0:8080 --allow-non-loopback
```

- **Discovery**: every non-hidden command becomes a tool (reusing
  `schema.BuildMCPSchema`); `__schema` and `mcp-server` are excluded.
- **Execution**: `tools/call` runs the command in machine/JSON mode and returns
  its verbatim `stdout` payload; a non-zero exit returns the `ax.Error` envelope
  with `IsError` set, and the server keeps serving.
- **Safety**: HTTP binds **loopback by default** and fails closed (exit 2) on a
  non-loopback bind without `--allow-non-loopback`; ax-go holds no credentials
  and runs no auth flow, so put authn/authz in front of an exposed endpoint.
- **Streams & tracing**: protocol I/O stays on the transport channel and logs
  on `stderr`; each call is a span that continues the caller's W3C trace.
- **Out of scope (MVP)**: incremental MCP streaming/progress notifications, an
  auth flow, and a standalone `cmd/` launcher (the runnable instance is any
  adopting CLI's mounted subcommand). A real build version is required
  (`WithVersion` or `-ldflags`); `dev`/`unknown` is rejected at startup.

### Asymmetric JSON flow

- **Input:** accept [Hujson](https://github.com/tailscale/hujson) (comments and
  trailing commas) for human convenience. Config reads are capped at 1 MiB by
  default at the read boundary; use `ax.WithMaxConfigBytes` when a CLI
  intentionally supports a larger bounded config. The public helpers are
  `ax.ParseConfig(ctx, reader, &cfg, ...)` and
  `ax.ParseConfigFile(ctx, path, &cfg, ...)`, so slow cooperative sources can be
  canceled through `context.Context`.
- **Output:** emit strict, minified JSON for bounded payloads; emit NDJSON for
  streaming / unbounded result sets.

Config read rejections are standard `ax.Error` envelopes. Oversized input uses
the frozen `error_code` `config_too_large`; an out-of-range cap (negative or
above `ax.MaxConfigBytesCeiling`, 1 GiB) uses `config_max_bytes_invalid`.
Invalid Hujson or schema mismatches use `config_invalid`, and a nil
`ParseConfigOption` uses `config_option_invalid`. All map to exit code `2` and
are discoverable with `errors.As(err, &axErr)`. Reads accept Hujson extensions,
but payload writes remain strict JSON.

To mutate an existing Hujson config without stripping user comments,
`ax.PatchConfig(ctx, reader, patch, ...)` and
`ax.PatchConfigFile(ctx, path, patch, ...)` apply RFC 6902 JSON Patch
operations to the Hujson AST. Comments survive; whitespace is normalized to
canonical Hujson formatting. `PatchConfigFile` writes back atomically (temp
file + rename) and preserves file permissions. An invalid patch document or a
failed patch operation uses the frozen `error_code` `config_patch_invalid`
(exit code `2`); an invalid existing config uses `config_invalid`.

### Agent-safety primitives

- **`--idempotency-key`** — auto-generated UUID v4 if absent and surfaced in the
  output envelope, killing duplicate-create from agent retries.
- **`--dry-run`** — universal middleware that emits the same envelope with
  `dry_run: true` and performs no side effects.
- **`ax.Guard` / `ax.Perform`** — helpers that guard a side-effecting operation
  on the dry-run flag, so commands stop hand-rolling
  `if ax.DryRunFromContext(ctx) { ... } else { ... }`. `Guard` runs an effect
  unless dry-run is active (and reports whether it ran); `Perform` runs the real
  `commit`, or a read-only `rehearse` preview under dry-run that surfaces the same
  validation errors without mutating. Each writes a single suppression line to
  `stderr` (never `stdout`) when it skips. The envelope's `dry_run: true` still
  flows automatically.

  ```go
  // Skip-only: writeReport runs for real, is suppressed under --dry-run.
  wrote, err := ax.Guard(ctx, func(ctx context.Context) error {
      return writeReport(ctx, path)
  })

  // Faithful preview: validate under --dry-run, mutate for real.
  err = ax.Perform(ctx,
      func(ctx context.Context) error { return validatePatch(ctx, path, doc) }, // dry-run
      func(ctx context.Context) error { return ax.PatchConfigFile(ctx, path, doc) }, // real
  )
  ```

- **`--format` flag / `AGENT_MODE` env var / TTY auto-detect** — selects machine
  vs. human output mode. The precedence is `--format` flag, then `AGENT_MODE`,
  then TTY detection.

### Standard `ax.Error` envelope

A structured, machine-readable error format emitted to `stderr`. Schema defined
by the root `ax.Error` facade and the isolated `contract.Error` type.

Required fields: `error_code`, `message`, `trace_id`, `tool`, `version`,
`schema_version`. Optional remediation fields let an agent self-correct instead
of just reporting:

- **`actionable_fix`** (string) — a best-effort human/machine hint for fixing the
  failure.
- **`suggestions`** (string array) — candidate recovery actions.
- **`retryable`** (bool) — whether a naive retry of the same command is safe.
  Tri-state: `true` (safe), `false` (do **not** retry), or absent (unspecified);
  absence is distinguishable from explicit `false`, set via `ax.WithRetryable`.
- **`retry_after_seconds`** (integer) — relative backoff, in whole seconds, before
  a retry should be attempted (delta-seconds, never an absolute timestamp, so
  output stays byte-identical across runs); meaningful only when `retryable` is
  `true`, set via `ax.WithRetryAfterSeconds`.

All optional fields are omitted when unset, so a default error envelope is
byte-identical to one emitted before these fields existed.

## Engineering Standards

- **Allocation discipline:** track allocations via standard `testing.B`
  benchmarks rather than asserting numeric bars. The logger hot path is
  measured by `BenchmarkLogger*` (see
  [spec 011](specs/011-hot-path-benchmarks/)): the enabled emit path, the
  filtered fast path, the no-trace-context path, typed fields, and labelled
  loggers all measure **0 allocs/op**. The single allocating path is emitting
  with an active trace context (**2 allocs/op**, ~48 B/op) from formatting the
  hex trace/span IDs — a bounded, documented exception, not a regression. Run
  `go test -run '^$' -bench '^BenchmarkLogger' -benchmem ./...` to reproduce.
- **Trace propagation:** contexts carry and propagate W3C Trace Context IDs by
  default, via the OpenTelemetry SDK in the root `ax` package
  ([ADR-0004](docs/adr/0004-trace-id-format.md);
  real export lifecycle delivered by
  [spec 004](specs/004-real-otel-export/)). The import-isolated packages
  (`contract`, `config`, `schema`, `id`) carry no live tracing — see
  [Public Import Surfaces](#public-import-surfaces).
- **Observability backends:** Grafana Loki for log aggregation via
  opt-in direct push (`AX_LOKI_URL`; see
  [`specs/007-loki-direct-push/research.md`](specs/007-loki-direct-push/research.md));
  Tempo / Jaeger / Honeycomb-compatible for traces via OTel.
- **ID strategy:** OTel trace/span IDs for observability; UUID v4 for
  idempotency keys; UUID v7 for resource and entity IDs. Never mix
  observability IDs with resource/entity IDs.
- **CLI framework:** built on Cobra ([ADR-0008](docs/adr/0008-cli-framework-cobra.md)).
  `ax.Execute()` wraps Cobra execution for mode resolution, schema wiring,
  error-envelope output, and OTel flush-on-exit.
- **Structured logging:** `ax.NewLogger(ctx)` returns an `ax.Logger` backed by
  zerolog with trace correlation wired in (Constitution Principle VIII; the
  single-backend guardrail is Principle VI).
- **Telemetry lifecycle:** `ax.Execute()` opens a recording root span around the
  command, so logs written with `cmd.Context()` carry non-zero `trace_id` and
  `span_id` even when no collector is configured. Set
  `OTEL_EXPORTER_OTLP_ENDPOINT` to enable OTLP HTTP trace export; a plaintext
  `http://` local collector is allowed, while `https://` uses verified TLS and
  TLS verification is never disabled. Set `AX_OTEL_DEBUG=1` to print
  human-readable span data to `stderr` for local debugging. Both destinations
  can be enabled together, all telemetry stays off `stdout`, and exporter
  failures degrade to a `stderr` diagnostic without changing the command's
  `stdout` payload or exit code. Export attempts and shutdown are bounded by the
  telemetry shutdown budget (default `2s`, configurable with
  `ax.WithTelemetryShutdownTimeout`).
- **Idiomatic Go:** package name is `ax`. Keep abstractions narrow and tied to
  accepted ADRs.

## Architecture Decisions (ADRs)

The ADRs are a frozen legacy decision log. New public API or runtime behavior
changes go through the Spec Kit workflow and record decisions in the feature's
`research.md`; retired ADR decisions are absorbed there before the ADR file is
deleted.

| ADR | Title | Status |
| --- | --- | --- |
| [0004](docs/adr/0004-trace-id-format.md) | Trace ID Format | **Accepted (2026-05-28)** |
| [0008](docs/adr/0008-cli-framework-cobra.md) | CLI Framework — Cobra | **Accepted (2026-05-28)** |

Absorbed decisions for mode resolution, error envelopes, schema output, ID
strategy, and import layout live in
[`specs/010-import-isolated-contracts/research.md`](specs/010-import-isolated-contracts/research.md).
The structured-logging (zerolog) choice is now governed by Constitution
Principles VI and VIII; its full decision record is absorbed into
[`specs/011-hot-path-benchmarks/research.md`](specs/011-hot-path-benchmarks/research.md).
Remaining frozen ADR text and rationale live in [`docs/adr/`](docs/adr/).

## Repository Layout

The primary public import path remains `github.com/rshade/ax-go` as `ax`.
Narrow public contract packages exist only for thin consumers:
`contract`, `config`, `schema`, and `id`. Private implementation mechanics live
under [`internal/`](internal/) so they do not become accidental public API.
Public JSON contract fixtures live under [`testdata/`](testdata/). Runnable
support binaries belong under `cmd/` when real command behavior exists. `pkg/`,
`src/`, and broad public subpackages remain intentionally avoided.

## Examples

The runnable integration command in
[`examples/integration/`](examples/integration/) exercises the public `ax-go`
API from a real Cobra CLI. It covers bounded JSON envelopes, NDJSON streaming,
Hujson config parsing, in-place Hujson patching, `__schema`, structured
`ax.Error` output, idempotency keys, and stderr logging.

```sh
go run ./examples/integration --format=json --idempotency-key=demo-key --name=Ada
go run ./examples/integration stream --format=json --count=3
go run ./examples/integration patch-config --format=json --config=config.json \
  --patch='[{"op":"replace","path":"/name","value":"Grace"}]'
go run ./examples/integration __schema
go run ./examples/integration fail --format=json
```

## Build-time version injection

Production CLIs built on ax-go should resolve their version once at process
startup and pass the same value to every version surface. Keep the linker target
as a writable `var`, then use `ax.ResolveVersion`:

```go
var version string // set by -ldflags "-X main.version=..."

func run(ctx context.Context, root *cobra.Command) int {
    resolved := ax.ResolveVersion(version)

    logger := ax.NewLogger(ctx, ax.WithLoggerLabels(ax.Labels{
        Application: "mytool",
        Version:     resolved,
    }))
    _ = logger

    return ax.Execute(ctx, root, ax.WithVersion(resolved))
}
```

`ResolveVersion` returns a non-placeholder injected value when present,
otherwise it falls back to the running binary's Go build metadata
(`Main.Version`, then `vcs.revision` with a dirty marker) and finally to
`0.0.0-unknown`. It never returns an empty string or the bare placeholders
`dev` or `unknown`.

Build the integration example with the documented injection target:

```sh
make build-example
./bin/ax-integration __schema
```

## Slimming the binary (optional OTLP and gRPC)

Two **optional, negative** build constraints let you decline the two heaviest
dependency roots. Both default to off, so existing consumers need do nothing —
setting neither tag is the state every build is already in.

| Tag | Declines | Default |
| --- | --- | --- |
| `ax_no_otlp` | OTLP HTTP trace export | absent (export active) |
| `ax_no_grpc` | `ax.GRPCDial` and gRPC client instrumentation | absent (helper present) |

```sh
go build -tags=ax_no_grpc,ax_no_otlp -ldflags="-s -w" ./cmd/yourcli
```

### Decline both, or neither

The size benefit requires **both**. Each tag removes one of _two independent
roots_ over the same gRPC subtree, so one alone buys you almost nothing:

| Tags | Δ stripped size | grpc packages left |
| --- | ---: | ---: |
| `ax_no_grpc` | −0.00% | 66 |
| `ax_no_otlp` | −15.1% | 64 |
| **both** | **−63.3%** | **0** |

Measured on linux/amd64 against a root-facade fixture consumer
(14,893,218 → 5,460,130 bytes); windows/amd64 gives −62.5%. With both tags set,
`go list -deps` reports exactly zero packages from `google.golang.org/grpc`,
`google.golang.org/protobuf`, `go.opentelemetry.io/proto/otlp`, and
`github.com/grpc-ecosystem/grpc-gateway/v2`.

### What you keep

Tracing degrades to _no export_, never to _no tracing_. In every configuration:

- W3C `TRACEPARENT`/`TRACESTATE` extraction and continuation
- a recording root span around `ax.Execute`
- `trace_id`/`span_id` on every log line emitted inside that span
- `AX_OTEL_DEBUG` local span output on stderr
- `ax.HTTPClient` / `ax.NewHTTPClient`, fully instrumented
- byte-identical `__schema` and `ax.Error` payloads, stream separation, exit
  codes, `--dry-run`, and `--idempotency-key`

### What you give up

**With `ax_no_otlp`:** OTLP network export. A configured
`OTEL_EXPORTER_OTLP_ENDPOINT` is _not_ an error — ax stays fail-open, emitting
one `ax: otel exporter disabled: …` line on stderr per telemetry start and
succeeding normally. That is by design, but it means a misconfigured build
degrades silently to no export. Watch for that diagnostic.

**With `ax_no_grpc`:** `ax.GRPCDial` is absent from the build. Calling it fails
at compile time with Go's standard `undefined: ax.GRPCDial`; the toolchain gives
a library no way to customise that message, so the explanation lives in the
package documentation (`grpc_disabled.go`). To restore it, drop the tag — nothing
else changes: no source edit, no import change, no API difference.

`ax.GRPCDial` is the **only** public identifier whose presence varies with a
build tag. This is enforced on every PR by `make surface-check`, which
type-checks all seven public packages across 4 configurations × 6 GOOS/GOARCH
profiles and diffs the result against a committed baseline.

The target injects `git describe --tags --always --dirty` into
`main.version`:

```sh
go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" \
  -o bin/ax-integration ./examples/integration
```

Override `VERSION` for release and reproducible builds:

```sh
make build-example VERSION=v1.2.3
```

The same resolved value feeds `__schema.version`, the `ax.Error` envelope
`version`, and the logger `version` label.

## Roadmap

The live roadmap — immediate focus, sequenced near-term and long-term items,
and completed milestones — is tracked in [`ROADMAP.md`](ROADMAP.md), with
every open item filed as a labeled GitHub issue.

## Compatibility

### ax-go version and Go version

| ax-go version   | Minimum Go version | Notes                                                         |
| --------------- | ------------------ | ------------------------------------------------------------- |
| v0.x (current)  | 1.26.5             | Pre-v1.0; `0.x.PATCH` is always safe; `0.MINOR.0` may break |
| v1.x            | TBD                | Stable API; targeted via the Spec Kit stability workflow      |

The minimum Go version is the `go` directive in [`go.mod`](go.mod). Patch
releases (`0.x.PATCH`) are always backward-compatible. Minor releases
(`0.MINOR.0`) may break either the Go API surface or machine-payload shapes
(`ax.Error`, `__schema`). See
[Constitution Principle XI](.specify/memory/constitution.md) for the full
stability and SemVer policy.

### Downstream consumers

| Consumer | Pinned ax-go version | Notes |
| -------- | -------------------- | ----- |
| _(no downstream consumers yet — first consumer pinning notes will appear here)_ | — | — |

Once a downstream project tags a release that pins an ax-go version, add a row
here. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the update process.

## Brand

The ax-go mascot (a robot gopher shouldering an axe), the geometric logomark,
the color palette, and usage guidelines live in
[`docs/brand/`](docs/brand/README.md). The logomark also serves as the docs
site-header logo and the source of the [favicon](docs/public/favicon.svg).

## Contributing

Before changing public behavior, use the Spec Kit feature workflow. Read the
constitution, absorb any governing frozen ADR decisions into the feature's
`research.md`, and keep README plus `examples/integration/` current with the
public contract. Do not create or edit ADRs for new work.

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the compatibility matrix update
process and release checklist.

## License

Licensed under the [Apache License 2.0](LICENSE).
