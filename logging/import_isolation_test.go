package logging_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

const (
	loggingImportPath = "github.com/rshade/ax-go/logging"
	exampleImportPath = "github.com/rshade/ax-go/examples/logging"

	zerologImportPath   = "github.com/rs/zerolog"
	otelTraceImportPath = "go.opentelemetry.io/otel/trace"
)

// buildConfigurations is the exhaustive set of supported build-tag combinations.
// Both ax-go constraints are negative, so the default configuration passes no
// tags at all.
//
// Iterating all four is not defensive padding. A green untagged `go list` says
// nothing about the graph under ax_no_grpc or ax_no_otlp — the toolchain resolves
// a different file set entirely — and FR-014 requires the isolation guarantee to
// hold identically under every configuration. This surface should be the easy
// case (it links none of the trees those tags decline, so its graph must not vary
// at all), and asserting that explicitly is what turns "should" into "does".
func buildConfigurations() [][]string {
	return [][]string{
		nil,
		{"ax_no_grpc"},
		{"ax_no_otlp"},
		{"ax_no_grpc", "ax_no_otlp"},
	}
}

func configName(tags []string) string {
	if len(tags) == 0 {
		return "default"
	}
	return strings.Join(tags, ",")
}

// TestLoggingSurfaceIsImportIsolated asserts FR-014 for both the public package
// and the example command that consumes it, under all four build configurations.
//
// The example is checked alongside the package deliberately. The package's own
// graph could be clean while the program built from it still linked a forbidden
// tree through the stdlib entry points a main package pulls in — and the binary
// is what the size claim is measured on, so the binary's graph is what has to be
// clean.
func TestLoggingSurfaceIsImportIsolated(t *testing.T) {
	t.Parallel()

	surfaces := []string{loggingImportPath, exampleImportPath}

	for _, surface := range surfaces {
		for _, tags := range buildConfigurations() {
			t.Run(surface+"/"+configName(tags), func(t *testing.T) {
				t.Parallel()
				testutil.AssertLoggingSurfaceIsolated(context.Background(), t, surface, tags...)
			})
		}
	}
}

// TestLoggingSurfaceRetainsTraceCorrelationDependencies is the positive half of
// the contract, and it exists because an isolation test alone is satisfiable by
// deleting the feature.
//
// A refactor that dropped trace correlation — removing the OTel trace API — would
// make the binary smaller and every forbidden-import assertion greener, while
// silently destroying the guarantee that every log line correlates with its span.
// Requiring both dependencies to be PRESENT means the isolation gate can only be
// satisfied by a surface that still does its job.
func TestLoggingSurfaceRetainsTraceCorrelationDependencies(t *testing.T) {
	t.Parallel()

	required := []string{zerologImportPath, otelTraceImportPath}

	for _, tags := range buildConfigurations() {
		t.Run(configName(tags), func(t *testing.T) {
			t.Parallel()

			imports, err := testutil.ResolvePackageImports(
				context.Background(),
				testutil.RepoRoot(t),
				loggingImportPath,
				tags...,
			)
			if err != nil {
				t.Fatalf("resolve imports for %s: %v", loggingImportPath, err)
			}

			for _, want := range required {
				if !slices.Contains(imports, want) {
					t.Errorf(
						"%s does not depend on %s under %s: the isolated surface must still "+
							"provide trace-correlated structured logging, not merely a small graph",
						loggingImportPath, want, configName(tags),
					)
				}
			}
		})
	}
}

// TestLoggingSurfaceGraphIsConfigurationIndependent pins the sharper form of
// FR-014: the graph is not merely clean under each configuration, it is the SAME
// under each. Because this surface links none of the trees ax_no_grpc and
// ax_no_otlp decline, any difference between configurations means something
// reachable from logging has started varying by build tag — which is drift worth
// failing on even when both variants happen to be forbidden-import clean.
func TestLoggingSurfaceGraphIsConfigurationIndependent(t *testing.T) {
	root := testutil.RepoRoot(t)

	baseline, err := testutil.ResolvePackageImports(context.Background(), root, loggingImportPath)
	if err != nil {
		t.Fatalf("resolve default imports: %v", err)
	}
	slices.Sort(baseline)

	for _, tags := range buildConfigurations()[1:] {
		t.Run(configName(tags), func(t *testing.T) {
			got, resolveErr := testutil.ResolvePackageImports(context.Background(), root, loggingImportPath, tags...)
			if resolveErr != nil {
				t.Fatalf("resolve imports under %s: %v", configName(tags), resolveErr)
			}
			slices.Sort(got)

			if !slices.Equal(baseline, got) {
				t.Errorf(
					"dependency graph under %s differs from the default build;\n default: %v\n  tagged: %v",
					configName(tags), baseline, got,
				)
			}
		})
	}
}
