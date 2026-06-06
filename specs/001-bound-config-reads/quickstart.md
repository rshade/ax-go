# Quickstart: Bounded Config Reads

**Feature**: `001-bound-config-reads` | **Date**: 2026-06-02

How a CLI built on ax-go loads configuration safely. The read is bounded at the
input boundary, so an accidental or hostile oversized config becomes a
predictable validation error — never an out-of-memory crash.

## 1. Parse a config stream (default 1 MiB cap)

```go
import (
    "context"
    ax "github.com/rshade/ax-go"
)

var cfg AppConfig // your struct with json tags
err := ax.ParseConfig(ctx, r, &cfg) // r is any io.Reader; ctx enables cancelation
```

Hujson is accepted on input — comments and trailing commas are fine. The decoded
result follows strict JSON semantics.

## 2. Parse a config file

```go
err := ax.ParseConfigFile(ctx, "config.hujson", &cfg)
```

Same cap and error behavior as `ParseConfig`; the file is opened and closed for
you.

## 3. Adjust the cap for one invocation

```go
// A consumer with a legitimately larger generated config:
err := ax.ParseConfig(ctx, r, &cfg, ax.WithMaxConfigBytes(4<<20)) // 4 MiB

// A tighter limit for an untrusted context:
err := ax.ParseConfig(ctx, r, &cfg, ax.WithMaxConfigBytes(64<<10)) // 64 KiB
```

The override applies to that call only — there is no global or residual state.

> ⚠️ The cap has a safe maximum, `ax.MaxConfigBytesCeiling` (1 GiB). A cap above
> it — including `math.MaxInt64` — is rejected as `config_max_bytes_invalid`
> (exit `2`); there is no unbounded read path.

## 4. Classify a rejection (agent-friendly)

```go
import "errors"

if err := ax.ParseConfig(ctx, r, &cfg); err != nil {
    var axErr *ax.Error
    if errors.As(err, &axErr) {
        switch axErr.ErrorCode {
        case "config_too_large":
            // shrink the config or raise the limit with WithMaxConfigBytes
        case "config_max_bytes_invalid":
            // the cap was out of range; set a limit between 0 and
            // ax.MaxConfigBytesCeiling
        case "config_invalid":
            // the config is not valid Hujson or does not match the schema;
            // the parser's error is preserved as the cause (errors.Is/As
            // through Unwrap)
        case "config_option_invalid":
            // a nil ParseConfigOption was passed; remove it
        }
        // axErr.ExitCode() == 2 for all four (validation)
    }
    // Otherwise it's an unclassified failure (missing file, broken stream,
    // or a canceled/expired ctx) — its error chain is preserved; inspect with
    // errors.Is/As. A canceled or timed-out read surfaces context.Canceled /
    // context.DeadlineExceeded; at the CLI boundary ax.ErrorExitCode maps
    // DeadlineExceeded → 3 (timeout) and Canceled → 1.
}
```

Both `error_code` values are **frozen public contract** — branch on them
directly, never parse the human-facing message. `axErr.Context["max_bytes"]`
carries the active cap as informational data (not part of the frozen contract).

## Boundary cheat-sheet

| Input vs. cap | Result |
|---------------|--------|
| smaller than cap | parsed |
| **exactly** the cap | parsed (inclusive) |
| one byte over the cap | `config_too_large`, exit `2` |
| any non-empty input, cap `0` | `config_too_large`, exit `2` |
| empty input, cap `0` | passes size check (then parsed — empty is not valid Hujson → `config_invalid`, exit `2`, parser error preserved as cause) |
| syntactically invalid Hujson / schema mismatch | `config_invalid`, exit `2` (underlying error preserved as cause via `Unwrap`) |
| nil `ParseConfigOption` | `config_option_invalid`, exit `2` |
| negative cap | `config_max_bytes_invalid`, exit `2` |
| cap above `MaxConfigBytesCeiling` (1 GiB) | `config_max_bytes_invalid`, exit `2` |

## Verify locally

```bash
go test -race ./...        # correctness incl. tripwire/counting reader + boundary/edge tests
make bench                 # -benchmem: bytes read & allocs stay bounded as input grows (SC-001)
make doc-coverage          # ExampleParseConfig / ExampleParseConfigFile stay gated and green
make lint                  # golangci-lint (incl. godoclint require-doc) + markdownlint
```
