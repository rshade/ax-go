package testutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindForbiddenImportsMatchesExactAndPrefix(t *testing.T) {
	forbidden := []ForbiddenImport{
		{
			Pattern: moduleRootImportPath,
			Reason:  "root facade pulls runtime adapters into thin consumers",
		},
		{
			Pattern: "go.opentelemetry.io/otel/exporters/",
			Reason:  "exporters are runtime telemetry adapters",
		},
	}
	imports := []string{
		moduleRootImportPath,
		"go.opentelemetry.io/otel/exporters/stdout/stdouttrace",
		"github.com/rshade/ax-go/contract",
	}

	got := FindForbiddenImports("github.com/rshade/ax-go/schema", imports, forbidden)
	if len(got) != 2 {
		t.Fatalf("FindForbiddenImports returned %d violations, want 2: %#v", len(got), got)
	}
	if got[0].Surface != "github.com/rshade/ax-go/schema" {
		t.Fatalf("Surface = %q, want schema import path", got[0].Surface)
	}
	if got[0].ImportPath != moduleRootImportPath {
		t.Fatalf("first ImportPath = %q, want root facade", got[0].ImportPath)
	}
	if got[1].Pattern != otelExportersPrefix {
		t.Fatalf("second Pattern = %q, want exporter prefix", got[1].Pattern)
	}
}

func TestFindForbiddenImportsIgnoresAllowedPrefixSiblings(t *testing.T) {
	forbidden := []ForbiddenImport{
		{
			Pattern: moduleRootImportPath,
			Reason:  "root facade pulls runtime adapters into thin consumers",
		},
		{
			Pattern: "google.golang.org/grpc",
			Reason:  "grpc is a runtime transport adapter",
		},
	}
	imports := []string{
		"github.com/rshade/ax-go/contract",
		"google.golang.org/grpc/status",
	}

	got := FindForbiddenImports("github.com/rshade/ax-go/config", imports, forbidden)
	if len(got) != 1 {
		t.Fatalf("FindForbiddenImports returned %d violations, want 1: %#v", len(got), got)
	}
	if got[0].ImportPath != "google.golang.org/grpc/status" {
		t.Fatalf("ImportPath = %q, want grpc/status", got[0].ImportPath)
	}
}

func TestFormatImportViolationsNamesSurfaceDependencyAndReason(t *testing.T) {
	violations := []ImportViolation{
		{
			Surface:    "github.com/rshade/ax-go/id",
			ImportPath: zerologImportPath,
			Pattern:    zerologImportPath,
			Reason:     "logger dependency violates thin ID helpers",
		},
	}

	got := FormatImportViolations(violations)
	for _, want := range []string{
		"github.com/rshade/ax-go/id",
		zerologImportPath,
		"logger dependency violates thin ID helpers",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatImportViolations() = %q, want substring %q", got, want)
		}
	}
}

func TestJoinBuildTags(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{name: "no tags omits the flag", tags: nil, want: ""},
		{name: "empty slice omits the flag", tags: []string{}, want: ""},
		{name: "single tag", tags: []string{"ax_no_grpc"}, want: "ax_no_grpc"},
		{
			name: "both tags joined by comma",
			tags: []string{"ax_no_grpc", "ax_no_otlp"},
			want: "ax_no_grpc,ax_no_otlp",
		},
		{name: "empty entries dropped", tags: []string{"", "ax_no_otlp", ""}, want: "ax_no_otlp"},
		{name: "only empty entries omits the flag", tags: []string{"", ""}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinBuildTags(tc.tags); got != tc.want {
				t.Fatalf("joinBuildTags(%#v) = %q, want %q", tc.tags, got, tc.want)
			}
		})
	}
}

// TestResolvePackageImportsAppliesBuildTags asserts the tags actually reach the
// go list invocation, by observing the resolved graph rather than the argv: a
// fixture package splits its import between a default file and a tag-gated
// sibling, exactly the shape ax_no_grpc and ax_no_otlp use.
func TestResolvePackageImportsAppliesBuildTags(t *testing.T) {
	dir := newTaggedFixtureModule(t)

	tests := []struct {
		name       string
		tags       []string
		wantImport string
		denyImport string
	}{
		{
			name:       "omitted tags resolve the default file",
			tags:       nil,
			wantImport: "net/http",
			denyImport: "text/tabwriter",
		},
		{
			name:       "supplied tag resolves the gated sibling",
			tags:       []string{"fixture_no_http"},
			wantImport: "text/tabwriter",
			denyImport: "net/http",
		},
		{
			name:       "empty tag behaves as omitted",
			tags:       []string{""},
			wantImport: "net/http",
			denyImport: "text/tabwriter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			imports, err := ResolvePackageImports(context.Background(), dir, "./thin", tc.tags...)
			if err != nil {
				t.Fatalf("ResolvePackageImports(tags=%v): %v", tc.tags, err)
			}
			if !containsImport(imports, tc.wantImport) {
				t.Fatalf("resolved graph (%d packages) is missing %q", len(imports), tc.wantImport)
			}
			if containsImport(imports, tc.denyImport) {
				t.Fatalf("resolved graph (%d packages) unexpectedly contains %q", len(imports), tc.denyImport)
			}
		})
	}
}

func TestAssertNoForbiddenImportsAppliesBuildTags(t *testing.T) {
	dir := newTaggedFixtureModule(t)
	forbidden := []ForbiddenImport{{
		Pattern: "net/http",
		Reason:  "HTTP runtime dependency is forbidden for this surface",
	}}

	spy := &tbSpy{T: t}
	AssertNoForbiddenImports(context.Background(), spy, dir, "./thin", forbidden)
	if !spy.errorfCalled {
		t.Fatal("untagged assertion did not report the default build's net/http dependency")
	}

	tagged := &tbSpy{T: t}
	AssertNoForbiddenImports(context.Background(), tagged, dir, "./thin", forbidden, "fixture_no_http")
	if tagged.errorfCalled {
		t.Fatalf("tagged assertion reported a violation: %v", tagged.messages)
	}
}

// TestForbiddenGRPCTreeImportsMatchesSubpackages pins that each pattern covers
// its whole subtree. A rule that only matched the tree root would let
// google.golang.org/grpc/status slip through the minimal configuration's
// zero-dependency guarantee.
func TestForbiddenGRPCTreeImportsMatchesSubpackages(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		wantRoot   string
	}{
		{
			name:       "grpc subpackage",
			importPath: "google.golang.org/grpc/status",
			wantRoot:   "google.golang.org/grpc",
		},
		{
			name:       "grpc tree root",
			importPath: "google.golang.org/grpc",
			wantRoot:   "google.golang.org/grpc",
		},
		{
			name:       "protobuf subpackage",
			importPath: "google.golang.org/protobuf/proto",
			wantRoot:   "google.golang.org/protobuf",
		},
		{
			name:       "otlp proto subpackage",
			importPath: "go.opentelemetry.io/proto/otlp/collector/trace/v1",
			wantRoot:   "go.opentelemetry.io/proto/otlp",
		},
		{
			name:       "grpc gateway subpackage",
			importPath: "github.com/grpc-ecosystem/grpc-gateway/v2/runtime",
			wantRoot:   "github.com/grpc-ecosystem/grpc-gateway/v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindForbiddenImports("github.com/rshade/ax-go", []string{tc.importPath}, ForbiddenGRPCTreeImports())
			if len(got) != 1 {
				t.Fatalf("FindForbiddenImports(%q) returned %d violations, want 1: %#v", tc.importPath, len(got), got)
			}
			if got[0].Pattern != tc.wantRoot {
				t.Fatalf("Pattern = %q, want %q", got[0].Pattern, tc.wantRoot)
			}
		})
	}
}

// TestForbiddenGRPCTreeImportsAllowsStdoutTrace is the reason this rule set is
// distinct from ForbiddenRuntimeImports: that one forbids the whole
// go.opentelemetry.io/otel/exporters/ prefix, which would flag stdouttrace —
// legitimately linked in the declined build to serve AX_OTEL_DEBUG.
func TestForbiddenGRPCTreeImportsAllowsStdoutTrace(t *testing.T) {
	allowed := []string{
		"go.opentelemetry.io/otel/exporters/stdout/stdouttrace",
		"go.opentelemetry.io/otel/sdk/trace",
		"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
	}

	if got := FindForbiddenImports("github.com/rshade/ax-go", allowed, ForbiddenGRPCTreeImports()); len(got) != 0 {
		t.Fatalf("FindForbiddenImports flagged permitted dependencies: %#v", got)
	}
}

// TestForbiddenGRPCTreeImportsNamesOffendingDependency proves the boundary
// bites: a minimal-configuration import list that has regained a gRPC package
// produces a violation naming the dependency, its pattern, and the reason.
func TestForbiddenGRPCTreeImportsNamesOffendingDependency(t *testing.T) {
	imports := []string{
		"context",
		"github.com/rshade/ax-go/contract",
		"go.opentelemetry.io/otel/exporters/stdout/stdouttrace",
		"google.golang.org/grpc/status",
	}

	violations := FindForbiddenImports("github.com/rshade/ax-go", imports, ForbiddenGRPCTreeImports())
	if len(violations) != 1 {
		t.Fatalf("FindForbiddenImports returned %d violations, want 1: %#v", len(violations), violations)
	}
	if violations[0].ImportPath != "google.golang.org/grpc/status" {
		t.Fatalf("ImportPath = %q, want google.golang.org/grpc/status", violations[0].ImportPath)
	}

	message := FormatImportViolations(violations)
	for _, want := range []string{
		"google.golang.org/grpc/status",
		"google.golang.org/grpc",
		"ax_no_grpc,ax_no_otlp",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("FormatImportViolations() = %q, want substring %q", message, want)
		}
	}
}

// TestForbiddenLoggingImportsPermitsTheLoggingBackend is the reason the logging
// surface needs its own rule set rather than reusing ForbiddenRuntimeImports.
// The four contract packages (contract, config, schema, id) forbid zerolog
// outright — logging is a runtime concern they must not carry. The logging
// surface is the opposite case: zerolog appears in Logger's exported method set
// and the OTel trace API is what makes trace correlation possible, so both are
// required, and flagging either would make the isolated surface unbuildable.
func TestForbiddenLoggingImportsPermitsTheLoggingBackend(t *testing.T) {
	required := []string{
		"github.com/rs/zerolog",
		"go.opentelemetry.io/otel/trace",
		"github.com/rshade/ax-go/contract",
		"github.com/rshade/ax-go/internal/logcore",
		"context",
		"io",
		"os",
	}

	if got := FindForbiddenImports(
		"github.com/rshade/ax-go/logging",
		required,
		ForbiddenLoggingImports(),
	); len(got) != 0 {
		t.Fatalf("ForbiddenLoggingImports flagged required dependencies: %#v", got)
	}
}

// TestForbiddenRuntimeImportsStillForbidsZerolog pins the complement of the case
// above: adding a per-surface rule set must not relax the four existing contract
// packages. If this ever passes, ForbiddenRuntimeImports was edited when
// ForbiddenLoggingImports should have been.
func TestForbiddenRuntimeImportsStillForbidsZerolog(t *testing.T) {
	got := FindForbiddenImports(
		"github.com/rshade/ax-go/contract",
		[]string{"github.com/rs/zerolog"},
		ForbiddenRuntimeImports(),
	)
	if len(got) != 1 {
		t.Fatalf("ForbiddenRuntimeImports returned %d violations for zerolog, want 1: %#v", len(got), got)
	}
}

// TestForbiddenLoggingImportsFlagsEveryExcludedTree walks the contract table one
// dependency at a time so a missing rule fails loudly and names itself, rather
// than being absorbed into a single "some import was flagged" assertion. net/http
// and crypto/tls are the two entries absent from the contract-package list and
// the largest single size lever, which is why they are enumerated explicitly.
func TestForbiddenLoggingImportsFlagsEveryExcludedTree(t *testing.T) {
	tests := []struct {
		name       string
		importPath string
		wantRoot   string
	}{
		{
			name:       "root facade",
			importPath: "github.com/rshade/ax-go",
			wantRoot:   "github.com/rshade/ax-go",
		},
		{
			name:       "telemetry setup",
			importPath: "github.com/rshade/ax-go/internal/telemetry",
			wantRoot:   "github.com/rshade/ax-go/internal/telemetry",
		},
		{
			name:       "mcp server runtime",
			importPath: "github.com/rshade/ax-go/internal/mcpserver",
			wantRoot:   "github.com/rshade/ax-go/internal/mcpserver",
		},
		{
			name:       "otel sdk subpackage",
			importPath: "go.opentelemetry.io/otel/sdk/trace",
			wantRoot:   "go.opentelemetry.io/otel/sdk",
		},
		{
			name:       "otel exporter",
			importPath: "go.opentelemetry.io/otel/exporters/otlp/otlptrace",
			wantRoot:   "go.opentelemetry.io/otel/exporters/",
		},
		{
			name:       "otelhttp instrumentation",
			importPath: "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
			wantRoot:   "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
		},
		{
			name:       "otelgrpc instrumentation",
			importPath: "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc",
			wantRoot:   "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc",
		},
		{
			name:       "grpc subpackage",
			importPath: "google.golang.org/grpc/status",
			wantRoot:   "google.golang.org/grpc",
		},
		{
			name:       "protobuf subpackage",
			importPath: "google.golang.org/protobuf/proto",
			wantRoot:   "google.golang.org/protobuf",
		},
		{
			name:       "cobra",
			importPath: "github.com/spf13/cobra",
			wantRoot:   "github.com/spf13/cobra",
		},
		{
			name:       "net/http",
			importPath: "net/http",
			wantRoot:   "net/http",
		},
		{
			name:       "crypto/tls",
			importPath: "crypto/tls",
			wantRoot:   "crypto/tls",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindForbiddenImports(
				"github.com/rshade/ax-go/logging",
				[]string{tc.importPath},
				ForbiddenLoggingImports(),
			)
			if len(got) != 1 {
				t.Fatalf("FindForbiddenImports(%q) returned %d violations, want 1: %#v", tc.importPath, len(got), got)
			}
			if got[0].Pattern != tc.wantRoot {
				t.Fatalf("Pattern = %q, want %q", got[0].Pattern, tc.wantRoot)
			}
		})
	}
}

// TestForbiddenLoggingImportsDoesNotFlagHTTPLookalikes guards the net/http rule
// against over-matching. net/http/httptest is genuinely forbidden (it pulls the
// same tree), but net/textproto and net/url are not — a rule written as a bare
// "net/" prefix would flag them and send a maintainer chasing a phantom.
func TestForbiddenLoggingImportsDoesNotFlagHTTPLookalikes(t *testing.T) {
	allowed := []string{"net/url", "net/textproto", "net"}

	if got := FindForbiddenImports(
		"github.com/rshade/ax-go/logging",
		allowed,
		ForbiddenLoggingImports(),
	); len(got) != 0 {
		t.Fatalf("ForbiddenLoggingImports flagged permitted stdlib packages: %#v", got)
	}
}

// newTaggedFixtureModule writes a throwaway module whose single package splits
// its import across a default file and a fixture_no_http-gated sibling.
func newTaggedFixtureModule(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "go.mod"),
		[]byte("module example.com/tagged\n\ngo 1.26.4\n"),
		0o600,
	); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	pkgDir := filepath.Join(dir, "thin")
	if err := os.Mkdir(pkgDir, 0o700); err != nil {
		t.Fatalf("mkdir thin package: %v", err)
	}
	files := map[string]string{
		"thin.go":          "//go:build !fixture_no_http\n\npackage thin\n\nimport _ \"net/http\"\n",
		"thin_disabled.go": "//go:build fixture_no_http\n\npackage thin\n\nimport _ \"text/tabwriter\"\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(pkgDir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func containsImport(imports []string, want string) bool {
	for _, importPath := range imports {
		if importPath == want {
			return true
		}
	}
	return false
}

func TestAssertNoForbiddenImportsReportsSurfaceDependencyAndReason(t *testing.T) {
	dir := t.TempDir()
	goModPath := filepath.Join(dir, "go.mod")
	goMod := []byte("module example.com/boundary\n\ngo 1.26.4\n")
	if err := os.WriteFile(goModPath, goMod, 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	pkgDir := filepath.Join(dir, "thin")
	if err := os.Mkdir(pkgDir, 0o700); err != nil {
		t.Fatalf("mkdir thin package: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(pkgDir, "thin.go"),
		[]byte("package thin\n\nimport _ \"net/http\"\n"),
		0o600,
	); err != nil {
		t.Fatalf("write thin package: %v", err)
	}

	spy := &tbSpy{T: t}
	AssertNoForbiddenImports(
		context.Background(),
		spy,
		dir,
		"./thin",
		[]ForbiddenImport{{
			Pattern: "net/http",
			Reason:  "HTTP runtime dependency is forbidden for this surface",
		}},
	)
	if !spy.errorfCalled {
		t.Fatal("AssertNoForbiddenImports did not report a forbidden dependency")
	}
	got := strings.Join(spy.messages, "\n")
	for _, want := range []string{"./thin", "net/http", "HTTP runtime dependency is forbidden"} {
		if !strings.Contains(got, want) {
			t.Fatalf("AssertNoForbiddenImports message = %q, want substring %q", got, want)
		}
	}
}
