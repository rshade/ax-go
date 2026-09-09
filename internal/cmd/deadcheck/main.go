// Command deadcheck gates unexported and internal functions reported unreachable
// by deadcode -test in all four supported build-tag configurations on the host
// platform. It intersects reports by package and function name; a function
// missing from any report (reachable or absent in that build) is not a finding.
//
// Tests are roots alongside main packages. This detects helpers called by
// neither production code nor tests; it does not prove production usage.
// Exported methods and functions outside internal packages are excluded because
// downstream library consumers are not analysis roots. This is not an exported
// API gate: surfacecheck and apidiff-verdict own that question. Generated code
// and marker methods retain deadcode's default exclusions.
//
// The binary is pinned in mise.toml; run make ensure before using this gate.
// All four configurations always run, with a five-minute timeout per analysis.
// A pass writes one minified JSON object to stdout and nothing to stderr (0).
// Failures write only one ax.Error envelope to stderr: findings, invalid output,
// or tool/build failures (2), timeout (3), permission denial (4), or internal
// failures (1).
// Run from the module root: go run ./internal/cmd/deadcheck.
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
	"runtime/debug"
	"strings"

	"github.com/rshade/ax-go/contract"
)

const (
	codeDead       = "deadcode_unreachable"
	codeArtifact   = "invalid_deadcode_artifact"
	codeAnalysis   = "deadcode_analysis_failed"
	codePermission = "deadcode_permission"
	codeInternal   = "deadcode_internal"
	codeTimeout    = "deadcode_timeout"
)

type analyzer func(context.Context, string) ([]byte, error)

type result struct {
	Status         string `json:"status"`
	Configurations int    `json:"configurations"`
	Unreachable    int    `json:"unreachable"`
}

// buildTags is policy, not a user override. Keep in sync with BUILD_TAG_MATRIX;
// TestBuildTagMatrix verifies the Makefile contains exactly these combinations.
func buildTags() []string {
	return []string{"", "ax_no_grpc", "ax_no_otlp", "ax_no_grpc,ax_no_otlp"}
}

func main() {
	os.Exit(run(context.Background(), deadcode, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, analyze analyzer, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("deadcheck", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		return emitFailure(
			ctx,
			stderr,
			codeArtifact,
			"invalid deadcheck flags",
			"remove unsupported flags",
			contract.ExitValidation,
			err,
		)
	}
	if flags.NArg() != 0 {
		return emitFailure(
			ctx,
			stderr,
			codeArtifact,
			"invalid deadcheck arguments",
			"run deadcheck from the module root without arguments",
			contract.ExitValidation,
			fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " ")),
		)
	}
	var common map[string]finding
	for i, tags := range buildTags() {
		data, err := analyze(ctx, tags)
		if err != nil {
			return analysisFailure(ctx, stderr, tags, err)
		}
		report, err := parseReport(data)
		if err != nil {
			return emitFailure(
				ctx,
				stderr,
				codeArtifact,
				"invalid deadcode JSON output",
				"check the pinned deadcode binary and its output",
				contract.ExitValidation,
				err,
			)
		}
		if i == 0 {
			common = report
			continue
		}
		for key := range common {
			if _, found := report[key]; !found {
				delete(common, key)
			}
		}
	}
	if len(common) != 0 {
		return emitFailure(
			ctx,
			stderr,
			codeDead,
			"unreachable unexported or internal functions",
			"remove unused helpers or restore their callers; review findings before deleting code",
			contract.ExitValidation,
			errors.New(describeFindings(common)),
		)
	}
	payload, err := json.Marshal(result{Status: "pass", Configurations: len(buildTags()), Unreachable: 0})
	if err != nil {
		return emitFailure(ctx, stderr, codeInternal, "could not encode result", "", contract.ExitInternal, err)
	}
	if _, writeErr := stdout.Write(append(payload, '\n')); writeErr != nil {
		return emitFailure(ctx, stderr, codeInternal, "could not write result", "", contract.ExitInternal, writeErr)
	}
	return contract.ExitSuccess
}

func analysisFailure(ctx context.Context, stderr io.Writer, tags string, err error) int {
	code, exit := codeAnalysis, contract.ExitValidation
	switch {
	case errors.Is(err, fs.ErrPermission):
		code, exit = codePermission, contract.ExitAuth
	case errors.Is(err, context.DeadlineExceeded):
		code, exit = codeTimeout, contract.ExitNetwork
	case errors.Is(err, context.Canceled):
		code, exit = codeInternal, contract.ExitInternal
	}
	return emitFailure(
		ctx,
		stderr,
		code,
		"deadcode analysis did not complete",
		"run make ensure and fix tool or package-loading errors; reachability was not checked",
		exit,
		fmt.Errorf("tags=%q: %w", tags, err),
	)
}

func emitFailure(ctx context.Context, stderr io.Writer, code, message, suggestion string, exit int, cause error) int {
	opts := []contract.ErrorOption{
		contract.WithErrorTool("deadcheck"), contract.WithErrorVersion(toolVersion()),
		contract.WithErrorExitCode(exit), contract.WithRetryable(false), contract.WithErrorCause(cause),
	}
	// WithErrorCause preserves error identity but is not serialized. Put the
	// diagnostic in the existing context extension rather than changing ax.Error.
	if cause != nil {
		opts = append(opts, contract.WithErrorContext(map[string]any{"cause": cause.Error()}))
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
