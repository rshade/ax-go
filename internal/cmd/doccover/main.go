// Command doccover enforces ExampleXxx coverage on ax-go's primary API surface.
//
// Go's ExampleXxx functions compile and (when they declare an Output comment)
// are executed by `go test`, which makes them self-verifying documentation that
// cannot silently drift the way a prose comment can. Only output-declaring
// examples ("// Output:" or "// Unordered output:") count as coverage:
// a compile-only example proves nothing about behavior. doccover gates the
// curated primary API only: constructors, the core exported types, and the
// principal entry points. The broad expectation that every exported symbol
// carries a doc comment is enforced separately by golangci-lint (godoclint's
// require-doc).
//
// Coverage is ratcheted through baseline.txt so the gate can land before every
// example exists. doccover fails on (1) a regression — a required symbol with
// no verified example that is not listed in the baseline; (2) a required symbol
// that is no longer exported (renamed or removed); and (3) a stale baseline
// entry — once a symbol gains a verified example its baseline line must be
// pruned, making the ratchet one-way.
//
// Run from the module root:
//
//	go run ./internal/cmd/doccover
package main

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// requiredSymbols returns the curated primary API surface that MUST carry a
// verified ExampleXxx. It is intentionally a hand-maintained subset
// (constructors, core types, and top-level entry points), not every exported
// identifier: WithX option setters are demonstrated inside a parent example, and
// the rest of the surface is covered by doc comments. Add a symbol here when it
// becomes part of the primary API an agent is expected to call directly.
func requiredSymbols() []string {
	return []string{
		// constructors and entry points
		"BuildMCPSchema",
		"BuildSchema",
		"Execute",
		"Flush",
		"NewEntityID",
		"NewEnvelope",
		"NewError",
		"NewIdempotencyKey",
		"NewLogger",
		"ParseConfig",
		"ParseConfigFile",
		"PatchConfig",
		"PatchConfigFile",
		"ResolveVersion",
		"StartTelemetry",
		"WithLokiFromEnv",
		// core types
		"Envelope",
		"Error",
		"Logger",
		"Mode",
		"Schema",
		"Telemetry",
	}
}

func main() {
	root, err := moduleRoot()
	if err != nil {
		failf("locating module root: %v", err)
	}

	exported, examples, err := scanPackage(root)
	if err != nil {
		failf("scanning package: %v", err)
	}

	baselinePath := filepath.Join(root, "internal", "cmd", "doccover", "baseline.txt")
	baseline, err := readBaseline(baselinePath)
	if err != nil {
		failf("reading baseline: %v", err)
	}

	os.Exit(report(os.Stdout, requiredSymbols(), exported, examples, baseline))
}

// scanPackage parses the root-package Go files and returns the set of exported
// top-level symbol names (funcs and types from non-test files) and the set of
// verified ExampleXxx function names (from _test.go files; only examples that
// declare an output comment count).
func scanPackage(dir string) (map[string]bool, map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	exported := map[string]bool{}
	examples := map[string]bool{}
	fset := token.NewFileSet()

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}

		// ParseComments is required: go/doc.Examples reads the "// Output:"
		// comment from file.Comments, and without it every example would
		// report no output and count as unverified.
		file, parseErr := parser.ParseFile(
			fset,
			filepath.Join(dir, name),
			nil,
			parser.ParseComments|parser.SkipObjectResolution,
		)
		if parseErr != nil {
			return nil, nil, parseErr
		}

		if strings.HasSuffix(name, "_test.go") {
			collectVerifiedExamples(file, examples)
			continue
		}
		collectExported(file, exported)
	}

	return exported, examples, nil
}

// collectExported records exported top-level functions (without receivers) and
// exported type names declared in file.
func collectExported(file *ast.File, exported map[string]bool) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.IsExported() {
				exported[d.Name.Name] = true
			}
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
					exported[ts.Name.Name] = true
				}
			}
		}
	}
}

// collectVerifiedExamples records ExampleXxx functions in file that declare an
// output comment ("// Output:" or "// Unordered output:"), keyed by their full
// Go name (ExampleSymbol or ExampleSymbol_suffix). Examples without an output
// comment compile but never execute under `go test`, so they verify nothing
// and do not count as coverage. The file must be parsed with
// parser.ParseComments.
func collectVerifiedExamples(file *ast.File, examples map[string]bool) {
	for _, example := range doc.Examples(file) {
		if example.Output == "" && !example.EmptyOutput {
			continue
		}
		examples["Example"+example.Name] = true
	}
}

// hasExample reports whether symbol has an associated example, following Go's
// naming convention: ExampleSymbol, or ExampleSymbol_suffix for variants and
// methods.
func hasExample(symbol string, examples map[string]bool) bool {
	if examples["Example"+symbol] {
		return true
	}
	prefix := "Example" + symbol + "_"
	for name := range examples {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// report prints the coverage summary to out and returns the process exit code:
// non-zero on a regression (a required symbol with no verified example that is
// not exempted in the baseline), on a required symbol that is no longer
// exported (renamed or removed), or on a stale baseline entry (the ratchet is
// one-way: once covered, the baseline line must be pruned).
func report(out io.Writer, required []string, exported, examples, baseline map[string]bool) int {
	var covered, missing, regressions, unknown []string
	for _, symbol := range required {
		if !exported[symbol] {
			unknown = append(unknown, symbol)
		}
		if hasExample(symbol, examples) {
			covered = append(covered, symbol)
			continue
		}
		missing = append(missing, symbol)
		if !baseline[symbol] {
			regressions = append(regressions, symbol)
		}
	}

	stale := staleBaseline(required, baseline, examples)
	sort.Strings(missing)
	sort.Strings(regressions)
	sort.Strings(unknown)
	sort.Strings(stale)

	fmt.Fprintf(out, "doc-coverage: %d/%d required symbols have an example\n", len(covered), len(required))
	if len(missing) > 0 {
		fmt.Fprintf(out, "missing (%d, baseline-exempt unless marked NEW):\n", len(missing))
		for _, symbol := range missing {
			marker := ""
			if !baseline[symbol] {
				marker = "  <- NEW regression"
			}
			fmt.Fprintf(out, "  - %s%s\n", symbol, marker)
		}
	}
	for _, symbol := range unknown {
		fmt.Fprintf(out, "required symbol %q is not exported (renamed or removed?)\n", symbol)
	}
	for _, symbol := range stale {
		fmt.Fprintf(out, "stale baseline entry %q (now covered or no longer required)\n", symbol)
	}

	return printVerdict(out, regressions, unknown, stale)
}

// printVerdict writes the FAIL summaries for regressions, unknown required
// symbols, and stale baseline entries, returning 1 when any are present and 0
// (with the OK line) otherwise.
func printVerdict(out io.Writer, regressions, unknown, stale []string) int {
	failed := false
	if len(regressions) > 0 {
		failed = true
		fmt.Fprintf(
			out,
			"\nFAIL: %d required symbol(s) missing a verified example (not baselined):\n",
			len(regressions),
		)
		for _, symbol := range regressions {
			fmt.Fprintf(out, "  - %s\n", symbol)
		}
		fmt.Fprintln(
			out,
			"\nAdd an ExampleXxx with an // Output: comment (see example_test.go) or add the symbol to internal/cmd/doccover/baseline.txt.",
		)
	}
	if len(unknown) > 0 {
		failed = true
		fmt.Fprintf(
			out,
			"\nFAIL: %d required symbol(s) not exported (renamed or removed?) — update requiredSymbols in internal/cmd/doccover/main.go.\n",
			len(unknown),
		)
	}
	if len(stale) > 0 {
		failed = true
		fmt.Fprintf(
			out,
			"\nFAIL: %d stale baseline entry(ies) — prune covered/obsolete entries from internal/cmd/doccover/baseline.txt (the ratchet is one-way).\n",
			len(stale),
		)
	}
	if failed {
		return 1
	}

	fmt.Fprintln(out, "OK: no example-coverage regressions.")
	return 0
}

// staleBaseline returns baseline entries that are no longer required or are now
// covered by an example, so they can be pruned.
func staleBaseline(required []string, baseline, examples map[string]bool) []string {
	requiredSet := map[string]bool{}
	for _, symbol := range required {
		requiredSet[symbol] = true
	}

	var stale []string
	for symbol := range baseline {
		if !requiredSet[symbol] || hasExample(symbol, examples) {
			stale = append(stale, symbol)
		}
	}
	return stale
}

// readBaseline loads the newline-delimited baseline file. Blank lines and lines
// starting with '#' are ignored. A missing file is treated as an empty baseline.
func readBaseline(path string) (map[string]bool, error) {
	baseline := map[string]bool{}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return baseline, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		baseline[line] = true
	}
	return baseline, scanner.Err()
}

// moduleRoot walks up from the working directory to the directory containing
// go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found from working directory")
		}
		dir = parent
	}
}

// failf prints a fatal diagnostic to stderr and exits non-zero.
func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "doccover: "+format+"\n", args...)
	os.Exit(1)
}
