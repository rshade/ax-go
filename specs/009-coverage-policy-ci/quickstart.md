# Quickstart: Coverage Policy Local Workflow

**Feature**: `009-coverage-policy-ci`

This guide lets you run the exact same coverage check that CI runs, before
pushing a PR.

---

## Prerequisites

- Go 1.26.4 installed
- Working directory: repository root

---

## One-Step Check (recommended)

```bash
make cover-check
```

This:
1. Runs `go test -race -coverprofile=coverage.out -covermode=atomic ./...`
2. Runs `go run ./internal/cmd/covercheck -coverage coverage.out`
3. Prints a pass summary (stdout) or violation list (stderr) + exits 1 on failure

---

## Step-by-Step

```bash
# Step 1: Run tests with coverage (produces coverage.out)
go test -race -coverprofile=coverage.out -covermode=atomic ./...

# Step 2: (Optional) Human-readable function-level detail
go tool cover -func=coverage.out

# Step 3: Enforce floors (this is what CI runs)
go run ./internal/cmd/covercheck -coverage coverage.out
```

---

## Interpreting Output

### Pass

```
PASS  coverage floors met
  repo-wide:  83.7% >= 70.0% floor
  github.com/rshade/ax-go:  83.7% >= 80.0% floor
  ...
```

→ Your PR will not fail the coverage gate.

### Failure

```
FAIL  1 coverage floor violation(s):
  github.com/rshade/ax-go/internal/config:  55.0% < 65.0% floor  (-10.0pp)
```

→ Add tests to `internal/config` until `go run ./internal/cmd/covercheck` passes.

---

## Adding a New Package

If you create a new package, it will be checked against the `defaultPackageFloor`
(25%). To avoid an immediate CI failure:
- Either add tests that cover ≥ 25% of statements.
- Or (with justification) add the package to the `excludedPackages` list in
  `internal/cmd/covercheck/main.go` and document it in `AGENTS.md`.

---

## Raising a Floor

```bash
# 1. Edit the floor value
$EDITOR internal/cmd/covercheck/main.go   # update packageFloors map

# 2. Verify it passes with the current coverage
make cover-check

# 3. Commit with a clear message
git commit -m "chore: raise internal/config coverage floor to 70%"
```

---

## Current Floor Values

See `AGENTS.md` → Coverage Policy section for the authoritative table.
See `internal/cmd/covercheck/main.go` for the exact values used at runtime.
