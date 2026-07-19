// Package mcpserver is the internal MCP protocol engine behind the public mcp
// package. It owns the github.com/modelcontextprotocol/go-sdk dependency and
// all protocol, transport, and dispatch mechanics so the public surface stays
// thin and the SDK never leaks past internal/.
//
// The engine reflects an adopting CLI's Cobra command tree into MCP tool
// registrations (sharing internal/mcp's tool projection with the static
// "__schema --as=mcp" adapter, so the live and static tool sets cannot
// diverge), answers the initialize handshake and tools/list, and dispatches
// each tools/call into an isolated, machine-mode invocation of the command
// tree. Command stdout is captured verbatim into the tool result; a non-zero
// exit is mapped to the ax.Error envelope with IsError set, never crashing the
// server. It serves over stdio and a loopback-default streamable HTTP
// transport.
package mcpserver
