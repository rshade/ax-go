package mcp_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/mcp"
)

// TestServeSubtreeToolsCallRunsTheListedTool mounts mcp.NewCommand on a
// non-root command and drives tools/list and tools/call through mcp.Serve.
// os.Args names that mcp-server command, which is what a subtree server
// re-executes when tools/call dispatches on the mounted command instead of
// the real root. The listed tool must run once, and the server command must
// not run again.
func TestServeSubtreeToolsCallRunsTheListedTool(t *testing.T) {
	var listCalls, serverCalls int

	root := &cobra.Command{Use: "ht"}
	group := &cobra.Command{Use: "location", Args: cobra.MinimumNArgs(1)}
	group.AddCommand(&cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			listCalls++
			_, err := fmt.Fprintln(cmd.OutOrStdout(), `{"ok":true}`)
			return err
		},
	})
	serverCmd := mcp.NewCommand(group, mcp.WithVersion(testVersion))
	serve := serverCmd.RunE
	serverCmd.RunE = func(cmd *cobra.Command, args []string) error {
		serverCalls++
		return serve(cmd, args)
	}
	group.AddCommand(serverCmd)
	root.AddCommand(group)

	setOSArgs(t, []string{"ht", "location", "mcp-server", "--transport", "http", "--addr", "127.0.0.1:1"})

	session := serveTestHTTP(t, group)

	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var toolName string
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		if tool.Name == "ht-location-list" {
			toolName = tool.Name
		}
	}
	if toolName == "" {
		t.Fatalf("tools/list = %v, missing ht-location-list", names)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	call, err := session.CallTool(ctx, &sdk.CallToolParams{Name: toolName})
	if err != nil {
		t.Fatalf("tools/call %s: %v (mcp-server runs=%d, list runs=%d)", toolName, err, serverCalls, listCalls)
	}
	body := contentText(t, call)
	if serverCalls != 0 {
		t.Fatalf("mcp-server ran %d times during tools/call; list ran %d; body %q", serverCalls, listCalls, body)
	}
	if listCalls != 1 {
		t.Fatalf("list ran %d times, want 1; body %q", listCalls, body)
	}
	if call.IsError {
		t.Fatalf("tools/call returned IsError: %s", body)
	}
	if !strings.Contains(body, `{"ok":true}`) {
		t.Fatalf("tools/call body = %q, want the list payload", body)
	}
}

// setOSArgs replaces os.Args until the test ends. A subtree tools/call that
// executes the mounted command instead of the real root makes Cobra parse
// this vector, which names mcp-server, so the test sees the server command
// run again.
func setOSArgs(t *testing.T, args []string) {
	t.Helper()
	previous := os.Args
	os.Args = args //nolint:reassign // Cobra reads os.Args on a non-root Execute; restored below.
	t.Cleanup(func() {
		os.Args = previous //nolint:reassign // restore the process args captured above.
	})
}
