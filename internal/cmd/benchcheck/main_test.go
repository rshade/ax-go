package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/perf/benchstat"
)

// Synthetic go test -bench=. -benchmem output fixtures. Each benchmark has
// >=5 samples per config: a single-sample fixture never registers as
// statistically significant with benchstat's default U-test (research.md
// Decision 1), so a realistic multi-sample fixture is required to exercise
// both the significant-regression and no-regression paths.

const sharedBaseline = `goos: linux
goarch: amd64
pkg: example
cpu: test
BenchmarkExample-8   	1000000	       100.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	       101.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	        99.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	        99.5 ns/op	       0 B/op	       0 allocs/op
`

// noRegressionBaseline, nsOpRegressionBaseline, and allocsOpRegressionBaseline
// are the same fixture under three names: each test case pairs it with a
// different current-side fixture, but the baseline side is identical across
// all three.
const noRegressionBaseline = sharedBaseline
const noRegressionCurrent = sharedBaseline
const nsOpRegressionBaseline = sharedBaseline
const allocsOpRegressionBaseline = sharedBaseline

const nsOpRegressionCurrent = `goos: linux
goarch: amd64
pkg: example
cpu: test
BenchmarkExample-8   	1000000	       112.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	       113.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	       111.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	       112.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkExample-8   	1000000	       111.5 ns/op	       0 B/op	       0 allocs/op
`

const allocsOpRegressionCurrent = `goos: linux
goarch: amd64
pkg: example
cpu: test
BenchmarkExample-8   	1000000	       100.0 ns/op	      16 B/op	       2 allocs/op
BenchmarkExample-8   	1000000	       101.0 ns/op	      16 B/op	       2 allocs/op
BenchmarkExample-8   	1000000	        99.0 ns/op	      16 B/op	       2 allocs/op
BenchmarkExample-8   	1000000	       100.5 ns/op	      16 B/op	       2 allocs/op
BenchmarkExample-8   	1000000	        99.5 ns/op	      16 B/op	       2 allocs/op
`

const malformedContent = "this is not a go test -bench output file\njust some random text\n"

// missingBenchmarkBaseline tracks two benchmarks; missingBenchmarkCurrent
// only reports one of them, simulating a previously-tracked benchmark whose
// package failed to build or panicked during the current run.
const missingBenchmarkBaseline = `goos: linux
goarch: amd64
pkg: example
cpu: test
BenchmarkAlpha-8   	1000000	       100.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       101.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	        99.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	        99.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       200.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       201.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       199.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       200.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       199.5 ns/op	       0 B/op	       0 allocs/op
`

const missingBenchmarkCurrent = `goos: linux
goarch: amd64
pkg: example
cpu: test
BenchmarkAlpha-8   	1000000	       100.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       101.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	        99.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	        99.5 ns/op	       0 B/op	       0 allocs/op
`

const noBenchmarkRowsCurrent = "?   \texample\t[no test files]\n"

// multiViolationBaseline/Current track two benchmarks that each regress on a
// different metric, so a passing test proves formatFail's alphabetical sort
// (BenchmarkAlpha before BenchmarkBeta) rather than accidental ordering.
const multiViolationBaseline = `goos: linux
goarch: amd64
pkg: example
cpu: test
BenchmarkAlpha-8   	1000000	       100.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       101.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	        99.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	        99.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       100.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       101.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	        99.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	        99.5 ns/op	       0 B/op	       0 allocs/op
`

const multiViolationCurrent = `goos: linux
goarch: amd64
pkg: example
cpu: test
BenchmarkAlpha-8   	1000000	       112.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       113.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       111.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       112.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkAlpha-8   	1000000	       111.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkBeta-8   	1000000	       100.0 ns/op	      16 B/op	       2 allocs/op
BenchmarkBeta-8   	1000000	       101.0 ns/op	      16 B/op	       2 allocs/op
BenchmarkBeta-8   	1000000	        99.0 ns/op	      16 B/op	       2 allocs/op
BenchmarkBeta-8   	1000000	       100.5 ns/op	      16 B/op	       2 allocs/op
BenchmarkBeta-8   	1000000	        99.5 ns/op	      16 B/op	       2 allocs/op
`

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture %s: %v", path, err)
	}
	return path
}

// writeOversizedFixture writes a syntactically valid but oversized (>
// maxBenchOutputBytes) go test -bench output file, to exercise
// addBoundedConfig's size cap.
func writeOversizedFixture(t *testing.T, dir, name string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("goos: linux\ngoarch: amd64\npkg: example\ncpu: test\n")
	const line = "BenchmarkExample-8   \t1000000\t       100.0 ns/op\t       0 B/op\t       0 allocs/op\n"
	for b.Len() <= maxBenchOutputBytes {
		b.WriteString(line)
	}
	return writeFixture(t, dir, name, b.String())
}

func TestRun(t *testing.T) {
	tests := []struct {
		name           string
		baselineFile   func(dir string) string
		currentFile    func(dir string) string
		wantExit       int
		wantStdoutHas  string
		wantStderrHas  []string
		wantStdoutNone bool
		wantStderrNone bool
	}{
		{
			name: "no regression passes",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", noRegressionBaseline)
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", noRegressionCurrent)
			},
			wantExit:       exitOK,
			wantStdoutHas:  "PASS",
			wantStderrNone: true,
		},
		{
			name: "ns/op regression beyond budget fails",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", nsOpRegressionBaseline)
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", nsOpRegressionCurrent)
			},
			wantExit: exitViolation,
			wantStderrHas: []string{
				"Example",
				"ns/op",
			},
			wantStdoutNone: true,
		},
		{
			name: "allocs/op regression beyond budget fails",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", allocsOpRegressionBaseline)
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", allocsOpRegressionCurrent)
			},
			wantExit: exitViolation,
			wantStderrHas: []string{
				"Example",
				"allocs/op",
			},
			wantStdoutNone: true,
		},
		{
			name: "missing baseline file is bad input",
			baselineFile: func(dir string) string {
				return filepath.Join(dir, "does-not-exist.txt")
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", noRegressionCurrent)
			},
			wantExit: exitBadInput,
			wantStderrHas: []string{
				"does-not-exist.txt",
			},
			wantStdoutNone: true,
		},
		{
			name: "malformed current file is bad input",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", noRegressionBaseline)
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", malformedContent)
			},
			wantExit:       exitBadInput,
			wantStdoutNone: true,
		},
		{
			name: "current flag omitted is bad input",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", noRegressionBaseline)
			},
			currentFile: func(dir string) string {
				return ""
			},
			wantExit: exitBadInput,
			wantStderrHas: []string{
				"-current is required",
			},
			wantStdoutNone: true,
		},
		{
			name: "baseline flag omitted is bad input",
			baselineFile: func(dir string) string {
				return ""
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", noRegressionCurrent)
			},
			wantExit: exitBadInput,
			wantStderrHas: []string{
				"-baseline is required",
			},
			wantStdoutNone: true,
		},
		{
			name: "oversized current file is bad input",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", noRegressionBaseline)
			},
			currentFile: func(dir string) string {
				return writeOversizedFixture(t, dir, "current.txt")
			},
			wantExit: exitBadInput,
			wantStderrHas: []string{
				"size limit",
			},
			wantStdoutNone: true,
		},
		{
			name: "benchmark missing from current run fails",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", missingBenchmarkBaseline)
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", missingBenchmarkCurrent)
			},
			wantExit: exitViolation,
			wantStderrHas: []string{
				"Beta",
				"missing from current run",
			},
			wantStdoutNone: true,
		},
		{
			name: "all baseline benchmarks missing from current run fails with names",
			baselineFile: func(dir string) string {
				return writeFixture(t, dir, "baseline.txt", missingBenchmarkBaseline)
			},
			currentFile: func(dir string) string {
				return writeFixture(t, dir, "current.txt", noBenchmarkRowsCurrent)
			},
			wantExit: exitViolation,
			wantStderrHas: []string{
				"Alpha",
				"Beta",
				"missing from current run",
			},
			wantStdoutNone: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			baselinePath := tc.baselineFile(dir)
			currentPath := tc.currentFile(dir)

			var stdout, stderr bytes.Buffer
			gotExit := run(baselinePath, currentPath, &stdout, &stderr)

			if gotExit != tc.wantExit {
				t.Errorf("run() exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
					gotExit, tc.wantExit, stdout.String(), stderr.String())
			}
			if tc.wantStdoutHas != "" && !strings.Contains(stdout.String(), tc.wantStdoutHas) {
				t.Errorf("stdout = %q, want substring %q", stdout.String(), tc.wantStdoutHas)
			}
			for _, want := range tc.wantStderrHas {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want substring %q", stderr.String(), want)
				}
			}
			if tc.wantStdoutNone && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
			if tc.wantStderrNone && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestRunMultipleViolationsSortedByBenchmark guards formatFail's ordering
// contract (sorted by benchmark, then metric — main.go's checkBudget) using
// two benchmarks that regress on different metrics, so accidental
// insertion-order output would be caught rather than masked by a
// single-violation fixture.
func TestRunMultipleViolationsSortedByBenchmark(t *testing.T) {
	dir := t.TempDir()
	baselinePath := writeFixture(t, dir, "baseline.txt", multiViolationBaseline)
	currentPath := writeFixture(t, dir, "current.txt", multiViolationCurrent)

	var stdout, stderr bytes.Buffer
	gotExit := run(baselinePath, currentPath, &stdout, &stderr)

	if gotExit != exitViolation {
		t.Fatalf("run() exit = %d, want %d\nstderr:\n%s", gotExit, exitViolation, stderr.String())
	}
	out := stderr.String()
	alphaIdx := strings.Index(out, "Alpha")
	betaIdx := strings.Index(out, "Beta")
	if alphaIdx == -1 || betaIdx == -1 {
		t.Fatalf("stderr missing expected benchmarks: %q", out)
	}
	if alphaIdx >= betaIdx {
		t.Errorf("stderr lists BenchmarkBeta before BenchmarkAlpha, want alphabetical order:\n%s", out)
	}
}

// TestRowViolation exercises the regression-budget boundary directly:
// exactly-at-budget deltas must pass (rowViolation uses a strict ">"), since
// line coverage alone would not catch a ">" -> ">=" regression here (both
// compile and execute regardless of which comparison operator is used).
func TestRowViolation(t *testing.T) {
	tests := []struct {
		name          string
		metric        string
		row           *benchstat.Row
		wantViolation bool
	}{
		{
			name:   "not statistically significant is never a violation",
			metric: metricTimeOp,
			row:    &benchstat.Row{Benchmark: "Example", Change: 0, PctDelta: 50.0},
		},
		{
			name:   "improvement is never a violation",
			metric: metricTimeOp,
			row:    &benchstat.Row{Benchmark: "Example", Change: +1, PctDelta: -50.0},
		},
		{
			name:   "ns/op exactly at budget passes",
			metric: metricTimeOp,
			row:    &benchstat.Row{Benchmark: "Example", Change: -1, PctDelta: maxNsOpRegressionPercent},
		},
		{
			name:          "ns/op just over budget fails",
			metric:        metricTimeOp,
			row:           &benchstat.Row{Benchmark: "Example", Change: -1, PctDelta: maxNsOpRegressionPercent + 0.1},
			wantViolation: true,
		},
		{
			name:   "allocs/op exactly at budget passes",
			metric: metricAllocsOp,
			row: &benchstat.Row{Benchmark: "Example", Change: -1, Metrics: []*benchstat.Metrics{
				{Mean: 0},
				{Mean: maxAllocsOpIncrease},
			}},
		},
		{
			name:   "allocs/op just over budget fails",
			metric: metricAllocsOp,
			row: &benchstat.Row{Benchmark: "Example", Change: -1, Metrics: []*benchstat.Metrics{
				{Mean: 0},
				{Mean: maxAllocsOpIncrease + 1},
			}},
			wantViolation: true,
		},
		{
			name:   "allocs/op with fewer than two metrics is never a violation",
			metric: metricAllocsOp,
			row: &benchstat.Row{Benchmark: "Example", Change: -1, Metrics: []*benchstat.Metrics{
				{Mean: 0},
			}},
		},
		{
			name:   "unrecognized metric is never a violation",
			metric: "B/op",
			row:    &benchstat.Row{Benchmark: "Example", Change: -1, PctDelta: 500.0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := rowViolation(tc.metric, tc.row)
			if ok != tc.wantViolation {
				t.Errorf("rowViolation(%q, %+v) ok = %v, want %v", tc.metric, tc.row, ok, tc.wantViolation)
			}
		})
	}
}

// TestMissingBenchmarks exercises missingBenchmarks directly against a
// synthetic Collection, isolating it from the text-parsing and statistical
// machinery exercised by TestRun's end-to-end "benchmark missing from
// current run fails" case.
func TestMissingBenchmarks(t *testing.T) {
	var coll benchstat.Collection
	coll.AddConfig(configBaseline, []byte(missingBenchmarkBaseline))
	coll.AddConfig(configCurrent, []byte(missingBenchmarkCurrent))

	// Alpha-8 is present in both configs and must be omitted; Beta-8 is
	// present only in the baseline and must be reported as missing.
	got := missingBenchmarks(&coll)
	want := []string{"Beta-8"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("missingBenchmarks() = %v, want %v", got, want)
	}
}
