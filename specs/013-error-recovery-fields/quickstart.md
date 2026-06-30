# Quickstart: Error-envelope recovery fields

**Feature**: `013-error-recovery-fields`

## Producer — emit retry guidance

```go
err := ax.NewError(ctx, "network_timeout", "upstream timed out",
    ax.WithErrorTool("app"),
    ax.WithErrorVersion(version),
    ax.WithErrorExitCode(ax.ExitNetwork),   // exit 3
    ax.WithActionableFix("retry the request"),
    ax.WithSuggestions("check upstream status"),
    ax.WithRetryable(true),                 // NEW: naive retry is safe
    ax.WithRetryAfterSeconds(30),           // NEW: wait 30s first
)
ax.WriteError(os.Stderr, err)
```

Emits to **stderr** (strict, minified JSON, trailing newline):

```json
{"error_code":"network_timeout","message":"upstream timed out","trace_id":"…","tool":"app","version":"…","schema_version":"1.0.0","actionable_fix":"retry the request","suggestions":["check upstream status"],"retryable":true,"retry_after_seconds":30}
```

## Producer — mark a failure as explicitly non-retryable

```go
err := ax.NewError(ctx, "validation_error", "name is required",
    ax.WithErrorExitCode(ax.ExitValidation),  // exit 2
    ax.WithRetryable(false),                   // explicit: do NOT retry
)
```

Emits `"retryable":false` — distinguishable from absence (the agent learns "this
will fail again," not "unknown").

## Producer — set nothing (default, unchanged)

```go
err := ax.NewError(ctx, "validation_error", "bad input")
```

No `retryable` / `retry_after_seconds` keys appear; bytes are identical to the
pre-feature envelope.

## Consumer — branch on the signal

```go
var e *ax.Error
if errors.As(runErr, &e) {
    switch {
    case e.Retryable != nil && *e.Retryable:
        time.Sleep(time.Duration(e.RetryAfterSeconds) * time.Second) // 0 ⇒ immediate
        return retry()
    case e.Retryable != nil && !*e.Retryable:
        return fmt.Errorf("non-retryable: %w", runErr) // give up cleanly
    default:
        return escalate(runErr) // no guidance — apply your own policy
    }
}
```

## Notes

- `retry_after_seconds` is **relative seconds**; the consumer computes its own
  wake time. Two runs of the same failure are byte-identical.
- `Retryable` is a `*bool`: `nil` = unspecified, `&true`/`&false` = explicit. The
  `WithRetryable(bool)` option hides the pointer from producers.
- Negative `WithRetryAfterSeconds(n)` is ignored (field omitted).
