// Package mcp exposes an ax-go CLI's command tree as a live Model Context
// Protocol (MCP) server with no per-tool work.
//
// It is the deliberately thin public surface over the internal protocol engine
// (internal/mcpserver): Serve runs the server until its context is canceled,
// and NewCommand returns the reserved "mcp-server" Cobra subcommand an adopting
// CLI mounts to expose itself (for example, "mycli mcp-server"). Tools are
// discovered from the same schema.BuildMCPSchema projection that backs
// "__schema --as=mcp", so the live tool set stays in lock-step with the static
// schema. A tools/call dispatches back into the command tree, returning the
// command's verbatim stdout payload on success and the ax.Error envelope on
// failure, while the server keeps serving.
//
// The server runs over stdio (the default) or a streamable HTTP transport that
// binds loopback by default and fails closed against accidental public exposure
// unless WithAllowNonLoopback is set. Stream separation is preserved: MCP
// protocol I/O uses the transport channel; logs and diagnostics go to stderr.
//
// Root ax gains no new exported symbols: all protocol, transport, and dispatch
// mechanics live behind internal/mcpserver, and the MCP Go SDK dependency is
// confined there.
package mcp
