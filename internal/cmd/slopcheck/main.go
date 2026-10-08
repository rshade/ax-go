// Command slopcheck gates struct fields that a composite literal assigns and
// no code reads: the table-test field (expectError bool) that every case sets
// and no assertion consults, so the suite passes while proving less than it
// appears to.
//
// No linter already in the pipeline sees this. unused treats a write in a
// literal as a use, structcheck was folded into unused without the check, the
// ineffassign family works on locals, and the make slop ast-grep report has
// no type information to resolve a selector to a field.
//
// Harness: the classification lives in internal/unreadfield, consumed by two
// front ends. This stdlib-only driver is the gate because it owns the stream
// and exit contract below; the go/analysis adapter in
// internal/unreadfield/analyzer gives analysistest fixtures and a future lint
// plugin path. Both call the same Analyze, so they cannot disagree.
//
// Scope: every package under -dir (./...), test files included, because tests
// are readers. Exported fields of types nameable outside the package are
// skipped, since a downstream consumer may read them. All four build-tag
// configurations (unreadfield.BuildConfigurations, kept equal to the Makefile
// BUILD_TAG_MATRIX) run on the host platform, and a field is reported only
// when it is unread in every configuration in which it is assigned.
//
// It cannot prove a field is dead. It treats every use it cannot follow
// (equality, interface conversion, reflection, code outside the package) as a
// read of every field, so it misses fields read that way; it does not see
// other platforms' build constraints; and it ignores fields that only
// assignment statements write. There is no allowlist: read the field where it
// matters, or delete it and its assignments.
//
// A pass writes one minified JSON object to stdout and nothing to stderr (0).
// Every failure writes nothing to stdout and one ax.Error envelope to stderr:
// unread fields, invalid flags or -dir, or a package that does not load or
// type-check (2); a configuration exceeding its five-minute timeout (3);
// permission denial (4); cancellation or an internal failure (1).
//
// Run from the module root: go run ./internal/cmd/slopcheck.
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
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/unreadfield"
)

const (
	codeUnread     = "slopcheck_unread_field"
	codeArtifact   = "invalid_slopcheck_artifact"
	codeAnalysis   = "slopcheck_analysis_failed"
	codeTimeout    = "slopcheck_timeout"
	codePermission = "slopcheck_permission"
	codeInternal   = "slopcheck_internal"

	analysisTimeout = 5 * time.Minute
)

// runner is unreadfield.Run's signature; tests substitute it to reach every
// failure class.
type runner func(context.Context, string, []unreadfield.Configuration, time.Duration) (unreadfield.Report, error)

type result struct {
	Status         string `json:"status"`
	Configurations int    `json:"configurations"`
	Packages       int    `json:"packages"`
	Fields         int    `json:"fields"`
	Unread         int    `json:"unread"`
}

func main() {
	os.Exit(run(context.Background(), unreadfield.Run, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, analyze runner, args []string, stdout, stderr io.Writer) int {
	dir, code := parseArgs(ctx, args, stderr)
	if code >= 0 {
		return code
	}
	report, err := analyze(ctx, dir, unreadfield.BuildConfigurations(), analysisTimeout)
	if err != nil {
		return analysisFailure(ctx, stderr, err)
	}
	if len(report.Findings) != 0 {
		causes := make([]string, len(report.Findings))
		for i, f := range report.Findings {
			causes[i] = f.String()
		}
		return emitFailure(ctx, stderr, codeUnread,
			"struct fields assigned in composite literals but never read",
			"read the field where it matters, or delete it and its assignments; review before deleting",
			contract.ExitValidation, errors.New(strings.Join(causes, "; ")))
	}
	payload, err := json.Marshal(result{
		Status: "pass", Configurations: report.Configurations, Packages: report.Packages, Fields: report.Fields,
	})
	if err != nil {
		return emitFailure(ctx, stderr, codeInternal, "could not encode result", "", contract.ExitInternal, err)
	}
	if _, writeErr := stdout.Write(append(payload, '\n')); writeErr != nil {
		return emitFailure(ctx, stderr, codeInternal, "could not write result", "", contract.ExitInternal, writeErr)
	}
	return contract.ExitSuccess
}

// parseArgs returns the module root to analyze and -1, or an exit code after
// emitting the failure envelope.
func parseArgs(ctx context.Context, args []string, stderr io.Writer) (string, int) {
	flags := flag.NewFlagSet("slopcheck", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dir := flags.String("dir", ".", "module root to analyze")
	if err := flags.Parse(args); err != nil {
		return "", emitFailure(ctx, stderr, codeArtifact, "invalid slopcheck flags",
			"remove unsupported flags", contract.ExitValidation, err)
	}
	if flags.NArg() != 0 {
		return "", emitFailure(ctx, stderr, codeArtifact, "invalid slopcheck arguments",
			"run slopcheck from the module root without arguments, or pass -dir", contract.ExitValidation,
			fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " ")))
	}
	if _, err := os.Stat(filepath.Join(*dir, "go.mod")); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return "", emitFailure(ctx, stderr, codePermission, "cannot read the module root", "",
				contract.ExitAuth, err)
		}
		return "", emitFailure(ctx, stderr, codeArtifact, "-dir is not a module root",
			"pass a directory containing go.mod", contract.ExitValidation, err)
	}
	return *dir, -1
}

// analysisFailure classifies err in the contract's precedence order: timeout,
// cancellation, permission, analysis, internal. A timeout, cancellation, or
// permission failure wrapped in *unreadfield.AnalysisError keeps its own code.
func analysisFailure(ctx context.Context, stderr io.Writer, err error) int {
	var analysisErr *unreadfield.AnalysisError
	code, exit := codeInternal, contract.ExitInternal
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code, exit = codeTimeout, contract.ExitNetwork
	case errors.Is(err, context.Canceled):
	case errors.Is(err, fs.ErrPermission):
		code, exit = codePermission, contract.ExitAuth
	case errors.As(err, &analysisErr):
		code, exit = codeAnalysis, contract.ExitValidation
	}
	return emitFailure(ctx, stderr, code, "unread-field analysis did not complete",
		"fix package-loading or type errors; unread fields were not checked", exit, err)
}

func emitFailure(ctx context.Context, stderr io.Writer, code, message, suggestion string, exit int, cause error) int {
	opts := []contract.ErrorOption{
		contract.WithErrorTool("slopcheck"), contract.WithErrorVersion(toolVersion()),
		contract.WithErrorExitCode(exit), contract.WithRetryable(false), contract.WithErrorCause(cause),
		// WithErrorCause preserves error identity but is not serialized; the
		// diagnostic goes in the existing context extension, as deadcheck does.
		contract.WithErrorContext(map[string]any{"cause": cause.Error()}),
	}
	if suggestion != "" {
		opts = append(opts, contract.WithSuggestions(suggestion))
	}
	if err := contract.WriteError(stderr, contract.NewError(ctx, code, message, opts...)); err != nil {
		return contract.ExitInternal
	}
	return exit
}

func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
