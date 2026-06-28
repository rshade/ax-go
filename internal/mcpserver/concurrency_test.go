package mcpserver

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// TestConcurrentToolCallsIsolated fires many simultaneous HTTP tool calls, each
// with a distinct argument, and asserts every response carries its own
// argument back — no cross-call flag/state leakage. Run under -race, it also
// guards that the dispatch path shares no mutable command/flag state
// (FR-021, INV-5, C-20).
func TestConcurrentToolCallsIsolated(t *testing.T) {
	addr := serveHTTPForTest(t, dispatchTestRoot())

	const callers = 16
	var wg sync.WaitGroup
	failures := make(chan error, callers)

	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session := connectHTTP(t, addr)
			name := fmt.Sprintf("caller-%d", i)
			res, err := session.CallTool(context.Background(), &sdk.CallToolParams{
				Name:      "demo echo",
				Arguments: map[string]any{"name": name},
			})
			if err != nil {
				failures <- fmt.Errorf("caller %d: %w", i, err)
				return
			}
			if res.IsError {
				failures <- fmt.Errorf("caller %d: unexpected IsError", i)
				return
			}
			env := decodeEnvelope(t, resultText(t, res))
			if env.Data.Name != name {
				failures <- fmt.Errorf("caller %d: got name %q, want %q", i, env.Data.Name, name)
			}
		}(i)
	}

	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}

// blockingRoot builds a tree with a "block" command that signals it is in
// flight (via started) and then blocks until its context is canceled, returning
// the cancellation error. It lets the shutdown test deterministically observe a
// call that is in flight when shutdown begins.
func blockingRoot(started chan<- struct{}) *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "root", RunE: noopRunE}
	block := &cobra.Command{Use: "block", Short: "block until canceled"}
	block.RunE = func(cmd *cobra.Command, _ []string) error {
		close(started)
		<-cmd.Context().Done()
		return cmd.Context().Err()
	}
	root.AddCommand(block)
	return root
}

// TestShutdownDrainsInFlightCalls asserts that on context cancellation the
// server stops serving, in-flight calls observe the canceled context and return
// promptly, and serveOnListener returns nil (FR-020, C-19/C-20).
func TestShutdownDrainsInFlightCalls(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	server := newTestServerCtx(t, ctx, blockingRoot(started))
	serveErr := make(chan error, 1)
	go func() { serveErr <- serveOnListener(ctx, server, listener) }()

	session := connectHTTP(t, addr)

	callResult := make(chan *sdk.CallToolResult, 1)
	go func() {
		res, _ := session.CallTool(context.Background(), &sdk.CallToolParams{Name: "demo block"})
		callResult <- res
	}()

	// Wait until the call is in flight inside the command, then trigger shutdown.
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("block command never started")
	}
	cancel()

	select {
	case res := <-callResult:
		if res != nil && !res.IsError {
			t.Error("in-flight call did not surface the cancellation as an error result")
		}
	case <-time.After(5 * time.Second):
		t.Error("in-flight call did not drain within 5s of shutdown")
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Errorf("serveOnListener returned non-nil on shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("serveOnListener did not return within 5s of cancellation")
	}
}
