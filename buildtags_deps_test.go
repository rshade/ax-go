package ax

import (
	"context"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

// TestMinimalConfigurationLinksNoForbiddenTree is the dependency half of the
// feature's value claim (FR-015, FR-016, SC-002, SC-008): under
// ax_no_grpc,ax_no_otlp the root facade's transitive graph must contain exactly
// zero packages from the gRPC, protobuf, OTLP-proto, and grpc-gateway trees.
//
// The assertion is an exact count rather than a spot check because "some gRPC
// removed" is not the contract — the linker only drops the ~9 MiB when the
// trees are gone entirely.
func TestMinimalConfigurationLinksNoForbiddenTree(t *testing.T) {
	imports, err := testutil.ResolvePackageImports(
		context.Background(),
		testutil.RepoRoot(t),
		moduleImportPath,
		"ax_no_grpc", "ax_no_otlp",
	)
	if err != nil {
		t.Fatalf("resolve minimal-configuration imports: %v", err)
	}

	violations := testutil.FindForbiddenImports(moduleImportPath, imports, testutil.ForbiddenGRPCTreeImports())
	if len(violations) != 0 {
		t.Errorf(
			"minimal configuration links %d forbidden package(s), want 0:\n%s",
			len(violations),
			testutil.FormatImportViolations(violations),
		)
	}
}

// TestDefaultConfigurationStillLinksForbiddenTree is the control that keeps the
// assertion above honest. An unrecognised build tag is silently ignored by the
// Go toolchain, so a typo in the tag names would make the zero-count assertion
// pass for the wrong reason. This test fails if the default build ever stops
// linking gRPC — at which point the tags are no longer what is doing the work.
func TestDefaultConfigurationStillLinksForbiddenTree(t *testing.T) {
	imports, err := testutil.ResolvePackageImports(context.Background(), testutil.RepoRoot(t), moduleImportPath)
	if err != nil {
		t.Fatalf("resolve default-configuration imports: %v", err)
	}

	violations := testutil.FindForbiddenImports(moduleImportPath, imports, testutil.ForbiddenGRPCTreeImports())
	if len(violations) == 0 {
		t.Fatal("default configuration links zero gRPC-tree packages; " +
			"the minimal-configuration assertion is now vacuous")
	}
}

const moduleImportPath = "github.com/rshade/ax-go"
