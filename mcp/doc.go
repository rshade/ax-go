// Package mcp exposes an ax-go CLI's command tree as a live Model Context
// Protocol (MCP) server with no per-tool work.
//
// It is the deliberately thin public surface over the internal protocol engine
// (internal/mcpserver): Serve runs the server until its context is canceled,
// and NewCommand returns the reserved "mcp-server" Cobra subcommand an adopting
// CLI mounts to expose itself (for example, "mycli mcp-server"). Tools are
// discovered through the same internal/mcp projection (WalkCallableCommands
// and BuildTool) that backs the static "__schema --as=mcp" adapter, so the
// live tool set stays in lock-step with the static schema.
//
// That walk applies three skip rules. A hidden command drops out with its
// whole subtree, as do the reserved __schema, mcp-server, completion, and help
// commands. A command that is not runnable (a pure group with neither Run nor
// RunE) drops out on its own, and its children are still walked. A command
// marked with Exclude also drops out on its own. Every rule lives on the
// command tree, never in server options, so the static and live surfaces
// cannot drift. Excluded commands remain in --help and "__schema --as=ax".
//
// A tools/call dispatches back into the command tree, returning the command's
// verbatim stdout payload on success and the ax.Error envelope on failure,
// while the server keeps serving.
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
