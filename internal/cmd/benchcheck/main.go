// Command benchcheck enforces a performance regression budget by comparing
// two `go test -bench=. -benchmem` output files (a baseline run and a current
// run) using golang.org/x/perf/benchstat's statistical comparison.
//
// It exists so a performance regression fails CI loudly and is reproducible
// locally via `make bench-check`. Budget thresholds are hardcoded constants
// in this file so every change is a reviewable Go-code commit auditable
// through git blame, rather than hidden in external configuration.
//
// Stream discipline (Constitution Principle I): the pass summary is written
// to stdout; violation messages are written to stderr. Exit codes
// (Constitution-style determinism): 0 = within budget, 1 = one or more
// budget violations, 2 = bad input (baseline or current file missing,
// unreadable, or malformed).
//
// benchcheck intentionally omits context.Context: it is a sub-second tool
// that reads two bounded files and exits, with no cancellation, network, or
// goroutine surface to govern (same recorded deviation as covercheck).
//
// Run from the module root:
//
//	go run ./internal/cmd/benchcheck -baseline <base-file> -current <current-file>
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"golang.org/x/perf/benchstat" //nolint:staticcheck // SA1019: benchstat is deprecated upstream but still the correct tool here; see research.md Decision 1
)

const (
	// maxNsOpRegressionPercent is the maximum tolerated increase in ns/op,
	// counted only when benchstat marks the delta statistically significant
	// (Row.Change == -1). See research.md Decision 2.
	maxNsOpRegressionPercent = 5.0
	// maxAllocsOpIncrease is the maximum tolerated absolute increase in mean
	// allocs/op. An absolute (not percentage) threshold avoids the
	// undefined-percentage case when the baseline is 0 allocs/op, which is
	// the common case in this codebase. See research.md Decision 2.
	maxAllocsOpIncrease = 1.0
	// significanceAlpha is the p-value cutoff benchstat uses to decide a
	// delta is statistically significant, matching benchstat's own default.
	significanceAlpha = 0.05
	// maxBenchOutputBytes bounds each input file read so a pathological or
	// truncated input is a deterministic validation error (exit 2) rather
	// than an unbounded read. A `-count=10` run across this repo's tracked
	// benchmarks is well under the 4 MiB limit.
	maxBenchOutputBytes = 4 << 20
)

const (
	// exitOK signals that every tracked benchmark is within budget.
	exitOK = 0
	// exitViolation signals one or more performance budget violations.
	exitViolation = 1
	// exitBadInput signals a missing, unreadable, or malformed input file.
	exitBadInput = 2
)

const (
	// metricTimeOp is the benchstat table metric name for operation time.
	metricTimeOp = "time/op"
	// metricAllocsOp is the benchstat table metric name for allocations per
	// operation, and also the Violation.Metric value reported for it.
	metricAllocsOp = "allocs/op"
	// metricNsOp is the Violation.Metric value reported for a time/op
	// regression (distinct from benchstat's own "time/op" table name).
	metricNsOp = "ns/op"
	// configBaseline and configCurrent name the two benchstat.Collection
	// configs; their AddFile call order fixes Row.Metrics[0] as baseline and
	// Row.Metrics[1] as current.
	configBaseline = "baseline"
	configCurrent  = "current"
)

// Violation is a single performance-budget failure, named and quantified for
// the stderr message (spec FR-004).
type Violation struct {
	// Benchmark is the offending benchmark's name (without the "Benchmark"
	// prefix benchstat strips).
	Benchmark string
	// Metric is the metric that exceeded budget: "ns/op" or "allocs/op".
	Metric string
	// Delta is the measured regression: a percentage for "ns/op", an
	// absolute increase for "allocs/op".
	Delta float64
	// Budget is the threshold Delta exceeded.
	Budget float64
}

// CheckResult is the complete outcome of one benchcheck run.
type CheckResult struct {
	// Benchmarks lists every benchmark present in both the baseline and
	// current inputs, sorted by name.
	Benchmarks []string
	// Violations lists every budget failure, sorted by benchmark then
	// metric; empty when Pass() is true.
	Violations []Violation
	// Missing lists benchmarks present in the baseline but absent from the
	// current run, sorted by name — the signal that a previously-tracked
	// benchmark failed to build or run (e.g. a compile error or panic in
	// that package), rather than merely regressing. benchstat's Collection
	// silently omits any row missing from either side of the comparison
	// (see Table in golang.org/x/perf/benchstat), so this must be detected
	// separately or a broken benchmark would vanish without a trace; empty
	// when Pass() is true.
	Missing []string
}

// Pass reports whether the run met every tracked benchmark's budget: no
// budget violations and no previously-tracked benchmark missing from the
// current run. Derived from Violations and Missing rather than stored, so
// the two can never silently disagree.
func (r CheckResult) Pass() bool {
	return len(r.Violations) == 0 && len(r.Missing) == 0
}

func main() {
	baselinePath := flag.String("baseline", "", "path to the baseline benchmark run")
	currentPath := flag.String("current", "", "path to the fresh benchmark run to check (required)")
	flag.Parse()
	os.Exit(run(*baselinePath, *currentPath, os.Stdout, os.Stderr))
}

// oversizedInputError reports that a baseline or current input exceeded
// maxBenchOutputBytes. It is a distinct type (rather than a plain
// fmt.Errorf string) so run() can tell an oversized-but-opened file apart
// from a genuine os.Open failure via errors.As, and report an accurate
// message for each rather than conflating them under one "cannot open"
// prefix.
type oversizedInputError struct {
	limit int64
}

func (e *oversizedInputError) Error() string {
	return fmt.Sprintf("exceeds the %d-byte size limit", e.limit)
}

// run compares the baseline and current benchmark output files, writes the
// pass summary to stdout or the violation list to stderr, and returns the
// process exit code (0 pass, 1 violation, 2 bad input).
func run(baselinePath, currentPath string, stdout, stderr io.Writer) int {
	if baselinePath == "" {
		fmt.Fprintln(stderr, "benchcheck: -baseline is required")
		return exitBadInput
	}
	if currentPath == "" {
		fmt.Fprintln(stderr, "benchcheck: -current is required")
		return exitBadInput
	}

	coll := &benchstat.Collection{Alpha: significanceAlpha, DeltaTest: benchstat.UTest}

	if _, err := addBoundedConfig(coll, configBaseline, baselinePath); err != nil {
		fmt.Fprintf(stderr, "benchcheck: %s\n", describeConfigError("baseline", baselinePath, err))
		return exitBadInput
	}
	currentSummary, err := addBoundedConfig(coll, configCurrent, currentPath)
	if err != nil {
		fmt.Fprintf(stderr, "benchcheck: %s\n", describeConfigError("current", currentPath, err))
		return exitBadInput
	}
	if !configHasBenchmarkMetrics(coll, configBaseline) {
		fmt.Fprintln(stderr, "benchcheck: no benchmark results in baseline; "+
			"check the file is valid `go test -bench` output")
		return exitBadInput
	}
	if !configHasBenchmarkMetrics(coll, configCurrent) && !currentSummary.looksLikeGoTestOutput {
		fmt.Fprintln(stderr, "benchcheck: no benchmark results in current; "+
			"check the file is valid `go test -bench` output")
		return exitBadInput
	}

	result, err := checkBudget(coll)
	if err != nil {
		fmt.Fprintf(stderr, "benchcheck: %v\n", err)
		return exitBadInput
	}

	if result.Pass() {
		formatPass(stdout, result)
		return exitOK
	}
	formatFail(stderr, result)
	return exitViolation
}

// describeConfigError renders addBoundedConfig's error for the named input
// (label is "baseline" or "current"), distinguishing an oversized-input
// rejection from a genuine open/read failure so the message points at the
// actual problem.
func describeConfigError(label, path string, err error) string {
	if _, ok := errors.AsType[*oversizedInputError](err); ok {
		return fmt.Sprintf("%s file %q %v", label, path, err)
	}
	return fmt.Sprintf("cannot open %s file %q: %v", label, path, err)
}

type benchInputSummary struct {
	looksLikeGoTestOutput bool
}

// addBoundedConfig reads path and adds its contents to coll under config,
// bounding the read at maxBenchOutputBytes so an oversized file is a
// deterministic error rather than unbounded memory growth (Constitution
// Principle IX).
func addBoundedConfig(coll *benchstat.Collection, config, path string) (benchInputSummary, error) {
	file, openErr := os.Open(path)
	if openErr != nil {
		return benchInputSummary{}, openErr
	}
	defer func() { _ = file.Close() }()

	limited := &io.LimitedReader{R: file, N: maxBenchOutputBytes + 1}
	data, readErr := io.ReadAll(limited)
	if readErr != nil {
		return benchInputSummary{}, readErr
	}
	if limited.N <= 0 {
		return benchInputSummary{}, &oversizedInputError{limit: maxBenchOutputBytes}
	}

	summary := benchInputSummary{looksLikeGoTestOutput: looksLikeGoTestOutput(data)}
	if addErr := coll.AddFile(config, bytes.NewReader(data)); addErr != nil {
		return benchInputSummary{}, addErr
	}
	return summary, nil
}

func looksLikeGoTestOutput(data []byte) bool {
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		switch {
		case bytes.HasPrefix(line, []byte("goos:")):
			return true
		case bytes.HasPrefix(line, []byte("goarch:")):
			return true
		case bytes.HasPrefix(line, []byte("pkg:")):
			return true
		case bytes.HasPrefix(line, []byte("cpu:")):
			return true
		}

		fields := bytes.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch {
		case bytes.Equal(fields[0], []byte("ok")):
			return true
		case bytes.Equal(fields[0], []byte("?")):
			return true
		case bytes.Equal(fields[0], []byte("FAIL")):
			return true
		}
	}
	return false
}

func configHasBenchmarkMetrics(coll *benchstat.Collection, config string) bool {
	for key := range coll.Metrics {
		if key.Config == config {
			return true
		}
	}
	return false
}

// checkBudget evaluates the benchstat comparison tables against the
// regression budget and cross-checks for benchmarks that dropped out of the
// current run entirely. It returns an error only when there is neither a
// comparable benchmark nor a missing-benchmark signal for the caller to
// report.
func checkBudget(coll *benchstat.Collection) (CheckResult, error) {
	tables := coll.Tables()
	benchmarkSet := map[string]bool{}
	var violations []Violation

	for _, table := range tables {
		for _, row := range table.Rows {
			benchmarkSet[row.Benchmark] = true
			if v, ok := rowViolation(table.Metric, row); ok {
				violations = append(violations, v)
			}
		}
	}

	missing := missingBenchmarks(coll)
	if len(benchmarkSet) == 0 {
		if len(missing) > 0 {
			return CheckResult{Missing: missing}, nil
		}
		return CheckResult{}, errors.New("no comparable benchmark results between baseline and current; " +
			"check both files are valid `go test -bench` output")
	}

	benchmarks := make([]string, 0, len(benchmarkSet))
	for b := range benchmarkSet {
		benchmarks = append(benchmarks, b)
	}
	sort.Strings(benchmarks)
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Benchmark != violations[j].Benchmark {
			return violations[i].Benchmark < violations[j].Benchmark
		}
		return violations[i].Metric < violations[j].Metric
	})

	return CheckResult{
		Benchmarks: benchmarks,
		Violations: violations,
		Missing:    missing,
	}, nil
}

// missingBenchmarks returns the benchmarks present in the baseline config
// but absent from the current config, sorted by name. benchstat's Table
// silently omits any row missing from either side of a two-config
// comparison (see golang.org/x/perf/benchstat's addTable), which is exactly
// right for a *new* benchmark not yet baselined (spec.md edge case) but
// would otherwise let a previously-tracked benchmark that fails to build or
// panics in the current run disappear from checkBudget without a trace.
func missingBenchmarks(coll *benchstat.Collection) []string {
	benchmarksByConfig := map[string]map[string]bool{}
	for key := range coll.Metrics {
		if benchmarksByConfig[key.Config] == nil {
			benchmarksByConfig[key.Config] = map[string]bool{}
		}
		benchmarksByConfig[key.Config][key.Benchmark] = true
	}

	inCurrent := benchmarksByConfig[configCurrent]
	var missing []string
	for b := range benchmarksByConfig[configBaseline] {
		if !inCurrent[b] {
			missing = append(missing, b)
		}
	}
	sort.Strings(missing)
	return missing
}

// rowViolation evaluates a single benchstat row against the regression
// budget for its metric. Only statistically significant regressions
// (row.Change == -1) are ever considered, satisfying the noise-tolerance
// requirement (spec FR-003) for both metrics.
func rowViolation(metric string, row *benchstat.Row) (Violation, bool) {
	if row.Change != -1 {
		return Violation{}, false
	}
	switch metric {
	case metricTimeOp:
		if row.PctDelta > maxNsOpRegressionPercent {
			return Violation{
				Benchmark: row.Benchmark,
				Metric:    metricNsOp,
				Delta:     row.PctDelta,
				Budget:    maxNsOpRegressionPercent,
			}, true
		}
	case metricAllocsOp:
		if len(row.Metrics) != 2 { //nolint:mnd // benchstat pairs exactly (baseline, current)
			return Violation{}, false
		}
		delta := row.Metrics[1].Mean - row.Metrics[0].Mean
		if delta > maxAllocsOpIncrease {
			return Violation{
				Benchmark: row.Benchmark,
				Metric:    metricAllocsOp,
				Delta:     delta,
				Budget:    maxAllocsOpIncrease,
			}, true
		}
	}
	return Violation{}, false
}

// formatPass writes the deterministic pass summary to w: a summary line
// followed by one line per checked benchmark.
func formatPass(w io.Writer, r CheckResult) {
	width := 0
	for _, b := range r.Benchmarks {
		if n := len("Benchmark" + b + ":"); n > width {
			width = n
		}
	}

	fmt.Fprintf(w, "PASS  performance budget met (%d benchmarks checked)\n", len(r.Benchmarks))
	for _, b := range r.Benchmarks {
		fmt.Fprintf(w, "  %-*s  within budget\n", width, "Benchmark"+b+":")
	}
}

// formatFail writes the deterministic failure list to w: a count line
// followed by one line per budget violation naming the benchmark, the
// exceeded metric, the measured delta, and the budget (spec FR-004/SC-003),
// then one line per benchmark missing from the current run entirely.
func formatFail(w io.Writer, r CheckResult) {
	width := 0
	for _, v := range r.Violations {
		if n := len("Benchmark" + v.Benchmark + ":"); n > width {
			width = n
		}
	}
	for _, b := range r.Missing {
		if n := len("Benchmark" + b + ":"); n > width {
			width = n
		}
	}

	fmt.Fprintf(w, "FAIL  %d performance budget failure(s):\n", len(r.Violations)+len(r.Missing))
	for _, v := range r.Violations {
		fmt.Fprintf(w, "  %-*s  %s  %s\n", width, "Benchmark"+v.Benchmark+":", v.Metric, formatDeltaVsBudget(v))
	}
	for _, b := range r.Missing {
		fmt.Fprintf(w, "  %-*s  missing from current run (tracked in baseline; check for a build failure or panic)\n",
			width, "Benchmark"+b+":")
	}
}

// formatDeltaVsBudget renders a violation's measured delta against its
// budget in the metric-appropriate unit: a percentage for "ns/op", a plain
// count for "allocs/op" (an absolute increase, not a percentage — see
// research.md Decision 2).
func formatDeltaVsBudget(v Violation) string {
	if v.Metric == metricAllocsOp {
		return fmt.Sprintf("+%.0f > %.0f budget", v.Delta, v.Budget)
	}
	return fmt.Sprintf("+%.1f%% > %.1f%% budget", v.Delta, v.Budget)
}
