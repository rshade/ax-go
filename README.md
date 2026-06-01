# ax-go

> Agentic Experience (AX) foundation for Go CLI tools — the "Common DNA" for
> the rshade portfolio.

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](go.mod)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

> **Status: 🚧 Implementation scaffold.** The module, license, accepted ADRs,
> and initial Go package skeleton are in place. The public API is still early,
> but core primitives such as `ax.Error`, `ax.Execute`, `ax.NewLogger`,
> `ax.ParseConfig`, and `ax.NewEntityID` now compile and are covered by focused
> contract tests.

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
  default; use `ax.WithMaxConfigBytes` when a CLI intentionally supports a
  larger bounded config.
- **Output:** emit strict, minified JSON for bounded payloads; emit NDJSON for
  streaming / unbounded result sets.

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
  ([ADR-0004](docs/adr/0004-trace-id-format.md),
  [ADR-0005](docs/adr/0005-otel-integration.md)).
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
- **Idiomatic Go:** package name is `ax`. Keep abstractions narrow and tied to
  accepted ADRs.

## Architecture Decisions (ADRs)

ax-go is ADR-driven: each decision is documented and pressure-tested before code
locks it in. These ADRs define the current public API direction.

| ADR | Title | Status |
| --- | --- | --- |
| [0001](docs/adr/0001-agent-mode-trigger.md) | Agent-Mode Trigger | **Accepted (2026-05-28)** |
| [0002](docs/adr/0002-error-envelope-schema.md) | JSON Error Envelope Schema | **Accepted (2026-05-28)** |
| [0003](docs/adr/0003-schema-output-format.md) | `__schema` Output Format | **Accepted (2026-05-28)** |
| [0004](docs/adr/0004-trace-id-format.md) | Trace ID Format | **Accepted (2026-05-28)** |
| [0005](docs/adr/0005-otel-integration.md) | OpenTelemetry SDK Integration | **Accepted (2026-05-28)** |
| [0006](docs/adr/0006-loki-integration.md) | Grafana Loki Backend Integration | **Accepted (2026-05-28)** |
| [0007](docs/adr/0007-id-strategy.md) | ID Strategy | **Accepted (2026-05-28)** |
| [0008](docs/adr/0008-cli-framework-cobra.md) | CLI Framework — Cobra | **Accepted (2026-05-28)** |
| [0009](docs/adr/0009-logger-zerolog.md) | Structured Logger — ZeroLog | **Accepted (2026-05-28)** |
| [0010](docs/adr/0010-input-config-hujson.md) | Input Config Format — Hujson | **Accepted (2026-05-28)** |
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
Hujson config parsing, `__schema`, structured `ax.Error` output, idempotency
keys, and stderr logging.

```sh
go run ./examples/integration --format=json --idempotency-key=demo-key --name=Ada
go run ./examples/integration stream --format=json --count=3
go run ./examples/integration __schema
go run ./examples/integration fail --format=json
```

## Roadmap

Sequenced from the accepted ADRs and the current scaffold:

1. **Complete telemetry exporters** — keep the no-op default, add
   `OTEL_EXPORTER_OTLP_ENDPOINT` OTLP/HTTP auto-configuration, and add the
   `AX_OTEL_DEBUG=1` stderr exporter path from
   [ADR-0005](docs/adr/0005-otel-integration.md).
2. **Harden `__schema`** — enforce example coverage, expand output-mode
   declarations, and mature the MCP adapter from
   [ADR-0003](docs/adr/0003-schema-output-format.md).
3. **Implement Loki direct push** — keep stderr shipping as the default and add
   opt-in `AX_LOKI_URL` direct push from
   [ADR-0006](docs/adr/0006-loki-integration.md).
4. **Expand examples and benchmarks** — keep
   [`examples/integration/`](examples/integration/) current with public API
   changes and benchmark hot paths with `testing.B` / `-benchmem`.

## Contributing

Decisions are made the ADR way: a proposal is captured in `docs/adr/`,
pressure-tested for trade-offs (the advisory-architect process described in
[`IMPLEMENTATION_PROMPT.md`](IMPLEMENTATION_PROMPT.md)), and only then accepted
to guide implementation.

Before changing public behavior, read the relevant ADR and update or supersede
it when the decision surface changes.

## License

Licensed under the [Apache License 2.0](LICENSE).
