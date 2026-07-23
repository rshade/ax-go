// Command sizecheck enforces ax-go's binary-size guarantee for the
// import-isolated logging surface.
//
// It builds two committed probe programs with production flags
// (-trimpath -ldflags="-s -w") and enforces two independent budgets:
//
//   - An ABSOLUTE CEILING on examples/logging, the program that imports only
//     github.com/rshade/ax-go/logging.
//   - A MINIMUM REDUCTION RATIO of examples/logging against examples/rootlogging,
//     the byte-for-byte identical program built on the root facade.
//
// Both are hardcoded Go constants, following the covercheck/benchcheck/
// surfacecheck precedent: every budget change is a reviewable commit auditable
// through git blame, never a config file edited in passing.
//
// The two are NOT adjusted under the same rules. See the constants' own doc
// comments — the distinction is the most important thing in this file.
//
// Stream and exit contract, matching the sibling gates: a pass writes one
// minified JSON object to stdout and nothing to stderr, exiting 0. Every failure
// writes nothing to stdout and exactly one minified ax.Error envelope to stderr.
//
// Run from the module root:
//
//	go run ./internal/cmd/sizecheck
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/rshade/ax-go/contract"
)

// maxIsolatedBinaryBytes is the absolute ceiling for a stripped, trimmed
// linux/amd64 build of examples/logging (SC-001).
//
// Measured at 2,261,257 bytes on Go 1.26.5, so this leaves roughly 32% headroom.
// The headroom is deliberate: absolute binary size drifts with the toolchain, and
// a ceiling with no slack would fail on a Go release that changed nothing about
// this repository.
//
// ADJUSTABLE for a reviewed reason, on the same protocol as the coverage and
// benchmark budgets: confirm the increase is understood (a jump usually means a
// new transitive dependency — check `go list -deps` BEFORE touching this
// constant), edit it, verify with `make size-check`, and record the reason in the
// commit message.
const maxIsolatedBinaryBytes int64 = 3_000_000

// minReductionPercent is the minimum size reduction of examples/logging against
// examples/rootlogging (SC-002). Measured at 81.18%.
//
// NOT adjustable on the same terms as the ceiling, and the difference is the
// whole reason both exist. This ratio is toolchain-INDEPENDENT: both probes are
// built by the same compiler from the same source in the same run, so a newer Go
// moves them together and cannot explain a ratio breach. A breach therefore means
// exactly one thing — the isolated surface gained weight the root facade did not,
// which is precisely the regression this feature exists to prevent.
//
// Lowering it re-negotiates the feature's headline claim. That is a spec change:
// update SC-002 in specs/017-import-isolated-logging/spec.md in the same commit,
// or find and fix the dependency that crept in. Never lower it to make a red gate
// green.
const minReductionPercent = 75.0

// statusPass is the value of the pass document's status field.
const statusPass = "pass"

// percentScale converts a 0..1 ratio to a percentage.
const percentScale = 100

// Probe programs. isolatedProbe is the measured subject; rootProbe is the ratio
// denominator. They are byte-for-byte the same program apart from the surface
// imported, which is what makes the ratio meaningful.
const (
	isolatedProbe = "./examples/logging"
	rootProbe     = "./examples/rootlogging"
)

// Error codes for the ax.Error envelope. Each names a distinct failure the
// maintainer resolves differently, which is why a build failure, a ceiling
// breach, and a ratio breach are never collapsed into one code.
const (
	codeBuildFailed    = "size_build_failed"
	codeCeiling        = "size_ceiling_exceeded"
	codeRatio          = "size_reduction_insufficient"
	codeArtifact       = "invalid_size_artifact"
	codeSizePermission = "size_permission"
	codeInternal       = "size_internal"
)

// buildFlags returns the production flags the size claim is stated under.
// Measuring an unstripped debug binary would report a number no consumer ever
// ships.
func buildFlags() []string {
	return []string{"-trimpath", "-ldflags=-s -w"}
}

// result is the pass document written to stdout.
type result struct {
	Status           string  `json:"status"`
	IsolatedBytes    int64   `json:"isolated_bytes"`
	RootBytes        int64   `json:"root_bytes"`
	ReductionPercent float64 `json:"reduction_percent"`
	CeilingBytes     int64   `json:"ceiling_bytes"`
	MinReduction     float64 `json:"min_reduction_percent"`
}

// builder builds pkg into out. It is an indirection solely so tests can exercise
// the three outcome paths without invoking the Go toolchain twice per case.
type builder func(ctx context.Context, dir, pkg, out string) error

type runConfig struct {
	Dir          string
	Build        builder
	Ceiling      int64
	MinReduction float64
	IsolatedPkg  string
	RootPkg      string
}

func (c runConfig) resolve() runConfig {
	if c.Dir == "" {
		c.Dir = "."
	}
	if c.Build == nil {
		c.Build = goBuild
	}
	if c.Ceiling == 0 {
		c.Ceiling = maxIsolatedBinaryBytes
	}
	if c.MinReduction == 0 {
		c.MinReduction = minReductionPercent
	}
	if c.IsolatedPkg == "" {
		c.IsolatedPkg = isolatedProbe
	}
	if c.RootPkg == "" {
		c.RootPkg = rootProbe
	}
	return c
}

func main() {
	os.Exit(run(context.Background(), runConfig{}, os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the gate. Success writes one minified JSON document to stdout;
// every failure writes exactly one minified ax.Error envelope to stderr.
func run(ctx context.Context, cfg runConfig, args []string, stdout, stderr io.Writer) int {
	flagSet := flag.NewFlagSet("sizecheck", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	if err := flagSet.Parse(args); err != nil {
		return emitFailure(ctx, stderr, codeArtifact, "invalid sizecheck flags",
			"fix the command line: "+err.Error(), contract.ExitValidation, err)
	}
	if flagSet.NArg() != 0 {
		err := fmt.Errorf("unexpected positional arguments: %s", strings.Join(flagSet.Args(), " "))
		return emitFailure(ctx, stderr, codeArtifact, "invalid sizecheck arguments",
			"remove the positional arguments", contract.ExitValidation, err)
	}

	cfg = cfg.resolve()

	tmp, err := os.MkdirTemp("", "ax-sizecheck-")
	if err != nil {
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure creating the build directory",
			"", contract.ExitInternal, err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	isolatedBytes, code := measure(ctx, stderr, cfg, cfg.IsolatedPkg, filepath.Join(tmp, "isolated.bin"))
	if code != 0 {
		return code
	}
	rootBytes, code := measure(ctx, stderr, cfg, cfg.RootPkg, filepath.Join(tmp, "root.bin"))
	if code != 0 {
		return code
	}

	if isolatedBytes > cfg.Ceiling {
		ceilingErr := fmt.Errorf("%s is %d bytes, exceeding the %d-byte ceiling by %d",
			cfg.IsolatedPkg, isolatedBytes, cfg.Ceiling, isolatedBytes-cfg.Ceiling)
		return emitFailure(ctx, stderr, codeCeiling,
			"the import-isolated logging binary exceeds its absolute size ceiling",
			"run `go list -deps "+cfg.IsolatedPkg+"` to find the new transitive dependency before "+
				"considering a ceiling change in internal/cmd/sizecheck/main.go",
			contract.ExitValidation, ceilingErr)
	}

	reduction, code := reductionPercent(ctx, stderr, isolatedBytes, rootBytes)
	if code != 0 {
		return code
	}

	if reduction < cfg.MinReduction {
		ratioErr := fmt.Errorf("reduction is %.2f%% (%d vs %d bytes), below the %.2f%% minimum",
			reduction, isolatedBytes, rootBytes, cfg.MinReduction)
		return emitFailure(ctx, stderr, codeRatio,
			"the import-isolated logging binary is not sufficiently smaller than the root-facade build",
			"the ratio is toolchain-independent, so a newer Go cannot explain this: something reachable "+
				"from the logging surface gained weight the root facade did not. Find it with "+
				"`go list -deps` before touching minReductionPercent, which is a spec change, not a calibration",
			contract.ExitValidation, ratioErr)
	}

	return writeDocument(ctx, stdout, stderr, result{
		Status:           statusPass,
		IsolatedBytes:    isolatedBytes,
		RootBytes:        rootBytes,
		ReductionPercent: reduction,
		CeilingBytes:     cfg.Ceiling,
		MinReduction:     cfg.MinReduction,
	})
}

// measure builds pkg and returns the resulting binary's size in bytes.
func measure(ctx context.Context, stderr io.Writer, cfg runConfig, pkg, out string) (int64, int) {
	if err := cfg.Build(ctx, cfg.Dir, pkg, out); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return 0, emitFailure(ctx, stderr, codeSizePermission,
				"permission denied executing the Go toolchain",
				"check execute and read permissions on the Go toolchain and module",
				contract.ExitAuth, err)
		}
		return 0, emitFailure(ctx, stderr, codeBuildFailed,
			"could not build the size probe "+pkg,
			"the size budget was NOT checked; fix the build failure first",
			contract.ExitValidation, err)
	}

	info, err := os.Stat(out)
	if err != nil {
		return 0, emitFailure(ctx, stderr, codeInternal,
			"unexpected internal failure measuring the built probe "+pkg,
			"", contract.ExitInternal, err)
	}
	return info.Size(), 0
}

// reductionPercent computes 1 - isolated/root as a percentage. A zero-byte root
// probe would make the ratio undefined; that can only mean the build produced
// nothing, so it is reported as an internal failure rather than silently treated
// as a 0% or 100% reduction.
func reductionPercent(ctx context.Context, stderr io.Writer, isolated, root int64) (float64, int) {
	if root <= 0 {
		err := fmt.Errorf("root probe measured %d bytes, so the reduction ratio is undefined", root)
		return 0, emitFailure(ctx, stderr, codeInternal,
			"unexpected internal failure computing the size reduction",
			"", contract.ExitInternal, err)
	}
	return (1 - float64(isolated)/float64(root)) * percentScale, 0
}

// goBuild builds pkg into out with the production flags the size claim is stated
// under.
func goBuild(ctx context.Context, dir, pkg, out string) error {
	args := append([]string{"build"}, buildFlags()...)
	args = append(args, "-o", out, pkg)

	// #nosec G204 -- pkg is a package path from this file's constants, not
	// shell-interpreted input; exec.CommandContext passes it as argv.
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	if combined, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build %s: %w: %s", pkg, err, strings.TrimSpace(string(combined)))
	}
	return nil
}

// writeDocument writes the minified pass document to stdout.
func writeDocument(ctx context.Context, stdout, stderr io.Writer, v any) int {
	payload, err := json.Marshal(v)
	if err != nil {
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure encoding the result",
			"", contract.ExitInternal, err)
	}
	if _, writeErr := stdout.Write(append(payload, '\n')); writeErr != nil {
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure writing the result",
			"", contract.ExitInternal, writeErr)
	}
	return contract.ExitSuccess
}

// emitFailure writes exactly one minified ax.Error envelope to stderr and returns
// the mapped exit code.
func emitFailure(ctx context.Context, stderr io.Writer, code, message, suggestion string, exit int, cause error) int {
	opts := []contract.ErrorOption{
		contract.WithErrorTool("sizecheck"),
		contract.WithErrorVersion(toolVersion()),
		contract.WithErrorExitCode(exit),
		contract.WithRetryable(false),
		contract.WithErrorCause(cause),
	}
	if suggestion != "" {
		opts = append(opts, contract.WithSuggestions(suggestion))
	}
	env := contract.NewError(ctx, code, message, opts...)
	if err := contract.WriteError(stderr, env); err != nil {
		return contract.ExitInternal
	}
	return exit
}

// toolVersion reports the module version embedded at build time, falling back to
// "dev" for go run and test binaries so output stays deterministic.
func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
