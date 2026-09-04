# Quickstart: Loki Direct-Push Addon

**Feature**: `007-loki-direct-push` | **Date**: 2026-06-14

---

## For CLI Authors

Add `ax.WithLokiFromEnv()` to your `NewLogger` call once and register the
late-bound logger with the shutdown lifecycle already owned by `ax.Execute`.

```go
// Declare this next to the Cobra root so the Execute shutdown closure can see
// the logger assigned later in RunE/PersistentPreRunE.
var logger ax.Logger

// In command setup:
logger = ax.NewLogger(
    cmd.Context(),
    ax.WithLoggerWriter(cmd.ErrOrStderr()),
    ax.WithLoggerLabels(ax.Labels{
        Application: "my-tool",
        Environment: os.Getenv("ENV"),
        Version:     version.String(),
    }),
    ax.WithLokiFromEnv(),  // ← add this line; no-op when AX_LOKI_URL is unset
)

// At the process boundary:
return ax.Execute(
    ctx,
    root,
    ax.WithFlushFunc(func(shutdownCtx context.Context) error {
        return ax.Flush(shutdownCtx, logger)
    }),
)
```

`Execute` calls the flush closure once after every normal command return path
and before telemetry shutdown. It supplies a fresh context bounded by the same
duration configured with `ax.WithTelemetryShutdownTimeout`; flush and telemetry
receive independent windows. A flush error becomes a single-line sanitized
`stderr` diagnostic and never changes the command exit code. `ax.Flush` is
nil-safe when parsing or pre-run setup fails before logger construction.
Sanitization prevents control-character line forging but is not redaction, so
the callback must not return an error containing PII, secrets, tokens, or
credentials.

The `ax.WithLokiFromEnv()` option:

- Is a **no-op** when `AX_LOKI_URL` is not set (no performance impact, no
  network connections).
- Reads `AX_LOKI_URL` and `AX_LOKI_AUTH_TOKEN` at construction time.
- Emits a warning to stderr if `AX_LOKI_URL` is malformed.

---

## For Operators

Set `AX_LOKI_URL` before running any ax-go–based CLI that includes the addon:

```bash
export AX_LOKI_URL=http://loki.example.com:3100
export AX_LOKI_AUTH_TOKEN=my-bearer-token    # optional

my-cli some-command
```

Logs appear both on `stderr` (as before) and in Loki under these stream labels:

```text
{environment="prod", application="my-tool", host="my-host", version="1.2.3", level="info"}
```

`trace_id`, `span_id`, and all other fields remain in the log line body (not
in labels) to preserve Loki cardinality discipline.

---

## Testing the Integration Locally

Start a local Loki instance (Docker):

```bash
docker run -d --name loki -p 3100:3100 grafana/loki:latest
export AX_LOKI_URL=http://localhost:3100
my-cli hello
```

Query the log in Grafana or via LogQL:

```bash
curl -s "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={application="my-cli"}' | jq .
```

---

## Failure Modes

| Situation | Behavior |
|-----------|----------|
| `AX_LOKI_URL` not set | No-op; stderr-only (unchanged from default) |
| Loki unreachable | Entry dropped; warning on stderr at debug level; CLI unaffected |
| Loki returns non-2xx | Batch dropped; same as above |
| Push buffer full | New entries dropped; no blocking |
| Process exits through `ax.Execute` | `WithFlushFunc` drains before telemetry shutdown; `ax.Flush` applies its internal 2-second ceiling |
| Process bypasses Go defers | Buffered entries may be lost; abrupt termination cannot run a shutdown callback |
| Malformed `AX_LOKI_URL` | Warning on stderr at construction; stderr-only fallback |

---

## `examples/integration` Update

The integration example in `examples/integration/main.go` includes
`ax.WithLokiFromEnv()` in its `NewLogger` call and registers a closure around
`ax.Flush` with `ax.WithFlushFunc`, demonstrating the lifecycle-owned pattern
for CLI authors without a command-local timeout/defer.
