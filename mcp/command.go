package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/mcpserver"
)

const (
	transportFlagName    = "transport"
	addrFlagName         = "addr"
	allowNonLoopbackFlag = "allow-non-loopback"
	transportStdioValue  = "stdio"
	transportHTTPValue   = "http"

	exampleCLIPlaceholder = "mycli"
)

// NewCommand returns the reserved "mcp-server" Cobra subcommand an adopting CLI
// mounts to expose itself as an MCP server (for example, "mycli mcp-server").
// It is NOT auto-mounted by ax.Execute; mounting is an explicit opt-in because
// the server executes commands. Running it calls Serve with the resolved
// options, overlaid by the --transport, --addr, and --allow-non-loopback flags.
// The command excludes itself from the tool list it serves.
func NewCommand(root *cobra.Command, opts ...Option) *cobra.Command {
	resolved := resolveOptions(opts)
	cliName := exampleCLIName(root)

	var (
		transport        string
		addr             string
		allowNonLoopback bool
	)

	cmd := &cobra.Command{
		Use:   mcpserver.ServerCommandName,
		Short: "Run this CLI as a live MCP server",
		Long: "Expose this CLI's command tree as a live Model Context Protocol " +
			"(MCP) server. Every visible, runnable command becomes a tool, except " +
			"the reserved __schema, mcp-server, completion, and help commands and " +
			"any command excluded with mcp.Exclude; tools/call runs the command in " +
			"machine mode and returns its payload. Serves over stdio by default, " +
			"or streamable HTTP (loopback unless --allow-non-loopback is set).",
		Example: "  " + cliName + " mcp-server\n" +
			"  " + cliName + " mcp-server --transport=http --addr=127.0.0.1:8080\n" +
			"  " + cliName + " mcp-server --transport=http --addr=0.0.0.0:8080 --allow-non-loopback",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := resolved.config()
			cfg.Stderr = cmd.ErrOrStderr()

			if cmd.Flags().Changed(transportFlagName) {
				parsed, err := parseTransport(cmd.Context(), transport)
				if err != nil {
					return err
				}
				cfg.Transport = parsed
			}
			if cmd.Flags().Changed(addrFlagName) {
				cfg.HTTPAddr = addr
			}
			if cmd.Flags().Changed(allowNonLoopbackFlag) {
				cfg.AllowNonLoopback = allowNonLoopback
			}
			return mcpserver.Serve(cmd.Context(), root, cfg)
		},
	}

	cmd.Flags().StringVar(&transport, transportFlagName, transportStdioValue,
		"MCP transport: stdio or http")
	cmd.Flags().StringVar(&addr, addrFlagName, defaultHTTPAddr,
		"HTTP bind address; loopback by default (used with --transport=http)")
	cmd.Flags().BoolVar(&allowNonLoopback, allowNonLoopbackFlag, false,
		"allow the HTTP transport to bind a non-loopback address (fail-closed without it)")

	return cmd
}

// exampleCLIName returns the adopting CLI's root command name for the help
// example, falling back to a placeholder when root is nil or unnamed.
func exampleCLIName(root *cobra.Command) string {
	if root == nil {
		return exampleCLIPlaceholder
	}
	if name := root.Root().Name(); name != "" {
		return name
	}
	return exampleCLIPlaceholder
}

// parseTransport maps the --transport flag value to an engine transport,
// failing closed with a validation error (exit 2) on an unknown value.
func parseTransport(ctx context.Context, value string) (mcpserver.Transport, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", transportStdioValue:
		return mcpserver.TransportStdio, nil
	case transportHTTPValue:
		return mcpserver.TransportHTTP, nil
	default:
		return 0, contract.NewError(ctx, "validation_error",
			fmt.Sprintf("mcp: unknown transport %q (want stdio or http)", value),
			contract.WithErrorExitCode(contract.ExitValidation))
	}
}
