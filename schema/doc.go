// Package schema provides import-isolated command discoverability contracts.
//
// It reflects Cobra command trees into ax-native schema output and a
// lightweight MCP-compatible adapter shape without importing the root ax
// runtime facade.
//
// DeclarePrompt and DeclareResource attach static MCP prompts (workflow
// templates) and resources (read-only reference metadata) to a command as
// Cobra annotations. The same tree walk that builds __schema projects them, so
// there is one source of truth: CommandSchema carries a command's own
// declarations, and MCPSchema aggregates every non-hidden command's in walk
// order. A prompt name or resource URI declared on two commands makes the
// __schema command fail with validation_error (exit 2); BuildSchema and
// BuildMCPSchema, which cannot return an error, keep the first declaration.
// The live MCP server does not serve declarations yet.
package schema
