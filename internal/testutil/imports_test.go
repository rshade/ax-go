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
