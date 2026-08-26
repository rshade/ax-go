package axtest_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

const axtestImportPath = "github.com/rshade/ax-go/axtest"

// buildConfigurations is the exhaustive set of supported build-tag
// combinations, copied verbatim from logging/import_isolation_test.go's
// helper of the same name. Both ax-go constraints are negative, so the
// default configuration passes no tags at all.
func buildConfigurations() [][]string {
	return [][]string{
		nil,
		{"ax_no_grpc"},
		{"ax_no_otlp"},
		{"ax_no_grpc", "ax_no_otlp"},
	}
}

// buildProfiles is the exhaustive set of supported GOOS/GOARCH targets,
// copied verbatim from internal/cmd/surfacecheck.DefaultProfiles. That command
// is a package main and cannot be imported here, so this standing guard keeps
// the same reviewable literal matrix locally.
func buildProfiles() []testutil.BuildProfile {
	return []testutil.BuildProfile{
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "linux", GOARCH: "arm64"},
		{GOOS: "darwin", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "arm64"},
	}
}

func configName(tags []string) string {
	if len(tags) == 0 {
		return "default"
	}
	return strings.Join(tags, ",")
}

// TestAxtestIsOnlyImportedFromTests is the FR-009 standing regression guard:
// like mcp/import_isolation_test.go's TestContractPackagesDoNotImportMCP and
// logging/import_isolation_test.go's TestLoggingSurfaceIsImportIsolated, it
// passes trivially today because nothing imports axtest yet, and stays green
// only as long as no non-test source file anywhere in this module imports it.
//
// Checking only the default (untagged) host build would miss a non-test file
// that imports axtest from behind either an ax_no_grpc/ax_no_otlp constraint
// or a platform suffix such as _windows.go. Both are instances of the "code
// behind a build constraint is invisible to the default toolchain" blind spot
// AGENTS.md warns about.
func TestAxtestIsOnlyImportedFromTests(t *testing.T) {
	for _, tags := range buildConfigurations() {
		for _, profile := range buildProfiles() {
			t.Run(configName(tags)+"/"+profile.String(), func(t *testing.T) {
				testutil.AssertNoProductionImport(
					context.Background(), t, testutil.RepoRoot(t), axtestImportPath, profile, tags...,
				)
			})
		}
	}
}
