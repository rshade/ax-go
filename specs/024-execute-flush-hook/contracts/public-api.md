# Public API Contract: Execute Shutdown Flush Hook

**Feature**: `024-execute-flush-hook` | **Date**: 2026-09-01

## Surface delta (root package `ax`)

```go
// WithFlushFunc registers flush to run once during Execute shutdown after
// command execution and before telemetry shutdown. Flush receives a fresh
// context bounded by the duration configured with
// WithTelemetryShutdownTimeout. Its timeout window is independent of the
// telemetry shutdown window.
//
// If flush returns an error, Execute writes a control-character-sanitized
// diagnostic to its configured stderr without changing stdout or the command
// exit code. Sanitization is not redaction; flush errors must not contain PII,
// secrets, tokens, or credentials. A nil flush disables the hook. If supplied
// more than once, the last WithFlushFunc option wins.
func WithFlushFunc(flush func(context.Context) error) ExecuteOption
```

`Execute`, `ExecuteOption`, `WithTelemetryShutdownTimeout`, `Flush`, `Logger`,
and every other exported signature remain unchanged. No new package, type,
method, constant, variable, flag, environment variable, or machine-payload field
is added.

## Behavioral contract

| Contract ID | Requirement |
|-------------|-------------|
| EF-01 | Omitted or nil callback means no invocation and preserves current behavior. |
| EF-02 | Last option wins, including a final nil clearing an earlier callback. |
| EF-03 | A registered callback runs exactly once on success and every normal Cobra error return path. |
| EF-04 | Callback runs after command execution/root span completion and before telemetry shutdown. |
| EF-05 | Callback receives a fresh background-derived context bounded by the configured telemetry shutdown duration. |
| EF-06 | Flush and telemetry contexts have independent windows of equal configured duration. |
| EF-07 | Callback errors emit `ax: flush failed: <sanitized>\n` on configured stderr. |
| EF-08 | Callback errors and timeouts never alter stdout, error envelope fields, or returned exit code. |
| EF-09 | The export and behavior are identical in all four supported build configurations and six surface profiles. |

## Stream and exit contract

- The callback may itself write according to its own contract; framework output
  caused by a callback error is `stderr`-only.
- `Execute` computes the command result before shutdown. The callback cannot
  replace it.
- Success remains exit `0`; errors retain their existing deterministic category
  (`1` internal, `2` validation, `3` network, `4` auth/permission).
- Error text uses the same `SanitizeDiagnostic` policy as OTel shutdown: ASCII
  controls and DEL become spaces, preventing extra forged lines.
- Sanitization is not redaction. Callback errors must not contain PII, secrets,
  tokens, or credentials because their remaining text is emitted verbatim.
- Flush diagnostics are operational text, not an `ax.Error` envelope. Existing
  command error envelopes remain authoritative.

## Timeout contract

`WithTelemetryShutdownTimeout(d)` supplies the duration for two separate
background-derived contexts when a flush callback exists:

1. one context passed to the callback;
2. one context passed to telemetry shutdown.

The maximum cooperative shutdown wait can therefore approach `2*d` when both
operations consume their full windows. This is deliberate: preserving the
existing full telemetry budget is more important than imposing one shared
deadline, and it avoids another exported timeout option. As with all Go context
contracts, the callback must observe cancellation; `Execute` cannot forcibly
stop callback code that ignores its context. A non-positive `d` retains the
existing Execute behavior: both newly created outer contexts are immediately
expired.

## Stability and release classification

- **Go API**: additive, supported root-package function.
- **Existing semantics**: unchanged for callers that omit the option.
- **Machine payloads**: unchanged.
- **SemVer**: non-breaking `feat:`; while pre-v1, release-please increments the
  minor digit under Constitution Principle XI.
- **Deprecation**: none.

## Public-surface artifacts

The reviewed artifacts gain this universally present feature:

```json
{
  "id": "func:WithFlushFunc",
  "signature": "func(func(context.Context) error) ExecuteOption",
  "configurations": "all",
  "profiles": "all"
}
```

The permanent audit classifies it `supported`, disposition `keep-public`,
lifecycle `live`. Root `ax` is already allowed by surface/apidiff policy, so no
package allowlist changes.

## Documentation and example contract

- `WithFlushFunc` carries the full contract doc comment above.
- `ExampleExecute` demonstrates the option inside the parent entry-point
  example, satisfying the repository's WithX documentation rule.
- `ExampleFlush` remains because direct flushing is still the correct API for
  lifecycles not owned by `Execute`.
- The integration example and first-CLI tutorial use a closure around
  `ax.Flush(ctx, logger)` and contain no command-local timeout defer.

## Non-goals

- No multiple-hook registry or error aggregation.
- No logger-specific `ExecuteOption`.
- No new flush-timeout option.
- No signal handling, `os.Exit`, or panic recovery.
- No changes to `logging.Flush`, Loki batching, or telemetry implementation.
- No callback invocation after a process-level abrupt termination that bypasses
  Go defers.
