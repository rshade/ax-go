package mcp_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/mcp"
)

const testVersion = "v1.2.3-test"

type echoPayload struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

// newDemoRoot builds an ax-style command tree used to exercise the public
// server end-to-end: a root, an "echo" leaf that returns a machine envelope,
// and a "fail" leaf that returns an ax.Error envelope (exit 2).
func newDemoRoot() *cobra.Command {
	root := &cobra.Command{Use: "demo", Short: "demo root", RunE: func(cmd *cobra.Command, _ []string) error {
		return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), echoPayload{Name: "root"}))
	}}

	var name string
	echo := &cobra.Command{Use: "echo", Short: "echo a name", RunE: func(cmd *cobra.Command, _ []string) error {
		mode, _ := ax.ModeFromContext(cmd.Context())
		return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), echoPayload{
			Name: name,
			Mode: mode.String(),
		}))
	}}
	echo.Flags().StringVar(&name, "name", "anon", "name to echo")
	root.AddCommand(echo)

	fail := &cobra.Command{Use: "fail", Short: "always fails", RunE: func(cmd *cobra.Command, _ []string) error {
		return ax.NewError(cmd.Context(), "demo_failure", "intentional failure",
			ax.WithErrorExitCode(ax.ExitValidation))
	}}
	root.AddCommand(fail)

	return root
}

// serveTestHTTP starts the public mcp.Serve over a loopback HTTP transport on a
// free port and returns a connected client session. The server is canceled and
// asserted to return nil on cleanup (clean shutdown, C-API-1/C-19). Driving the
// public Serve over real loopback HTTP is the in-process way to exercise the
// public entry point end-to-end; StdioTransport binds the process's own
// stdin/stdout and cannot be driven from within a test.
func serveTestHTTP(t *testing.T, root *cobra.Command, opts ...mcp.Option) *sdk.ClientSession {
	t.Helper()

	addr := freeLoopbackAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	allOpts := append([]mcp.Option{
		mcp.WithTransport(mcp.TransportHTTP),
		mcp.WithHTTPAddr(addr),
		mcp.WithVersion(testVersion),
	}, opts...)
	go func() { serveErr <- mcp.Serve(ctx, root, allOpts...) }()

	waitForListener(t, addr)

	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             "http://" + addr,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		cancel()
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		select {
		case err := <-serveErr:
			if err != nil {
				t.Errorf("Serve returned non-nil on shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return within 5s of cancellation")
		}
	})
	return session
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

func waitForListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server did not start listening on %s", addr)
}

// TestServeInitializeAndToolsList drives initialize and tools/list through the
// public Serve, asserting the handshake reports the injected version and the
// reserved mcp-server command is excluded from its own tool list (FR-003,
// C-API-1/2, C-1/C-2).
func TestServeInitializeAndToolsList(t *testing.T) {
	session := serveTestHTTP(t, newDemoRoot())

	init := session.InitializeResult()
	if init.ServerInfo == nil || init.ServerInfo.Version != testVersion {
		t.Fatalf("initialize version = %+v, want %q", init.ServerInfo, testVersion)
	}
	if init.ServerInfo.Name != "demo" {
		t.Errorf("server name = %q, want %q", init.ServerInfo.Name, "demo")
	}

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"demo", "demo-echo", "demo-fail"} {
		if !got[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}
}

// TestServeToolsCall drives tools/call through the public server: a successful
// call returns the command's verbatim machine payload (machine mode forced); a
// failing command returns IsError carrying the ax.Error envelope; and the
// server keeps serving the next call (FR-008/009/010, C-6/C-7/C-9).
func TestServeToolsCall(t *testing.T) {
	session := serveTestHTTP(t, newDemoRoot())
	ctx := context.Background()

	ok, err := session.CallTool(ctx, &sdk.CallToolParams{
		Name:      "demo-echo",
		Arguments: map[string]any{"name": "Ada"},
	})
	if err != nil {
		t.Fatalf("tools/call echo: %v", err)
	}
	if ok.IsError {
		t.Fatalf("echo returned IsError: %s", contentText(t, ok))
	}
	var env struct {
		Data echoPayload `json:"data"`
	}
	mustUnmarshal(t, contentText(t, ok), &env)
	if env.Data.Name != "Ada" {
		t.Errorf("data.name = %q, want %q", env.Data.Name, "Ada")
	}
	if env.Data.Mode != "json" {
		t.Errorf("data.mode = %q, want json (machine mode forced)", env.Data.Mode)
	}

	failed, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "demo-fail"})
	if err != nil {
		t.Fatalf("tools/call fail: %v", err)
	}
	if !failed.IsError {
		t.Fatal("fail did not return IsError")
	}
	var envelope struct {
		ErrorCode string `json:"error_code"`
	}
	mustUnmarshal(t, contentText(t, failed), &envelope)
	if envelope.ErrorCode != "demo_failure" {
		t.Errorf("error_code = %q, want %q", envelope.ErrorCode, "demo_failure")
	}

	// The server keeps serving after a tool failure.
	again, err := session.CallTool(ctx, &sdk.CallToolParams{
		Name:      "demo-echo",
		Arguments: map[string]any{"name": "still-alive"},
	})
	if err != nil {
		t.Fatalf("tools/call after failure: %v", err)
	}
	if again.IsError {
		t.Fatalf("server stopped serving after a failure: %s", contentText(t, again))
	}
}

func contentText(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content block, got %d", len(res.Content))
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *mcp.TextContent", res.Content[0])
	}
	return text.Text
}

func mustUnmarshal(t *testing.T, payload string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(payload), dst); err != nil {
		t.Fatalf("unmarshal %q: %v", payload, err)
	}
}

// TestServeRejectsPlaceholderVersion asserts an empty or placeholder version
// fails closed at startup with a validation error (exit 2), never reaching the
// handshake (FR-003, C-1, C-API-3).
func TestServeRejectsPlaceholderVersion(t *testing.T) {
	for _, version := range []string{"", "dev", "unknown"} {
		t.Run(version, func(t *testing.T) {
			err := mcp.Serve(context.Background(), newDemoRoot(), mcp.WithVersion(version))
			if err == nil {
				t.Fatal("expected a startup validation error, got nil")
			}
			if code := ax.ErrorExitCode(err); code != ax.ExitValidation {
				t.Errorf("exit code = %d, want %d", code, ax.ExitValidation)
			}
		})
	}
}
