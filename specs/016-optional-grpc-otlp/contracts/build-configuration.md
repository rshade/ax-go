# Contract: Build Configuration Surface & Behaviour

**Feature**: `016-optional-grpc-otlp` | **Status**: Design | **Date**: 2026-07-22

The external contract this feature exposes is not a Go API addition — it is a
set of **build constraints** consumers may set, plus the guarantees that hold
under each. This document is the normative statement of that contract.

---

## C1. Tag Contract

| Tag | Polarity | Declines | Default |
| --- | --- | --- | --- |
| `ax_no_otlp` | negative | OTLP HTTP trace export | absent (export active) |
| `ax_no_grpc` | negative | `ax.GRPCDial` + gRPC client instrumentation | absent (helper present) |

**Usage**

```bash
go build -tags=ax_no_grpc,ax_no_otlp ./...
```

**Guarantees**

- Setting neither tag is the default and is the state every existing consumer is
  already in. No action is required of anyone.
- The tags are independent; all four combinations are supported and built in CI.
- Tag names are stable public contract. Renaming one is a breaking change under
  Constitution Principle XI and rides a `0.MINOR.0`.
- No third tag will be added under this feature.

---

## C2. Public Go Surface

| Identifier | `default` | `no-grpc` | `no-otlp` | `minimal` |
| --- | :---: | :---: | :---: | :---: |
| `ax.GRPCDial` | ✅ | ❌ | ✅ | ❌ |
| `ax.HTTPClient`, `ax.NewHTTPClient`, `ax.WithHTTPTimeout`, `ax.DefaultHTTPTimeout` | ✅ | ✅ | ✅ | ✅ |
| `ax.StartTelemetry`, `ax.Telemetry`, `ax.Telemetry.TracerProvider`, `ax.WithTelemetry*` | ✅ | ✅ | ✅ | ✅ |
| `ax.Execute`, `ax.NewLogger`, `ax.NewSchemaCommand`, `ax.Error`, … | ✅ | ✅ | ✅ | ✅ |
| `config`, `contract`, `id`, `mcp`, `schema` package surfaces | ✅ | ✅ | ✅ | ✅ |

**`ax.GRPCDial` is the only identifier whose presence varies.** Verified: the
public telemetry surface exposes no grpc/otlp type, so declining export removes
nothing from the API.

**Under `ax_no_grpc`**, calling `ax.GRPCDial` fails at build time with the Go
toolchain's standard `undefined: ax.GRPCDial`. The toolchain provides no hook to
customise that message; the explanation is delivered as package documentation
(C5), not as a compiler diagnostic.

---

## C3. Runtime Behaviour

Identical in **all four** configurations:

| Behaviour | Guarantee |
| --- | --- |
| Stream separation | `stdout` = payload only; `stderr` = everything else |
| Exit codes | `0`/`1`/`2`/`3`/`4` mapping unchanged |
| `__schema` output | Byte-identical, matches existing golden fixtures |
| `ax.Error` envelope | Byte-identical, matches existing golden fixtures |
| Agent-mode precedence | `--format` > `AGENT_MODE` > TTY |
| `--dry-run`, `--idempotency-key` | Unchanged |
| W3C `TRACEPARENT`/`TRACESTATE` extraction | Unchanged |
| Root span around `Execute` | Created and recording |
| `trace_id`/`span_id` on log lines | Present whenever a span is active |
| `AX_OTEL_DEBUG` span output | Available (`stdouttrace` pulls zero gRPC) |
| Telemetry shutdown budget | `DefaultShutdownBudget` (2s), never blocks on an absent exporter |

Varying **only** with `ax_no_otlp`:

| Behaviour | `!ax_no_otlp` | `ax_no_otlp` |
| --- | --- | --- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` set | Spans exported over HTTP | No export; one diagnostic |
| Diagnostic on `stderr` | Only on real failure | `ax: otel exporter disabled: …` once per telemetry start |
| Exit code | Unaffected by export outcome | Unaffected — fail-open preserved |
| `StartTelemetry` error return | Always `nil` | Always `nil` |
| Returned `TracerProvider` | Recording | Recording |

**Fail-open is absolute**: an unreachable, misconfigured, or declined exporter
never fails a command. Tracing degrades to *no export*, never to *no tracing*.

---

## C4. Dependency Contract

Under `ax_no_grpc,ax_no_otlp`, a root-facade consumer's `go list -deps` reports
**exactly zero** packages matching:

```
google.golang.org/grpc
google.golang.org/protobuf
go.opentelemetry.io/proto/otlp
github.com/grpc-ecosystem/grpc-gateway/v2
```

Measured reference (2026-07-22, `741a8d4`, linux/amd64, `-ldflags="-s -w"`):
**410 → 264 packages, 14,897,314 → 5,460,130 bytes (−63.3%)**.

⚠️ **This guarantee holds only for the `minimal` configuration.** The
intermediate configurations retain 66 gRPC packages by design — each decline
alone removes one of two independent roots over a shared subtree.

---

## C5. Documentation Contract

`README.md` and package documentation MUST state:

1. Both tags, what each declines, and the usage form.
2. That the size benefit requires **both** — quoting the measured
   −0.03% / −15.1% / −63.3% ladder so nobody ships one and reports a regression.
3. That `ax_no_grpc` removes `ax.GRPCDial` from the build, and how to restore it.
4. **That the thin `contract`/`config`/`schema`/`id` packages carry no live
   tracing** — `contract.TraceIDFromContext` reads a value previously stored in
   the context and does not resolve an active span. Live tracing exists only in
   the root facade.

`internal/telemetry/otlp_disabled.go` and the root `grpc_disabled.go` carry
file-level doc comments naming the responsible tag and the restoration step.
Neither declares an exported symbol, and neither references any type from the
forbidden trees.

---

## C6. Gate Contract (`internal/cmd/surfacecheck`)

```
surfacecheck [-baseline <path>] [-audit <path>] [-list] [-audit-seed] [-update]
```

> **Superseded during implementation.** This feature was specified against a
> repository with no surface gate (see spec.md § Resolved Clarifications).
> PR #148 landed a different `internal/cmd/surfacecheck` — a root-only gate
> with a permanent audit and an `ax.Error` failure contract — before this work
> merged. Rather than ship two gates at one import path, the two were unified:
> this feature's configuration axis and six-package scope were grafted onto
> #148's deeper export-data scanner. The exit and stream contract below is
> #148's, which supersedes the `0`/`1`/`2` scheme originally specified here.

A pass writes one minified JSON object to `stdout`, nothing to `stderr`, and
exits `0`. Every failure writes nothing to `stdout` and exactly one minified
`ax.Error` envelope to `stderr`:

| Error code | Exit | Meaning |
| --- | --- | --- |
| `surface_drift` | `2` | A configuration × profile disagrees with the baseline or audit |
| `invalid_surface_artifact` | `2` | Baseline/audit missing, malformed, oversized, or schema-invalid; invalid flags |
| `surface_permission` | `4` | Permission denial reading artifacts or executing tooling |
| `surface_internal` | `1` | Unexpected internal failure |

Each drifted feature is named with its configuration and profile in the
envelope's `suggestions`.

**Fail-closed requirement (normative)**: the gate MUST assert that every
requested import path came back from `go list` with usable export data.
A loader that reports success while returning nothing for a requested path —
`packages.Load` does exactly this on an invalid `GOOS` — would make the gate
pass vacuously. A regression test MUST assert that a deliberately bogus
profile fails rather than reporting an empty surface.

**Local invocation** MUST be identical to CI's:

```bash
make surface-check
```

**Policy as constants**: the tag combinations, profile list, and public package
list are hardcoded Go constants in `inventory.go`, so a matrix change is a
reviewable commit auditable via `git blame` — matching `covercheck` and
`benchcheck`.
