# Quickstart: Loki Direct-Push Addon

**Feature**: `007-loki-direct-push` | **Date**: 2026-06-14

---

## For CLI Authors

Add `ax.WithLokiFromEnv()` to your `NewLogger` call once. That's it.

```go
// In your command's setup (e.g. cobra PersistentPreRunE or main)
logger := ax.NewLogger(
    cmd.Context(),
    ax.WithLoggerWriter(cmd.ErrOrStderr()),
    ax.WithLoggerLabels(ax.Labels{
        Application: "my-tool",
        Environment: os.Getenv("ENV"),
        Version:     version.String(),
    }),
    ax.WithLokiFromEnv(),  // ← add this line; no-op when AX_LOKI_URL is unset
)

// In your shutdown path (before os.Exit), drain buffered log entries:
defer func() {
    flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    _ = ax.Flush(flushCtx, logger)
}()
```

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

```
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
| Process exits before flush | `ax.Flush` drains up to 2 seconds; remaining entries dropped |
| Malformed `AX_LOKI_URL` | Warning on stderr at construction; stderr-only fallback |

---

## `examples/integration` Update

The integration example in `examples/integration/main.go` should be updated to
include `ax.WithLokiFromEnv()` in its `NewLogger` call and `ax.Flush` in its
shutdown path, demonstrating the pattern for CLI authors.
