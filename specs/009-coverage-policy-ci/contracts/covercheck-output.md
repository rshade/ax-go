# Contract: covercheck Output and Exit Codes

**Tool**: `go run ./internal/cmd/covercheck`
**Contract version**: 1.0 (pre-v1.0 module; governed by Constitution Principle XI)

---

## Exit Codes

| Code | Meaning | When |
|------|---------|------|
| `0` | All floors met | Per-package and repo-wide checks pass |
| `1` | Floor violation | One or more packages below their floor, OR aggregate below repo-wide floor |
| `2` | Bad input | Coverage file not found, unreadable, or malformed |

---

## Standard Output (exit 0 — pass)

Written to **stdout**. One line per checked package plus a repo-wide summary.

```text
PASS  coverage floors met
  repo-wide:                                          70.8% >= 70.0% floor
  github.com/rshade/ax-go:                           83.7% >= 80.0% floor
  github.com/rshade/ax-go/examples/integration:      87.5% >= 85.0% floor
  github.com/rshade/ax-go/internal/cmd/doccover:     49.6% >= 45.0% floor
  github.com/rshade/ax-go/internal/config:           69.6% >= 65.0% floor
  github.com/rshade/ax-go/internal/telemetry:        65.8% >= 60.0% floor
  github.com/rshade/ax-go/internal/testutil:         29.4% >= 25.0% floor
excluded (per-package floor not enforced):
  github.com/rshade/ax-go/internal/cli               0.0%
  github.com/rshade/ax-go/internal/mcp               0.0%
  github.com/rshade/ax-go/internal/schema            0.0%
```

---

## Standard Error (exit 1 — violation)

Written to **stderr**. The first line names the failure count; subsequent lines
identify each violating package with actual%, floor%, and the shortfall (delta).

```text
FAIL  2 coverage floor violation(s):
  github.com/rshade/ax-go/internal/config:     55.0% < 65.0% floor  (-10.0pp)
  repo-wide:                                    68.0% < 70.0% floor  (-2.0pp)
```

---

## Standard Error (exit 2 — bad input)

Written to **stderr**.

```text
covercheck: cannot open coverage file "coverage.out": open coverage.out: no such file or directory
```

---

## Guarantees

- **Determinism**: Given the same `coverage.out` file, the tool produces
  byte-identical output on every run. No timestamps, random values, or
  environment-dependent formatting. This format is guarded by **golden-file
  tests** (`internal/cmd/covercheck/testdata/`) so any drift fails CI loudly
  (Constitution Principle VII).
- **Stream discipline**: Violations go to stderr; pass summary goes to stdout.
  Nothing else is written to stdout (Constitution Principle I).
- **No side effects**: The tool reads one file and exits; it does not modify
  any file or make network calls.
- **Additive tolerance**: Future versions may add lines to the output (e.g.,
  additional per-package detail) without bumping the contract version. Consumers
  MUST tolerate unknown lines.

---

## Invocation

```bash
# Minimal (reads coverage.out in current directory)
go run ./internal/cmd/covercheck -coverage coverage.out

# Via Makefile (runs tests + checks in one step)
make cover-check
```

---

## Stability

This contract is internal (`internal/cmd/covercheck`) and governed by
Constitution Principle XI. The exit-code contract (0/1/2) and the
human-readable output format are considered stable-by-intent once the feature
ships; format changes that would break CI scripts require a note in the commit
message.
