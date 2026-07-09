// Command covercheck enforces per-package and repo-wide test-coverage floors
// against a Go coverage profile (the output of
// `go test -coverprofile=coverage.out -covermode=atomic ./...`).
//
// It exists so a coverage regression fails CI loudly and is reproducible
// locally via `make cover-check`. Floors are hardcoded constants in this file
// (see defaultFloorConfig) so every floor change is a reviewable Go-code commit
// auditable through git blame, rather than hidden in external configuration.
//
// Stream discipline (Constitution Principle I): the pass summary is written to
// stdout; violation messages are written to stderr. Exit codes
// (Constitution-style determinism): 0 = all floors met, 1 = one or more floor
// violations, 2 = bad input (coverage file missing, unreadable, or malformed).
//
// covercheck intentionally omits context.Context: it is a sub-second tool that
// reads one bounded file and exits, with no cancellation, network, or goroutine
// surface to govern (recorded deviation from Constitution Principle X).
//
// Run from the module root:
//
//	go run ./internal/cmd/covercheck -coverage coverage.out
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	// repoWideFloor is the minimum acceptable aggregate coverage percentage
	// across all packages. Set to 70.0 against the 2026-06-16 measured baseline
	// of 70.8% (a 0.8pp safety buffer); aspirational target 85%.
	repoWideFloor = 70.0
	// defaultPackageFloor applies to any package not listed in the per-package
	// floor map, so a newly added package faces a non-trivial gate by default.
	defaultPackageFloor = 25.0
	// percentScale converts a covered/total ratio to a percentage.
	percentScale = 100.0
	// profileFieldCount is the number of whitespace-separated fields in a Go
	// coverage profile data line: "file:block numStmt count".
	profileFieldCount = 3
	// repoWideScope is the sentinel import path used for the aggregate floor.
	repoWideScope = "repo-wide"
	// maxProfileBytes bounds the coverage profile read so a pathological or
	// truncated input is a deterministic validation error (exit 2) rather than
	// an unbounded read. A real `go test ./...` profile is well under 1 MiB.
	maxProfileBytes = 1 << 20
)

const (
	// exitOK signals that every floor was met.
	exitOK = 0
	// exitViolation signals one or more coverage floor violations.
	exitViolation = 1
	// exitBadInput signals a missing, unreadable, or malformed coverage file.
	exitBadInput = 2
)

// PackageCoverage is the aggregated coverage of one package, computed from the
// coverage profile.
type PackageCoverage struct {
	// ImportPath is the Go import path of the package.
	ImportPath string
	// Stmts is the total number of statements in the package.
	Stmts int
	// Covered is the number of executed (covered) statements.
	Covered int
	// Pct is the coverage percentage (Covered/Stmts * 100); 0 when Stmts is 0.
	Pct float64
}

// Violation is a single coverage floor failure.
type Violation struct {
	// Scope is the offending package import path, or "repo-wide" for the
	// aggregate floor.
	Scope string
	// Actual is the measured coverage percentage.
	Actual float64
	// Floor is the required minimum coverage percentage.
	Floor float64
	// Delta is Actual-Floor, always negative for a violation.
	Delta float64
}

// CheckResult is the complete outcome of one covercheck run.
type CheckResult struct {
	// Pass is true when there are no violations.
	Pass bool
	// RepoWide is the aggregate coverage across all packages (its ImportPath is
	// the sentinel "repo-wide").
	RepoWide PackageCoverage
	// Packages holds the non-excluded packages that were checked, sorted by
	// import path.
	Packages []PackageCoverage
	// Excluded holds packages exempt from the per-package floor gate, sorted by
	// import path; they still count toward the repo-wide aggregate.
	Excluded []PackageCoverage
	// Violations lists every floor failure; empty when Pass is true.
	Violations []Violation
}

// floorConfig is the coverage policy applied by a single covercheck run. It is
// a value (not package-level state) so tests can supply synthetic policies and
// the production policy lives in defaultFloorConfig.
type floorConfig struct {
	repoWide     float64
	defaultFloor float64
	perPackage   map[string]float64
	excluded     map[string]bool
}

// floorFor returns the per-package floor for importPath, falling back to the
// default floor when the package has no explicit override.
func (c floorConfig) floorFor(importPath string) float64 {
	if f, ok := c.perPackage[importPath]; ok {
		return f
	}
	return c.defaultFloor
}

// defaultFloorConfig returns the production coverage policy, calibrated to the
// 2026-06-16 baseline. See AGENTS.md "Coverage Policy" for the authoritative
// table and the escalation plan. The excluded packages have 0% baseline
// coverage and are pending tests; they still count toward the repo-wide floor.
//
//nolint:mnd // per-package floor percentages are policy data documented in AGENTS.md, not magic numbers
func defaultFloorConfig() floorConfig {
	return floorConfig{
		repoWide:     repoWideFloor,
		defaultFloor: defaultPackageFloor,
		perPackage: map[string]float64{
			"github.com/rshade/ax-go":                         80.0,
			"github.com/rshade/ax-go/examples/integration":    85.0,
			"github.com/rshade/ax-go/internal/cli":            100.0,
			"github.com/rshade/ax-go/internal/cmd/benchcheck": 80.0,
			"github.com/rshade/ax-go/internal/cmd/doccover":   45.0,
			"github.com/rshade/ax-go/internal/config":         65.0,
			"github.com/rshade/ax-go/internal/mcp":            90.0,
			"github.com/rshade/ax-go/internal/schema":         95.0,
			"github.com/rshade/ax-go/internal/telemetry":      60.0,
			"github.com/rshade/ax-go/internal/testutil":       25.0,
		},
		excluded: map[string]bool{},
	}
}

func main() {
	coveragePath := flag.String("coverage", "coverage.out", "path to the Go coverage profile to check")
	flag.Parse()
	os.Exit(run(*coveragePath, os.Stdout, os.Stderr))
}

// run reads the coverage profile at coveragePath, enforces the production
// floors, writes the pass summary to stdout or the violation list to stderr,
// and returns the process exit code (0 pass, 1 violation, 2 bad input).
func run(coveragePath string, stdout, stderr io.Writer) int {
	file, err := os.Open(coveragePath)
	if err != nil {
		fmt.Fprintf(stderr, "covercheck: cannot open coverage file %q: %v\n", coveragePath, err)
		return exitBadInput
	}
	defer func() { _ = file.Close() }()

	pkgs, err := parseCoverage(file)
	if err != nil {
		fmt.Fprintf(stderr, "covercheck: cannot parse coverage file %q: %v\n", coveragePath, err)
		return exitBadInput
	}

	cfg := defaultFloorConfig()
	result := checkFloors(pkgs, cfg)
	if result.Pass {
		formatPass(stdout, result, cfg)
		return exitOK
	}
	formatFail(stderr, result)
	return exitViolation
}

// parseCoverage reads a Go coverage profile and aggregates statements and
// covered statements per package import path. The read is capped at
// maxProfileBytes; a larger input, or any malformed data line, returns an error
// (mapped to exit code 2 by the caller).
func parseCoverage(r io.Reader) (map[string]PackageCoverage, error) {
	acc := map[string]PackageCoverage{}
	// Cap at maxProfileBytes+1 so consuming the whole limit (N == 0) proves the
	// input exceeded the cap rather than merely reaching it.
	limited := &io.LimitedReader{R: r, N: maxProfileBytes + 1}
	scanner := bufio.NewScanner(limited)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		importPath, numStmt, count, err := parseProfileLine(line)
		if err != nil {
			if limited.N <= 0 {
				return nil, fmt.Errorf("coverage profile exceeds the %d-byte size limit", maxProfileBytes)
			}
			return nil, err
		}
		pc := acc[importPath]
		pc.ImportPath = importPath
		pc.Stmts += numStmt
		if count > 0 {
			pc.Covered += numStmt
		}
		pc.Pct = percent(pc.Covered, pc.Stmts)
		acc[importPath] = pc
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading coverage profile: %w", err)
	}
	if limited.N <= 0 {
		return nil, fmt.Errorf("coverage profile exceeds the %d-byte size limit", maxProfileBytes)
	}
	return acc, nil
}

// parseProfileLine parses one coverage profile data line of the form
// "import/path/file.go:startLine.col,endLine.col numStmt count" and returns the
// package import path (the file's directory), the statement count, and the
// execution count.
func parseProfileLine(line string) (string, int, int, error) {
	fields := strings.Fields(line)
	if len(fields) != profileFieldCount {
		return "", 0, 0, fmt.Errorf("malformed coverage line %q: want %d fields", line, profileFieldCount)
	}
	block := fields[0]
	colon := strings.LastIndex(block, ":")
	if colon < 0 {
		return "", 0, 0, fmt.Errorf("malformed coverage block %q: missing position separator", block)
	}
	file := block[:colon]
	slash := strings.LastIndex(file, "/")
	if slash < 0 {
		return "", 0, 0, fmt.Errorf("malformed coverage path %q: missing package separator", file)
	}
	numStmt, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("parsing statement count in %q: %w", line, err)
	}
	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return "", 0, 0, fmt.Errorf("parsing execution count in %q: %w", line, err)
	}
	return file[:slash], numStmt, count, nil
}

// percent returns covered/total as a percentage, or 0 when total is 0.
func percent(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(covered) / float64(total) * percentScale
}

// checkFloors partitions pkgs into excluded and checked sets, applies the
// per-package floors to the checked set, computes the repo-wide aggregate over
// all packages, and returns the assembled result with violations sorted package
// failures first and the repo-wide failure (if any) last.
func checkFloors(pkgs map[string]PackageCoverage, cfg floorConfig) CheckResult {
	var checked, excluded []PackageCoverage
	totalStmts, totalCovered := 0, 0
	for _, p := range pkgs {
		totalStmts += p.Stmts
		totalCovered += p.Covered
		if cfg.excluded[p.ImportPath] {
			excluded = append(excluded, p)
			continue
		}
		checked = append(checked, p)
	}
	sort.Slice(checked, func(i, j int) bool { return checked[i].ImportPath < checked[j].ImportPath })
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].ImportPath < excluded[j].ImportPath })

	var violations []Violation
	for _, p := range checked {
		floor := cfg.floorFor(p.ImportPath)
		if p.Pct < floor {
			violations = append(violations, Violation{
				Scope:  p.ImportPath,
				Actual: p.Pct,
				Floor:  floor,
				Delta:  p.Pct - floor,
			})
		}
	}

	repoPct := percent(totalCovered, totalStmts)
	repo := PackageCoverage{ImportPath: repoWideScope, Stmts: totalStmts, Covered: totalCovered, Pct: repoPct}
	if repoPct < cfg.repoWide {
		violations = append(violations, Violation{
			Scope:  repoWideScope,
			Actual: repoPct,
			Floor:  cfg.repoWide,
			Delta:  repoPct - cfg.repoWide,
		})
	}

	return CheckResult{
		Pass:       len(violations) == 0,
		RepoWide:   repo,
		Packages:   checked,
		Excluded:   excluded,
		Violations: violations,
	}
}

// formatPass writes the deterministic pass summary to w: the repo-wide line,
// one line per checked package, then the excluded packages whose per-package
// floor is not enforced.
func formatPass(w io.Writer, r CheckResult, cfg floorConfig) {
	width := len(repoWideScope) + 1 // "repo-wide:"
	for _, p := range r.Packages {
		if n := len(p.ImportPath) + 1; n > width {
			width = n
		}
	}
	for _, p := range r.Excluded {
		if n := len(p.ImportPath); n > width {
			width = n
		}
	}

	fmt.Fprintln(w, "PASS  coverage floors met")
	fmt.Fprintf(w, "  %-*s  %.1f%% >= %.1f%% floor\n", width, repoWideScope+":", r.RepoWide.Pct, cfg.repoWide)
	for _, p := range r.Packages {
		fmt.Fprintf(w, "  %-*s  %.1f%% >= %.1f%% floor\n", width, p.ImportPath+":", p.Pct, cfg.floorFor(p.ImportPath))
	}
	if len(r.Excluded) > 0 {
		fmt.Fprintln(w, "excluded (per-package floor not enforced):")
		for _, p := range r.Excluded {
			fmt.Fprintf(w, "  %-*s  %.1f%%\n", width, p.ImportPath, p.Pct)
		}
	}
}

// formatFail writes the deterministic violation list to w: a count line
// followed by one line per violation with actual%, floor%, and the shortfall.
func formatFail(w io.Writer, r CheckResult) {
	width := 0
	for _, v := range r.Violations {
		if n := len(v.Scope) + 1; n > width {
			width = n
		}
	}

	fmt.Fprintf(w, "FAIL  %d coverage floor violation(s):\n", len(r.Violations))
	for _, v := range r.Violations {
		fmt.Fprintf(w, "  %-*s  %.1f%% < %.1f%% floor  (%.1fpp)\n", width, v.Scope+":", v.Actual, v.Floor, v.Delta)
	}
}
