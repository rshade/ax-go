---
title: Choose a logging surface
description: Take ax-go's trace-correlated logger without linking the telemetry runtime — and know when you still need the root package.
sidebar:
  order: 2
---

`ax-go` exposes its logger through two public import paths. They give you the
same logger — literally the same type, backed by the same implementation — but
they pull in very different amounts of code.

This guide shows you how to pick one, and what changes if you pick wrong.

## The short answer

| You need | Import |
| --- | --- |
| Logging only, smallest binary | `github.com/rshade/ax-go/logging` |
| Logging **plus** Loki direct push | root `github.com/rshade/ax-go` |
| Logging plus OpenTelemetry export or `ax.Execute` | root `github.com/rshade/ax-go` |

If you are writing a library, or a CLI that ships as a cross-compiled binary
and does not export traces, you want `logging`.

## Logging only

```go
package main

import (
    "context"

    "github.com/rshade/ax-go/logging"
)

func main() {
    ctx := context.Background()

    log := logging.NewLogger(ctx,
        logging.WithLoggerLabels(logging.Labels{
            Application: "my-cli",
            Environment: "prod",
        }),
    )

    log.Info(ctx).Str("stage", "startup").Msg("ready")
}
```

No build tags. No configuration. The line goes to stderr as JSON, carrying
`trace_id` and `span_id` — the real values when a span is active in `ctx`, and
valid zero-value hex constants when none is, so your log parser never has to
branch on a missing field.

## What you save

Measured on linux/amd64 with Go 1.26.5 and `-trimpath -ldflags="-s -w"`:

| Program | Linked packages | Stripped binary |
| --- | --- | --- |
| imports `logging` | 103 | **2,261,257 bytes** |
| imports root `ax` | 410 | 12,013,833 bytes |
| difference | −307 | **−81.2%** |

The isolated surface links none of the following, and a test asserts it under
all four supported build configurations:

- the OpenTelemetry SDK and every OTLP exporter
- gRPC and protobuf
- Cobra
- `net/http` and `crypto/tls`

`net/http` is the single largest lever. Excluding it is most of the 81%.

## What you give up

Exactly two things, and both for the same reason — they need dependencies the
isolation exists to exclude:

**Loki direct push.** `ax.WithLokiFromEnv()` and the `AX_LOKI_URL` /
`AX_LOKI_AUTH_TOKEN` variables live in root `ax`, because pushing to Loki needs
`net/http`. This is not a gap to be filled later; it is the boundary working as
designed. Note that direct push is the *opt-in* path anyway — ax-go's default log
shipping is decoupled (stderr → Promtail/Alloy), and that works identically from
either surface, because it is just stderr.

**Span creation and export.** `logging` *reads* an active span context, which is
why correlation works. It cannot *create* or *export* spans — ID generation and
export need the SDK. If you call `ax.StartTelemetry`, you are already importing
root `ax`.

`logging.Flush` exists and always returns `nil` for a consumer of `logging`
alone: the only destination that buffers anything is the Loki sink, and it is
unreachable from here. Calling it unconditionally in your shutdown path is safe,
and keeps working unchanged if you later migrate to the root facade.

## Mixing both is fine

The two surfaces name **one** logger. `ax.Logger` and `logging.Logger` are the
same type, not two similar ones, so all of this compiles:

```go
var a ax.Logger = logging.NewLogger(ctx)
var b logging.Logger = ax.NewLogger(ctx)

_ = ax.Flush(ctx, logging.NewLogger(ctx))
_ = logging.Flush(ctx, ax.NewLogger(ctx))

// An option built by root ax, accepted by the isolated constructor:
log := logging.NewLogger(ctx, ax.WithLokiFromEnv())
```

That last line is the sharpest demonstration: an option manufactured by root
`ax`, driving root-owned machinery, handed to `logging.NewLogger`. It works
because `ax.LoggerOption` and `logging.LoggerOption` are aliases of one
declaration.

A practical consequence: a library can accept a `logging.Logger` and stay small,
while the application that calls it uses root `ax` and passes its own
Loki-enabled logger straight in. Neither side converts anything.

:::note[Why isolation isn't just a build tag]
`ax-go` also has `ax_no_otlp` and `ax_no_grpc` build constraints that shrink a
root-facade consumer. Those are a different axis: they require every consumer to
know about and pass a flag, and they shrink a build you are already making.
A package boundary delivers the reduction by import alone, and a `logging`
consumer's dependency graph is byte-identical under all four tag combinations —
because it never links the trees those tags decline.
:::

## Verify it yourself

Confirm nothing forbidden is reachable:

```sh
go list -deps github.com/rshade/ax-go/logging | grep -E \
  '^github.com/rshade/ax-go$|^go.opentelemetry.io/otel/sdk|^google.golang.org/grpc|^net/http$|^crypto/tls$'
```

Expect no output. Then confirm the parts that must be there:

```sh
go list -deps github.com/rshade/ax-go/logging | grep -E 'zerolog|otel/trace'
```

Anchor those patterns. A bare `internal/telemetry` also matches OpenTelemetry's
own `otel/trace/internal/telemetry`, which is a legitimate part of the trace API
and not an `ax-go` runtime import.

## Related

- [Expose your command tree with `__schema`](/ax-go/guides/expose-schema/) — for
  the full CLI runtime, which is a root `ax` concern.
- [Why Agentic Experience?](/ax-go/explanation/why-agentic-experience/) — why
  stream separation puts every log line on stderr in the first place.
