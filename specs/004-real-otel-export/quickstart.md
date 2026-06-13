# Quickstart: Real OTel Export & Span Lifecycle

**Feature**: `004-real-otel-export` | **Date**: 2026-06-11

How telemetry behaves in a CLI built on ax-go. A root span now wraps every
command, so logs are trace-correlated out of the box, spans reach a collector
when one is configured (guaranteed flushed before exit), and a single debug
variable prints spans locally — all without ever touching `stdout`.

## 1. Correlation for free — no collector, no config

Build your CLI with `ax.Execute` and log via `cmd.Context()`:

```go
func run(ctx context.Context, args []string, stdout, stderr io.Writer, env func(string) string) int {
    root := newRootCommand()
    return ax.Execute(ctx, root,
        ax.WithStdout(stdout), ax.WithStderr(stderr),
        ax.WithEnv(env), ax.WithVersion(version),
    )
}

// inside a command's RunE:
logger := ax.NewLogger(cmd.Context(), ax.WithLoggerWriter(cmd.ErrOrStderr()))
logger.Info(cmd.Context()).Str("event", "ran").Msg("did the thing")
```

Every `stderr` log line now carries a **non-zero** `trace_id`/`span_id`
identifying this run — identical across the run's lines, with no inbound trace
and no collector. (Previously these were the all-zeros placeholders.)

> Log with `cmd.Context()`. That is the context the root span lives on; a
> `context.Background()` you create yourself has no active span and will log the
> documented all-zeros fallback.

## 2. Export to a collector (Tempo / Jaeger / Honeycomb via OTLP)

Set the standard OTLP endpoint variable; ax-go auto-configures an OTLP HTTP
exporter and **guarantees the run's spans are flushed before the process exits**:

```bash
# local collector over plaintext HTTP (permitted)
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
mytool do-something

# remote collector over verified TLS
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otlp.example.com
mytool do-something
```

- The endpoint's scheme is honored: `http://` is plaintext (no TLS), `https://`
  uses **verified** TLS. Certificate verification is never disabled.
- An inbound `TRACEPARENT` is continued — the exported spans carry the inbound
  `trace_id`, so a multi-process workflow stitches into one end-to-end trace. The
  span is recorded and exported **even if the inbound trace is marked
  not-sampled** (`flags=00`): ax-go always samples its own root span.

```bash
# continue an orchestrator's trace
export TRACEPARENT=00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
mytool do-something    # logs + exported spans share that trace_id
```

## 3. See spans locally without a collector

```bash
export AX_OTEL_DEBUG=1
mytool do-something     # human-readable span data on stderr; nothing on stdout
```

Strictly opt-in: without `AX_OTEL_DEBUG`, no span data is printed anywhere. If
both `AX_OTEL_DEBUG` and `OTEL_EXPORTER_OTLP_ENDPOINT` are set, **both**
destinations receive the spans.

## 4. Telemetry never breaks the command (fail-open)

```bash
# unreachable / malformed endpoint
export OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:1   # nothing listening
mytool do-something
echo $?    # unchanged exit code; stdout is the normal machine payload
```

A failing, unreachable, or malformed collector degrades to a `stderr` diagnostic
only. It never changes the command's exit code and never appears on `stdout`. A
genuinely stuck collector cannot hang the CLI — the export is bounded by the
shutdown budget (default 2s; tune with `ax.WithTelemetryShutdownTimeout`).

## 5. Outbound calls inherit the trace

Use the provided helpers so downstream services see this command as the parent:

```go
resp, err := ax.HTTPClient().Do(req.WithContext(cmd.Context()))
conn, err := ax.GRPCDial(cmd.Context(), "svc:443")
```

The active root span's context propagates automatically (W3C headers).

## Behavior cheat-sheet

| Environment | Logs (`stderr`) | Export | `stdout` |
|---|---|---|---|
| nothing set | non-zero correlated IDs | none (no-op, zero footprint) | unchanged, byte-identical |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | correlated IDs | OTLP HTTP, flushed before exit | unchanged |
| `AX_OTEL_DEBUG` | correlated IDs | span text on `stderr` | unchanged |
| both | correlated IDs | OTLP **and** `stderr` | unchanged |
| inbound `TRACEPARENT` | inbound `trace_id` | same `trace_id`, always sampled | unchanged |
| collector unreachable/bad | correlated IDs | `stderr` diagnostic only | unchanged, exit code unchanged |

## Verify locally

```bash
go test -race ./...        # correlation, export-to-receiver, debug, fail-open, race-clean
make doc-coverage          # add ExampleStartTelemetry, ratchet it off baseline.txt; stays green
make lint                  # golangci-lint (incl. godoclint require-doc) + markdownlint
```
