package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	moduleRootImportPath = "github.com/rshade/ax-go"
	otelExportersPrefix  = "go.opentelemetry.io/otel/exporters/"
	zerologImportPath    = "github.com/rs/zerolog"
	grpcImportPath       = "google.golang.org/grpc"
)

// ForbiddenImport describes an import path that a public contract surface must
// not include in its transitive dependency graph. Patterns match exactly by
// default and also match subpackages, except moduleRootImportPath is exact-only
// so isolated sibling packages remain importable by each other.
type ForbiddenImport struct {
	Pattern string
	Reason  string
}

// ImportViolation reports one forbidden dependency found in a package's
// transitive import graph.
type ImportViolation struct {
	Surface    string
	ImportPath string
	Pattern    string
	Reason     string
}

// ResolvePackageImports returns the transitive package imports for importPath
// by running `go list -deps` from moduleDir.
//
// The optional tags are the build constraints to resolve the graph under. When
// none are supplied the argv is byte-identical to the untagged form, so the
// default build's dependency graph is what gets inspected. Supplying tags is
// what lets a caller assert the dependency boundary of a declined
// configuration (for example ax_no_grpc,ax_no_otlp), which is invisible to an
// untagged `go list`.
func ResolvePackageImports(ctx context.Context, moduleDir, importPath string, tags ...string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	args := []string{"list", "-deps", "-f", "{{.ImportPath}}"}
	if joined := joinBuildTags(tags); joined != "" {
		args = append(args, "-tags", joined)
	}
	args = append(args, importPath)

	// #nosec G204 -- importPath is a package path supplied by repository tests,
	// not shell-interpreted input; exec.CommandContext passes it as argv.
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = moduleDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list dependencies for %s: %w: %s", importPath, err, strings.TrimSpace(string(out)))
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	imports := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		imports = append(imports, line)
	}
	return imports, nil
}

// FindForbiddenImports returns every forbidden dependency found in imports.
func FindForbiddenImports(surface string, imports []string, forbidden []ForbiddenImport) []ImportViolation {
	var violations []ImportViolation
	for _, importPath := range imports {
		for _, rule := range forbidden {
			if !matchesForbiddenImport(importPath, rule.Pattern) {
				continue
			}
			violations = append(violations, ImportViolation{
				Surface:    surface,
				ImportPath: importPath,
				Pattern:    rule.Pattern,
				Reason:     rule.Reason,
			})
		}
	}
	return violations
}

// FormatImportViolations formats dependency-boundary violations for test
// failures. The message names the public surface, dependency, pattern, and
// reason so the maintainer can remove the accidental runtime import quickly.
func FormatImportViolations(violations []ImportViolation) string {
	if len(violations) == 0 {
		return ""
	}

	var b strings.Builder
	for _, violation := range violations {
		fmt.Fprintf(
			&b,
			"%s imports forbidden dependency %s (matched %s): %s\n",
			violation.Surface,
			violation.ImportPath,
			violation.Pattern,
			violation.Reason,
		)
	}
	return strings.TrimRight(b.String(), "\n")
}

// joinBuildTags renders tags as a single comma-separated `-tags` value,
// discarding empty entries so a caller passing a zero-value string never
// produces a trailing comma. It returns "" when no usable tag remains, which is
// the signal to omit the flag entirely.
func joinBuildTags(tags []string) string {
	usable := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != "" {
			usable = append(usable, tag)
		}
	}
	return strings.Join(usable, ",")
}

// AssertNoForbiddenImports fails t when importPath's dependency graph contains
// a forbidden runtime dependency. The optional tags select the build
// configuration whose graph is inspected; omitting them inspects the default
// build.
func AssertNoForbiddenImports(
	ctx context.Context,
	t testing.TB,
	moduleDir string,
	importPath string,
	forbidden []ForbiddenImport,
	tags ...string,
) {
	t.Helper()

	imports, err := ResolvePackageImports(ctx, moduleDir, importPath, tags...)
	if err != nil {
		t.Fatalf("resolve imports for %s: %v", importPath, err)
	}
	violations := FindForbiddenImports(importPath, imports, forbidden)
	if len(violations) > 0 {
		t.Errorf("import isolation failed:\n%s", FormatImportViolations(violations))
	}
}

// ForbiddenRuntimeImports returns the canonical set of runtime dependencies
// that every public contract package (contract, config, schema, id) must keep
// out of its transitive import graph. The list is constitution-defined and
// identical for all isolated surfaces, so it lives here once instead of being
// copied into each package's test.
func ForbiddenRuntimeImports() []ForbiddenImport {
	return []ForbiddenImport{
		{
			Pattern: moduleRootImportPath,
			Reason:  "root runtime facade must not be imported by isolated contract packages",
		},
		{
			Pattern: "github.com/rshade/ax-go/internal/telemetry",
			Reason:  "telemetry setup is a root runtime responsibility",
		},
		{
			Pattern: "github.com/rshade/ax-go/internal/loki",
			Reason:  "Loki direct push is a runtime logging adapter",
		},
		{
			Pattern: otelExportersPrefix,
			Reason:  "OTel exporters are runtime adapters",
		},
		{
			Pattern: "go.opentelemetry.io/otel/sdk",
			Reason:  "OTel SDK setup is a root runtime responsibility",
		},
		{
			Pattern: "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
			Reason:  "HTTP instrumentation belongs to root transport helpers",
		},
		{
			Pattern: "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc",
			Reason:  "gRPC instrumentation belongs to root transport helpers",
		},
		{
			Pattern: grpcImportPath,
			Reason:  "gRPC transport helpers are runtime adapters",
		},
		{
			Pattern: zerologImportPath,
			Reason:  "logging is a root runtime concern",
		},
	}
}

// ForbiddenGRPCTreeImports returns the dependency trees that a root-facade
// consumer building with both ax_no_grpc and ax_no_otlp must link exactly zero
// packages from. Each pattern covers its whole subtree, because
// matchesForbiddenImport prefix-matches.
//
// This is deliberately a distinct rule set rather than a reuse of
// ForbiddenRuntimeImports: that one forbids the entire
// go.opentelemetry.io/otel/exporters/ prefix, which would wrongly flag
// stdouttrace — legitimately present in the declined build, where it backs the
// AX_OTEL_DEBUG local-span path.
//
// The guarantee holds for the minimal configuration only. Declining just one of
// the two tags leaves the gRPC subtree reachable through the other root, by
// design.
func ForbiddenGRPCTreeImports() []ForbiddenImport {
	return []ForbiddenImport{
		{
			Pattern: grpcImportPath,
			Reason:  "gRPC runtime must not link under ax_no_grpc,ax_no_otlp",
		},
		{
			Pattern: "google.golang.org/protobuf",
			Reason:  "protobuf runtime and reflection tables must not link under ax_no_grpc,ax_no_otlp",
		},
		{
			Pattern: "go.opentelemetry.io/proto/otlp",
			Reason:  "OTLP wire definitions must not link under ax_no_grpc,ax_no_otlp",
		},
		{
			Pattern: "github.com/grpc-ecosystem/grpc-gateway/v2",
			Reason:  "generated gateway stubs must not link under ax_no_grpc,ax_no_otlp",
		},
	}
}

// AssertContractPackageIsolated fails t when importPath's transitive dependency
// graph contains any canonical forbidden runtime dependency. importPath is a
// fully qualified package path, which go list resolves against the module that
// contains the test's working directory.
func AssertContractPackageIsolated(ctx context.Context, t testing.TB, importPath string) {
	t.Helper()
	AssertNoForbiddenImports(ctx, t, RepoRoot(t), importPath, ForbiddenRuntimeImports())
}

// RepoRoot returns the module root directory by walking up from the test's
// working directory until it finds go.mod.
func RepoRoot(t testing.TB) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above working directory")
		}
		dir = parent
	}
}

func matchesForbiddenImport(importPath, pattern string) bool {
	if importPath == pattern {
		return true
	}
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(importPath, pattern)
	}
	if pattern == moduleRootImportPath {
		return false
	}
	return strings.HasPrefix(importPath, pattern+"/")
}
