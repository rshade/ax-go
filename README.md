# ax-go

> Agentic Experience (AX) foundation for Go CLI tools — the "Common DNA" for
> the rshade portfolio.

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

> **Status: Released.** The current pinnable release is **v0.0.1**:
> `go get github.com/rshade/ax-go@v0.0.1`. The v0.1.0 output contracts are
> already frozen in code — core primitives such as `ax.Error`, `ax.Execute`,
> `ax.NewLogger`, `ax.ParseConfig`, and `ax.NewEntityID` are covered by
> contract tests, and all public output shapes are pinned by golden fixtures.
> v0.1.0 itself is tagged by release-please from the Conventional Commit
> history; see [CHANGELOG.md](CHANGELOG.md) for release history.

## Mission

ax-go is the shared foundation that standardizes **Agentic Experience (AX)**
across Go-based CLI tools. Its goal is simple:

> Ensure all Go-based CLI tools are as powerful and predictable for LLM agents
> as they are for human engineers.

Rather than every CLI reinventing how it talks to an autonomous agent — how it
emits data, reports errors, exposes its command tree, and stays safe under
retries — ax-go encodes those conventions once so the whole portfolio shares
the same predictable behavior.

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
`__schema --as=mcp` available as an MCP-compatible adapter
([ADR-0003](docs/adr/0003-schema-output-format.md)).

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
- **`--format` flag / `AGENT_MODE` env var / TTY auto-detect** — selects machine
  vs. human output mode ([ADR-0001](docs/adr/0001-agent-mode-trigger.md)).

### Standard `ax.Error` envelope

A structured, machine-readable error format emitted to `stderr`. Schema defined
in [ADR-0002](docs/adr/0002-error-envelope-schema.md).

## Engineering Standards

- **Allocation discipline:** track allocations via standard `testing.B`
  benchmarks; target zero or near-zero allocations on hot paths. Benchmark
  serializer choices rather than asserting numeric bars.
- **Trace propagation:** contexts carry and propagate W3C Trace Context IDs by
  default, via the OpenTelemetry SDK
  ([ADR-0004](docs/adr/0004-trace-id-format.md);
  real export lifecycle delivered by
  [spec 004](specs/004-real-otel-export/)).
- **Observability backends:** Grafana Loki for log aggregation
  ([ADR-0006](docs/adr/0006-loki-integration.md)); Tempo / Jaeger /
  Honeycomb-compatible for traces via OTel.
- **ID strategy:** OTel trace/span IDs for observability; UUID v4 for
  idempotency keys; UUID v7 for resource and entity IDs
  ([ADR-0007](docs/adr/0007-id-strategy.md)). Never mix observability IDs with
  resource/entity IDs.
- **CLI framework:** built on Cobra ([ADR-0008](docs/adr/0008-cli-framework-cobra.md)).
  `ax.Execute()` wraps Cobra execution for mode resolution, schema wiring,
  error-envelope output, and OTel flush-on-exit.
- **Structured logging:** `ax.NewLogger(ctx)` returns an `ax.Logger` backed by
  zerolog with trace correlation wired in
  ([ADR-0009](docs/adr/0009-logger-zerolog.md)).
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
| [0001](docs/adr/0001-agent-mode-trigger.md) | Agent-Mode Trigger | **Accepted (2026-05-28)** |
| [0002](docs/adr/0002-error-envelope-schema.md) | JSON Error Envelope Schema | **Accepted (2026-05-28)** |
| [0003](docs/adr/0003-schema-output-format.md) | `__schema` Output Format | **Accepted (2026-05-28)** |
| [0004](docs/adr/0004-trace-id-format.md) | Trace ID Format | **Accepted (2026-05-28)** |
| [0006](docs/adr/0006-loki-integration.md) | Grafana Loki Backend Integration | **Accepted (2026-05-28)** |
| [0007](docs/adr/0007-id-strategy.md) | ID Strategy | **Accepted (2026-05-28)** |
| [0008](docs/adr/0008-cli-framework-cobra.md) | CLI Framework — Cobra | **Accepted (2026-05-28)** |
| [0009](docs/adr/0009-logger-zerolog.md) | Structured Logger — ZeroLog | **Accepted (2026-05-28)** |
| [0011](docs/adr/0011-output-payload-json.md) | Output Payload Format — Strict JSON / NDJSON | **Accepted (2026-05-28)** |
| [0012](docs/adr/0012-directory-layout.md) | Directory Layout | **Accepted (2026-05-30)** |

Full text and rationale live in [`docs/adr/`](docs/adr/).

## Repository Layout

The public import path remains one package: `github.com/rshade/ax-go` as `ax`.
Per Go's official module layout guidance, public package files stay at the
module root. Private implementation mechanics live under [`internal/`](internal/)
so they do not become accidental public API before v1.0. Public JSON contract
fixtures live under [`testdata/`](testdata/). Runnable support binaries belong
under `cmd/` when real command behavior exists. `pkg/`, `src/`, and broad
public subpackages are intentionally avoided.

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

Sequenced from the accepted ADRs and the current scaffold:

1. **Harden `__schema`** — enforce example coverage, expand output-mode
   declarations, and mature the MCP adapter from
   [ADR-0003](docs/adr/0003-schema-output-format.md).
2. **Implement Loki direct push** — keep stderr shipping as the default and add
   opt-in `AX_LOKI_URL` direct push from
   [ADR-0006](docs/adr/0006-loki-integration.md).
3. **Expand examples and benchmarks** — keep
   [`examples/integration/`](examples/integration/) current with public API
   changes and benchmark hot paths with `testing.B` / `-benchmem`.

## Contributing

Before changing public behavior, use the Spec Kit feature workflow. Read the
constitution, absorb any governing frozen ADR decisions into the feature's
`research.md`, and keep README plus `examples/integration/` current with the
public contract. Do not create or edit ADRs for new work.

## License

Licensed under the [Apache License 2.0](LICENSE).
