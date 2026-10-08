# Contract: `slopcheck` gate CLI

Invocation, from the module root:

```bash
go run ./internal/cmd/slopcheck          # what make slop-check runs
go run ./internal/cmd/slopcheck -dir PATH
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-dir` | `.` | Module root to analyze (`./...` beneath it) |

Positional arguments are rejected.

## Streams

| Outcome | stdout | stderr | Exit |
| --- | --- | --- | --- |
| Pass | one minified JSON object, newline-terminated | empty | `0` |
| Any failure | empty | one minified `ax.Error` envelope, newline-terminated | per table below |

## Pass document

```json
{"status":"pass","configurations":4,"packages":57,"fields":312,"unread":0}
```

The counts depend only on the analyzed tree, so two runs on the same tree are
byte-identical.

## Failure envelopes

Every envelope is built with `contract.NewError` and sets `tool:"slopcheck"`,
`version` from build info, `retryable:false`, the exit code, and
`context.cause`.

| `error_code` | Exit | When | `context.cause` | `suggestions` |
| --- | --- | --- | --- | --- |
| `slopcheck_unread_field` | `2` | ≥ 1 finding after intersection | sorted findings joined by a semicolon and a space | read the field where it matters, or delete it and its assignments; review before deleting |
| `invalid_slopcheck_artifact` | `2` | bad flag, positional argument, `-dir` not a module root | the reason | — |
| `slopcheck_analysis_failed` | `2` | `go list` failure, type-check error, missing export data, oversized output | configuration name + first error | — |
| `slopcheck_timeout` | `3` | a configuration exceeded five minutes | configuration name | — |
| `slopcheck_permission` | `4` | `fs.ErrPermission` reading sources or running `go` | path | — |
| `slopcheck_internal` | `1` | anything else, including cancellation | the error | — |

Classification order when one error matches several rows: timeout
(`context.DeadlineExceeded`) first, then cancellation (`context.Canceled`, which
maps to internal), then permission (`fs.ErrPermission`), then
`*AnalysisError`, then internal. A timeout, cancellation or permission failure
wrapped inside an `*AnalysisError` is therefore still exit `3`, `1` or `4`.

Finding entry format:

```text
internal/x/a_test.go:14:3: github.com/rshade/ax-go/internal/x.struct{...}@a_test.go:12:10.expectError (assigned at internal/x/a_test.go:19:44, internal/x/a_test.go:20:44)
```

## Golden files

`internal/cmd/slopcheck/testdata/`:

- `pass.stdout.golden`, plus an asserted-empty stderr
- `fail.stderr.golden`, plus an asserted-empty stdout
- `analysis.stderr.golden`, plus an asserted-empty stdout
