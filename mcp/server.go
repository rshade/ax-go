package mcp

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/mcpserver"
)

// Serve runs an ax-go CLI's command tree as a live MCP server until ctx is
// canceled, returning a non-nil error only on startup or transport failure
// (never on an individual tool-call failure). Stream separation is preserved:
// MCP protocol I/O uses the transport channel; logs and diagnostics go to
// stderr.
//
// Tools are discovered from schema.BuildMCPSchema(root), excluding hidden
// commands and the reserved __schema and mcp-server commands. A tools/call
// dispatches into an isolated, machine-mode invocation of the command tree:
// success returns the command's verbatim stdout payload and a non-zero exit
// returns the ax.Error envelope with IsError set. Invalid options (for example
// a non-loopback HTTP address without WithAllowNonLoopback, or an empty or
// placeholder version) fail closed at startup with a validation error (exit 2).
func Serve(ctx context.Context, root *cobra.Command, opts ...Option) error {
	return mcpserver.Serve(ctx, root, resolveOptions(opts).config())
}

// config maps the resolved public options onto the internal engine
// configuration. ServerName and Stderr are left zero so the engine applies its
// defaults (root.Name() and os.Stderr).
func (o options) config() mcpserver.Config {
	cfg := mcpserver.Config{
		HTTPAddr:         o.httpAddr,
		AllowNonLoopback: o.allowNonLoopback,
		Version:          o.version,
	}
	switch o.transport {
	case TransportHTTP:
		cfg.Transport = mcpserver.TransportHTTP
	case TransportStdio:
		cfg.Transport = mcpserver.TransportStdio
	default:
		cfg.Transport = mcpserver.TransportStdio
	}
	return cfg
}
