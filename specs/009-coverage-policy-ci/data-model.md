# Data Model: Coverage Policy and CI Enforcement

**Feature**: `009-coverage-policy-ci`

This feature introduces no database schema and no persistent data structures
visible to consumers. All entities below are internal to the `covercheck` tool
and its test suite.

---

## Entities

### CoverageProfile

The raw output of `go test -coverprofile=coverage.out -covermode=atomic ./...`.
Read from disk by `covercheck`.

```
mode: atomic
<import-path>/<file>.go:<start-line>.<start-col>,<end-line>.<end-col> <numStmt> <count>
...
```

Fields:
- `import-path` — Go import path of the package (e.g. `github.com/rshade/ax-go`)
- `file` — relative filename within the package
- `numStmt` — number of statements in the block
- `count` — execution count (0 = not covered, ≥1 = covered)

### PackageCoverage

Aggregated coverage for one package, computed by `covercheck` from the profile.

```go
type PackageCoverage struct {
    ImportPath string
    Stmts      int     // total statements
    Covered    int     // covered statements
    Pct        float64 // Covered/Stmts * 100
}
```

### FloorConfig

The embedded policy in `internal/cmd/covercheck/main.go`.

```go
const repoWideFloor = 70.0 // initial; aspirational: 85.0

var packageFloors = map[string]float64{
    "github.com/rshade/ax-go":                         80.0,
    "github.com/rshade/ax-go/examples/integration":    85.0,
    "github.com/rshade/ax-go/internal/cmd/doccover":   45.0,
    "github.com/rshade/ax-go/internal/config":         65.0,
    "github.com/rshade/ax-go/internal/telemetry":      60.0,
    "github.com/rshade/ax-go/internal/testutil":       25.0,
}

const defaultPackageFloor = 25.0 // applies to packages not in the map

var excludedPackages = []string{
    "github.com/rshade/ax-go/internal/cli",
    "github.com/rshade/ax-go/internal/mcp",
    "github.com/rshade/ax-go/internal/schema",
}
```

### Violation

A single coverage floor failure reported by `covercheck`.

```go
type Violation struct {
    Scope     string  // import path (per-package) or "repo-wide" (aggregate)
    Actual    float64 // measured coverage %
    Floor     float64 // required minimum %
    Delta     float64 // Actual - Floor (always negative for a violation)
}
```

### CheckResult

The complete output of one `covercheck` run.

```go
type CheckResult struct {
    Pass       bool
    RepoWide   PackageCoverage // aggregate across all packages
    Packages   []PackageCoverage
    Violations []Violation
}
```

---

## State Transitions

`covercheck` is stateless. There are no persistent state transitions.

The policy state is versioned by git commit (floor changes are code changes).

---

## Entity Relationships

```text
CoverageProfile (file on disk)
    │
    │ parsed by covercheck
    ▼
PackageCoverage[] (one per package in profile)
    │
    │ compared against
    ▼
FloorConfig (embedded in covercheck binary)
    │
    │ produces
    ▼
CheckResult ──► Violation[] (empty on pass)
```
