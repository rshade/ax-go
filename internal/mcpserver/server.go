package mcpserver

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	internalmcp "github.com/rshade/ax-go/internal/mcp"
	"github.com/rshade/ax-go/schema"
)

// Transport identifies which MCP transport the engine serves over.
type Transport int

const (
	// TransportStdio serves MCP over stdin/stdout using newline-delimited JSON.
	TransportStdio Transport = iota
	// TransportHTTP serves MCP over a streamable HTTP transport.
	TransportHTTP
)

const (
	// ServerCommandName is the reserved subcommand name an adopting CLI mounts
	// to expose itself as an MCP server. It is exported so the public
	// mcp.NewCommand mounts the same name the discovery walk (internal/mcp)
	// excludes, keeping the server from ever listing itself as a callable tool.
	ServerCommandName = "mcp-server"

	// versionDev and versionUnknown are placeholder versions rejected
	// fail-closed at startup so a real build version always reaches the
	// initialize handshake (FR-003, Principle X).
	versionDev     = "dev"
	versionUnknown = "unknown"
)

// Config is the resolved engine configuration handed down from the public mcp
// package. Every field is validated at startup; invalid values fail closed with
// a validation error (exit 2) rather than a panic.
type Config struct {
	// Transport selects the active transport (stdio default, or HTTP).
	Transport Transport
	// HTTPAddr is the bind address used when Transport is TransportHTTP. It
	// defaults to loopback; a non-loopback host requires AllowNonLoopback.
	HTTPAddr string
	// AllowNonLoopback must be true before HTTPAddr may bind a non-loopback
	// interface (FR-016/FR-018).
	AllowNonLoopback bool
	// Version is reported in the MCP initialize handshake; it must be a real
	// build version (never empty/"dev"/"unknown").
	Version string
	// ServerName is the MCP implementation name; defaults to the root command's
	// Name().
	ServerName string
	// Stderr receives server logs and captured command stderr. It defaults to
	// os.Stderr. The protocol channel is never written here (FR-013/FR-014).
	Stderr io.Writer
}

// Serve builds an MCP server from root's command tree and runs it over the
// configured transport until ctx is canceled. It returns nil on clean shutdown
// and a wrapped error on startup or transport failure; an individual tool-call
// failure never returns from Serve (C-API-1).
func Serve(ctx context.Context, root *cobra.Command, cfg Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if root == nil {
		return contract.NewError(ctx, "validation_error", "mcp: root command is required",
			contract.WithErrorExitCode(contract.ExitValidation))
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.ServerName == "" {
		cfg.ServerName = root.Name()
	}
	if err := validateVersion(ctx, cfg.Version); err != nil {
		return err
	}

	dispatch := newDispatcher(ctx, root, cfg)
	server := newMCPServer(dispatch, cfg)

	switch cfg.Transport {
	case TransportStdio:
		return serveStdio(ctx, server)
	case TransportHTTP:
		return serveHTTP(ctx, server, cfg)
	default:
		return contract.NewError(ctx, "validation_error",
			fmt.Sprintf("mcp: unknown transport %d", cfg.Transport),
			contract.WithErrorExitCode(contract.ExitValidation))
	}
}

// validateVersion rejects an empty or placeholder version fail-closed so the
// initialize handshake never advertises "dev"/"unknown" (FR-003, C-1).
func validateVersion(ctx context.Context, version string) error {
	switch strings.TrimSpace(version) {
	case "", versionDev, versionUnknown:
		return contract.NewError(ctx, "validation_error",
			fmt.Sprintf(
				"mcp: server version must be a real build version, got %q "+
					"(inject one via WithVersion or -ldflags)", version),
			contract.WithErrorExitCode(contract.ExitValidation))
	default:
		return nil
	}
}

// newMCPServer constructs the SDK server and registers every discovered tool
// with the shared dispatch handler. The implementation name and version come
// from cfg and surface in the initialize handshake (C-1).
func newMCPServer(dispatch *dispatcher, cfg Config) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{
		Name:    cfg.ServerName,
		Version: cfg.Version,
	}, nil)
	for _, tool := range dispatch.tools {
		server.AddTool(&sdk.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}, dispatch.handle)
	}
	return server
}

// discoverTools projects root's command tree into the callable MCP tool set and
// the name→command target map dispatch uses to resolve calls. The walk shares
// internal/mcp's traversal with the static __schema --as=mcp adapter — hidden
// subtrees pruned wholesale, the reserved __schema, mcp-server, and completion
// commands excluded (D8, FR-004/005/006, INV-3) — so the live and static tool
// sets cannot diverge. Commands that require positional arguments are excluded
// on top (see requiresPositionalArgs).
func discoverTools(root *cobra.Command) ([]schema.MCPTool, map[string]*cobra.Command) {
	var tools []schema.MCPTool
	targets := map[string]*cobra.Command{}
	internalmcp.WalkCallableCommands(root, func(cmd *cobra.Command) {
		if requiresPositionalArgs(cmd) {
			return
		}
		tool := internalmcp.BuildTool(cmd)
		tools = append(tools, schema.MCPTool{
			Name:                   tool.Name,
			Description:            tool.Description,
			InputSchema:            tool.InputSchema,
			NonDeterministicFields: tool.NonDeterministicFields,
		})
		targets[tool.Name] = cmd
	})
	return tools, targets
}

// requiresPositionalArgs reports whether cmd's Cobra Args validator rejects a
// zero-length argument vector. The flat MCP argument object maps only onto
// flags (positional-argument mapping is a deferred open item), so such a
// command could never be satisfied by a tools/call. Excluding it keeps the
// tool list to commands an agent can actually invoke rather than advertising a
// tool that always fails.
func requiresPositionalArgs(cmd *cobra.Command) bool {
	return cmd.Args != nil && cmd.Args(cmd, []string{}) != nil
}
