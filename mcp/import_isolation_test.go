package mcp_test

import (
	"context"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

const mcpImportPath = "github.com/rshade/ax-go/mcp"

// TestMCPStaysThin asserts the public mcp package keeps the root runtime facade
// and runtime adapters (telemetry, Loki, OTel exporters/SDK, gRPC, zerolog) out
// of its transitive import graph (contracts/public-api.md C-API-6/7). The
// package MAY import schema, contract, id, internal/mcpserver, the MCP SDK, and
// cobra; the SDK and all protocol mechanics stay behind internal/mcpserver.
func TestMCPStaysThin(t *testing.T) {
	testutil.AssertNoForbiddenImports(
		context.Background(),
		t,
		testutil.RepoRoot(t),
		mcpImportPath,
		testutil.ForbiddenRuntimeImports(),
	)
}

// TestContractPackagesDoNotImportMCP asserts the import-isolated contract
// packages keep the runtime mcp package out of their dependency graphs
// (C-API-6). mcp depends on those packages, never the reverse.
func TestContractPackagesDoNotImportMCP(t *testing.T) {
	forbidden := []testutil.ForbiddenImport{{
		Pattern: mcpImportPath,
		Reason:  "import-isolated contract packages must not import the runtime mcp package",
	}}
	for _, pkg := range []string{
		"github.com/rshade/ax-go/contract",
		"github.com/rshade/ax-go/config",
		"github.com/rshade/ax-go/schema",
		"github.com/rshade/ax-go/id",
	} {
		t.Run(pkg, func(t *testing.T) {
			testutil.AssertNoForbiddenImports(
				context.Background(),
				t,
				testutil.RepoRoot(t),
				pkg,
				forbidden,
			)
		})
	}
}
