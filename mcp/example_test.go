package mcp_test

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/mcp"
)

// ExampleServe runs a CLI's command tree as an MCP server. Serve blocks until
// the context is canceled; this example cancels immediately and confirms the
// clean shutdown (Serve returns nil). A real adopter passes a context canceled
// on SIGINT/SIGTERM and a real build version via WithVersion.
func ExampleServe() {
	root := &cobra.Command{Use: "mycli", Short: "a demo CLI"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // shut down immediately so the example terminates

	err := mcp.Serve(ctx, root,
		mcp.WithTransport(mcp.TransportHTTP),
		mcp.WithHTTPAddr("127.0.0.1:0"),
		mcp.WithVersion("v1.0.0"),
	)
	fmt.Println("clean shutdown:", err == nil)
	// Output: clean shutdown: true
}

// ExampleNewCommand mounts the reserved "mcp-server" subcommand so the CLI can
// expose itself as an MCP server (for example, "mycli mcp-server"). Mounting is
// an explicit opt-in; ax.Execute does not auto-mount it.
func ExampleNewCommand() {
	root := &cobra.Command{Use: "mycli", Short: "a demo CLI"}
	root.AddCommand(mcp.NewCommand(root, mcp.WithVersion("v1.0.0")))

	cmd, _, _ := root.Find([]string{"mcp-server"})
	fmt.Println(cmd.Name())
	// Output: mcp-server
}
