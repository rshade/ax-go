package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rshade/ax-go/mcp"
)

// TestLiveServerServesDeclaredAgentContext drives the example's real command
// tree through the public mcp.Serve and asserts the agent-facing surface a
// client such as Claude Code relies on: instructions at initialize, a rendered
// prompt, and a readable resource (feature 030).
func TestLiveServerServesDeclaredAgentContext(t *testing.T) {
	root, _ := newRootCommand(
		strings.NewReader(""),
		goldenVersion,
		func() (string, error) { return deterministicEntityID, nil },
	)
	if err := declareAgentContext(root); err != nil {
		t.Fatalf("declareAgentContext: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- mcp.Serve(ctx, root,
			mcp.WithTransport(mcp.TransportHTTP),
			mcp.WithHTTPAddr(addr),
			mcp.WithVersion(goldenVersion),
			mcp.WithInstructions(agentInstructions),
		)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-serveErr:
			if err != nil {
				t.Errorf("Serve returned %v on shutdown", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Serve did not return after cancellation")
		}
	})

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0-test"}, nil)
	var session *sdk.ClientSession
	for deadline := time.Now().Add(5 * time.Second); ; {
		session, err = client.Connect(ctx, &sdk.StreamableClientTransport{
			Endpoint: "http://" + addr, DisableStandaloneSSE: true,
		}, nil)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("connect: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Cleanup(func() { _ = session.Close() })

	if got := session.InitializeResult().Instructions; got != agentInstructions {
		t.Fatalf("instructions = %q, want %q", got, agentInstructions)
	}

	rendered, err := session.GetPrompt(ctx, &sdk.GetPromptParams{
		Name: "greet-then-stream", Arguments: map[string]string{"name": "Ada"},
	})
	if err != nil {
		t.Fatalf("prompts/get: %v", err)
	}
	text, ok := rendered.Messages[0].Content.(*sdk.TextContent)
	if !ok || !strings.Contains(text.Text, "--name Ada") || strings.Contains(text.Text, "{{") {
		t.Fatalf(
			"rendered prompt = %+v, want the name substituted and no placeholders left",
			rendered.Messages[0].Content,
		)
	}

	read, err := session.ReadResource(ctx, &sdk.ReadResourceParams{URI: "ax-integration://docs/exit-codes"})
	if err != nil {
		t.Fatalf("resources/read: %v", err)
	}
	if len(read.Contents) != 1 || read.Contents[0].Text != exitCodesDoc || read.Contents[0].MIMEType != "text/plain" {
		t.Fatalf("resources/read = %+v, want the declared exit-codes document", read.Contents)
	}
}
