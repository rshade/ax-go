# Quickstart: Performance Regression Budget in CI

**Feature**: `014-bench-regression-budget` | **Date**: 2026-07-08

How to run the regression check locally and read a failure. This is the
verification path for spec SC-001…SC-006.

## Run the exact check CI runs

```bash
make bench-check
```

This runs a fresh `go test -run='^$' -bench=. -cpu=1 -count=10 -benchmem ./...`
pass for the current worktree, captures the same benchmark set from
`BENCH_BASE_REF` in a temporary git worktree, and compares the two files via
`internal/cmd/benchcheck`. Both runs use `-cpu=1`, so benchmark names are
stable across machines.

CI sets `BENCH_BASE_REF` to the pull request base SHA. Locally, it defaults
to `git merge-base HEAD origin/main`, then `HEAD~1` if `origin/main` is not
available; override it when you need a different comparison point:

```bash
BENCH_BASE_REF=HEAD~1 make bench-check
```

Or step by step:

```bash
BENCH_BASE_REF="${BENCH_BASE_REF:-$(git merge-base HEAD origin/main 2>/dev/null || git rev-parse --verify --quiet HEAD~1)}"
git worktree add --detach /tmp/ax-go-bench-base "$BENCH_BASE_REF"
(cd /tmp/ax-go-bench-base && go test -run='^$' -bench=. -cpu=1 -count=10 -benchmem ./... > /tmp/bench-base.txt)
go test -run='^$' -bench=. -cpu=1 -count=10 -benchmem ./... > /tmp/bench-current.txt
go run ./internal/cmd/benchcheck -baseline /tmp/bench-base.txt -current /tmp/bench-current.txt
git worktree remove --force /tmp/ax-go-bench-base
```

## Reading a pass

```text
PASS  performance budget met (8 benchmarks checked)
  BenchmarkLoggerEmit/enabled/no_fields:               within budget
  ...
```

## Reading a failure

```text
FAIL  1 performance budget violation(s):
  BenchmarkWriteError:       ns/op  +12.4% > 5.0% budget
```

The line names the benchmark, the metric that regressed (`ns/op` or
`allocs/op`), the measured delta, and the budget it exceeded (SC-003) — no
need to re-run benchmarks locally to know what broke.

## Deliberately accepting a performance trade-off

There is no committed benchmark baseline to update. A reviewed,
intentional performance trade-off either stays within the existing budget or
changes the budget constants in `internal/cmd/benchcheck/main.go` with the
rationale in the commit message. After the trade-off lands on the base
branch, future PRs compare against that new branch state on their own
runner.

## Acceptance verification map

| Spec criterion | How this quickstart verifies it |
|-----------------|----------------------------------|
| SC-001/SC-002 (regression blocks merge) | `make bench-check` exits non-zero on a real ns/op or allocs/op regression |
| SC-003 (failure identifies the benchmark) | the stderr line names the benchmark and the exceeded metric, no local re-run needed |
| SC-004 (documented in under 60s) | see `AGENTS.md`'s "Performance Regression Budget" section |
| SC-005 (noise doesn't fail CI) | re-run `make bench-check` against unchanged code a few times; it stays green |
| SC-006 (accepted trade-off path) | budget changes are explicit Go-code changes, not hidden benchmark-output regeneration |

## Full local gate (before handing work back)

```bash
gofmt -s -l .            # must print nothing
go vet ./...
golangci-lint run
go test -race ./...      # includes internal/cmd/benchcheck's own tests
make bench-check         # the regression gate itself
make cover-check          # internal/cmd/benchcheck needs its own calibrated floor
make doc-coverage
markdownlint-cli2 "specs/014-bench-regression-budget/**/*.md"
```
