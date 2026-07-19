package mcpserver

import (
	"context"
	"net"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
)

// serveHTTPForTest starts the HTTP transport on an ephemeral loopback port via
// the white-box serveOnListener seam and returns the bound address. The server
// is canceled on cleanup and asserted to return nil (clean shutdown).
func serveHTTPForTest(t *testing.T, root *cobra.Command) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	server := newTestServerCtx(t, ctx, root)
	serveErr := make(chan error, 1)
	go func() { serveErr <- serveOnListener(ctx, server, listener) }()

	t.Cleanup(func() {
		cancel()
		// The wait must exceed the engine's graceful HTTP shutdown budget so it
		// reliably observes serveOnListener returning even under -race load.
		select {
		case err := <-serveErr:
			if err != nil {
				t.Errorf("serveOnListener returned non-nil on shutdown: %v", err)
			}
		case <-time.After(httpShutdownBudget + 10*time.Second):
			t.Error("serveOnListener did not return after cancellation")
		}
	})
	return addr
}

func connectHTTP(t *testing.T, addr string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(
		context.Background(),
		// DisableStandaloneSSE avoids a long-lived GET stream that would otherwise
		// keep graceful shutdown waiting for the full budget in tests.
		&sdk.StreamableClientTransport{Endpoint: "http://" + addr, DisableStandaloneSSE: true},
		nil,
	)
	if err != nil {
		t.Fatalf("connect to %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestLoopbackListenerFailsClosed asserts the loopback policy: loopback hosts
// bind, a non-loopback or all-interfaces host fails closed with a validation
// error (exit 2) unless AllowNonLoopback is set, and a malformed address is a
// validation error (FR-016/FR-018, C-15).
func TestLoopbackListenerFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"loopback ip binds", Config{HTTPAddr: "127.0.0.1:0"}, false},
		{"localhost binds", Config{HTTPAddr: "localhost:0"}, false},
		{"all-interfaces fails closed", Config{HTTPAddr: "0.0.0.0:0"}, true},
		{"empty host fails closed", Config{HTTPAddr: ":0"}, true},
		{"malformed address fails", Config{HTTPAddr: "not-an-address"}, true},
		{"non-loopback with opt-in binds", Config{HTTPAddr: "0.0.0.0:0", AllowNonLoopback: true}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := loopbackListener(context.Background(), tc.cfg)
			if listener != nil {
				t.Cleanup(func() { _ = listener.Close() })
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected a validation error, got nil")
				}
				if code := contract.ErrorExitCode(err); code != contract.ExitValidation {
					t.Errorf("exit code = %d, want %d", code, contract.ExitValidation)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if listener == nil {
				t.Fatal("expected a bound listener")
			}
		})
	}
}

// TestHTTPTransportParity asserts the HTTP transport exposes identical
// initialize/list/call behavior to the in-memory (stdio) path (SC-007, C-16).
func TestHTTPTransportParity(t *testing.T) {
	session := connectHTTP(t, serveHTTPForTest(t, dispatchTestRoot()))
	ctx := context.Background()

	if version := session.InitializeResult().ServerInfo.Version; version != testServerVersion {
		t.Errorf("initialize version = %q, want %q", version, testServerVersion)
	}

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	if !names["demo-echo"] {
		t.Errorf("tools/list missing %q over HTTP", "demo-echo")
	}

	call, err := session.CallTool(ctx, &sdk.CallToolParams{
		Name:      "demo-echo",
		Arguments: map[string]any{"name": "over-http"},
	})
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	if call.IsError {
		t.Fatalf("echo over HTTP returned IsError")
	}
	if env := decodeEnvelope(t, resultText(t, call)); env.Data.Name != "over-http" {
		t.Errorf("data.name = %q, want %q", env.Data.Name, "over-http")
	}
}
