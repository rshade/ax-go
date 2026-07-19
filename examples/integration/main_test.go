package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/mcp"
)

func TestRunDefaultCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(
		context.Background(),
		[]string{"--format=json", "--idempotency-key=test-key", "--name=Ada"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}

	var got ax.Envelope[helloPayload]
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not a JSON envelope: %v", err)
	}
	if got.Data.Greeting != "hello" {
		t.Fatalf("greeting = %q, want hello", got.Data.Greeting)
	}
	if got.Data.Name != "Ada" {
		t.Fatalf("name = %q, want Ada", got.Data.Name)
	}
	if got.Data.Mode != ax.ModeJSON.String() {
		t.Fatalf("mode = %q, want %q", got.Data.Mode, ax.ModeJSON)
	}
	if got.Meta.IdempotencyKey != "test-key" {
		t.Fatalf("idempotency key = %q, want test-key", got.Meta.IdempotencyKey)
	}

	var logLine map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &logLine); err != nil {
		t.Fatalf("stderr log line was not JSON: %v", err)
	}
	if logLine["application"] != appName {
		t.Fatalf("application label = %v, want %s", logLine["application"], appName)
	}
}

func TestRunAcceptsHujsonConfigFromStdin(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	config := `{
		// accepted by Hujson
		"name": "configured",
		"count": 2,
	}`

	code := run(
		context.Background(),
		[]string{"--format=json", "--idempotency-key=test-key", "--config=-"},
		strings.NewReader(config),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}

	var got ax.Envelope[helloPayload]
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not a JSON envelope: %v", err)
	}
	if got.Data.Config == nil {
		t.Fatal("config missing from payload")
	}
	if got.Data.Config.Name != "configured" {
		t.Fatalf("config name = %q, want configured", got.Data.Config.Name)
	}
	if got.Data.Config.Count != 2 {
		t.Fatalf("config count = %d, want 2", got.Data.Config.Count)
	}
}

func TestRunRejectsOversizedHujsonConfigFromStdin(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	config := strings.Repeat(" ", int(ax.DefaultMaxConfigBytes)) + "{}"

	code := run(
		context.Background(),
		[]string{"--format=json", "--idempotency-key=test-key", "--config=-"},
		strings.NewReader(config),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitValidation {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitValidation, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var got ax.Error
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("stderr was not an error envelope: %v", err)
	}
	if got.ErrorCode != "config_too_large" {
		t.Fatalf("error code = %q, want config_too_large", got.ErrorCode)
	}
}

func TestRunStreamCommandEmitsNDJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(
		context.Background(),
		[]string{"stream", "--format=json", "--idempotency-key=test-key", "--count=2", "--name=Ada"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2; stdout=%s", len(lines), stdout.String())
	}

	for i, line := range lines {
		var got ax.Envelope[streamPayload]
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d was not a JSON envelope: %v", i, err)
		}
		if got.Data.Index != i {
			t.Fatalf("line %d index = %d, want %d", i, got.Data.Index, i)
		}
		if got.Data.Name != "Ada" {
			t.Fatalf("line %d name = %q, want Ada", i, got.Data.Name)
		}
	}
}

func TestRunFailCommandWritesErrorEnvelopeToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(
		context.Background(),
		[]string{failCommandName, "--format=json"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitValidation {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitValidation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var got ax.Error
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("stderr was not an error envelope: %v", err)
	}
	if got.ErrorCode != "integration_failure" {
		t.Fatalf("error code = %q, want integration_failure", got.ErrorCode)
	}
	if got.Tool != appName {
		t.Fatalf("tool = %q, want %s", got.Tool, appName)
	}
	want := ax.ResolveVersion(version)
	if got.Version != want {
		t.Fatalf("version = %q, want %q", got.Version, want)
	}
	if got.Retryable == nil || *got.Retryable {
		t.Fatalf("retryable = %v, want explicit false (validation failures are not retryable)", got.Retryable)
	}
}

func TestRunUsesResolvedVersionAcrossSchemaAndLogger(t *testing.T) {
	want := ax.ResolveVersion(version)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(
		context.Background(),
		[]string{"--name", "Ada"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}

	gotLogVersion := ""
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if line == "" {
			continue
		}
		var logLine map[string]any
		if err := json.Unmarshal([]byte(line), &logLine); err != nil {
			t.Fatalf("stderr log line was not JSON: %v", err)
		}
		if versionValue, ok := logLine["version"].(string); ok && versionValue != "" {
			gotLogVersion = versionValue
			break
		}
	}
	if gotLogVersion == "" {
		t.Fatalf("stderr log version missing; stderr=%s", stderr.String())
	}
	if gotLogVersion != want {
		t.Fatalf("logger version = %q, want %q", gotLogVersion, want)
	}

	stdout.Reset()
	stderr.Reset()
	code = run(
		context.Background(),
		[]string{"__schema"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitSuccess {
		t.Fatalf("schema exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}

	var gotSchema ax.Schema
	if err := json.Unmarshal(stdout.Bytes(), &gotSchema); err != nil {
		t.Fatalf("stdout was not schema JSON: %v", err)
	}
	if gotSchema.Version != want {
		t.Fatalf("schema version = %q, want %q", gotSchema.Version, want)
	}
}

func TestRunPatchConfigCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")
	initial := []byte(`{
	// integration config
	"name": "Ada",
	"count": 2,
}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(
		context.Background(),
		[]string{
			"patch-config",
			"--format=json",
			"--idempotency-key=test-key",
			"--config=" + path,
			`--patch=[{"op":"replace","path":"/count","value":5}]`,
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}

	var got ax.Envelope[patchConfigPayload]
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not a JSON envelope: %v", err)
	}
	if !got.Data.Patched {
		t.Fatal("payload patched = false, want true")
	}
	if got.Data.Path != path {
		t.Fatalf("payload path = %q, want %q", got.Data.Path, path)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if !strings.Contains(string(result), "// integration config") {
		t.Fatalf("patch stripped comments; got:\n%s", result)
	}
	if !strings.Contains(string(result), "5") {
		t.Fatalf("patch was not applied; got:\n%s", result)
	}
}

// captureProcessStderr swaps os.Stderr for a pipe while fn runs and returns
// what was written. ax.NewLogger writes the dry-run suppression line to the
// process os.Stderr, so this is how an end-to-end test observes it. It mutates
// the global os.Stderr, so callers MUST NOT be parallel.
func captureProcessStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w //nolint:reassign // redirect process stderr to capture the canonical logger's suppression line
	defer func() {
		os.Stderr = orig //nolint:reassign // restore process stderr after capture
	}()
	fn()
	if cerr := w.Close(); cerr != nil {
		t.Fatalf("close pipe writer: %v", cerr)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out)
}

func TestRunPatchConfigCommandDryRunHasNoSideEffects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")
	initial := []byte(`{
	// integration config
	"name": "Ada",
	"count": 2,
}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	// ax.Perform writes the dry-run suppression line to the process os.Stderr via
	// the canonical logger (not the Execute-configured stderr buffer), so capture
	// os.Stderr to observe it end-to-end and to keep it out of test output.
	var code int
	processStderr := captureProcessStderr(t, func() {
		code = run(
			context.Background(),
			[]string{
				"patch-config",
				"--format=json",
				"--dry-run",
				"--idempotency-key=test-key",
				"--config=" + path,
				`--patch=[{"op":"replace","path":"/count","value":5}]`,
			},
			strings.NewReader(""),
			&stdout,
			&stderr,
			func(string) string { return "" },
		)
	})

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}

	// SC-007: the suppressed commit emits exactly one diagnostic line to stderr.
	if got := strings.Count(processStderr, "side effect suppressed"); got != 1 {
		t.Fatalf("want exactly one suppression line on stderr, got %d: %q", got, processStderr)
	}
	if !strings.Contains(processStderr, `"ax_helper":"Perform"`) {
		t.Fatalf("suppression line missing ax_helper=Perform; got: %q", processStderr)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !bytes.Equal(result, initial) {
		t.Fatalf("dry-run modified the file; got:\n%s", result)
	}

	var got ax.Envelope[patchConfigPayload]
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not a JSON envelope: %v", err)
	}
	if !got.Meta.DryRun {
		t.Fatal("meta dry_run = false, want true")
	}
	if !got.Data.Patched {
		t.Fatal("payload patched = false, want true (dry-run payload must match a real run)")
	}
	if got.Data.Path != path {
		t.Fatalf("payload path = %q, want %q", got.Data.Path, path)
	}
}

func TestRunPatchConfigCommandDryRunSurfacesPatchErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")
	initial := []byte(`{"count": 2}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(
		context.Background(),
		[]string{
			"patch-config",
			"--format=json",
			"--dry-run",
			"--config=" + path,
			`--patch=[{"op":"remove","path":"/nonexistent"}]`,
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitValidation {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitValidation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout not empty on dry-run patch failure: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "config_patch_invalid") {
		t.Fatalf("stderr missing config_patch_invalid envelope; got: %s", stderr.String())
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !bytes.Equal(result, initial) {
		t.Fatalf("dry-run modified the file; got:\n%s", result)
	}
}

func TestRunPatchConfigCommandRequiresFlags(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(
		context.Background(),
		[]string{"patch-config", "--format=json"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitValidation {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitValidation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout not empty on validation failure: %s", stdout.String())
	}
}

func TestRunSchemaCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run(
		context.Background(),
		[]string{"__schema"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		func(string) string { return "" },
	)

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
	}

	var got ax.Schema
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not schema JSON: %v", err)
	}
	if got.Tool != appName {
		t.Fatalf("tool = %q, want %s", got.Tool, appName)
	}
	if got.Version == "" {
		t.Fatal("schema version is empty")
	}
	if got.Version == "v0.1.0" {
		t.Fatal("schema version still uses the old hardcoded v0.1.0 placeholder")
	}
	if len(got.Command.Commands) == 0 {
		t.Fatal("schema did not include subcommands")
	}
}

// TestMCPServerSmoke exercises the mounted mcp-server end to end: it serves the
// integration command tree over a loopback HTTP transport and asserts a client
// can discover the integration commands as tools (with mcp-server excluded) and
// invoke one to get its machine payload. It is the runnable reference instance
// for feature 011.
func TestMCPServerSmoke(t *testing.T) {
	root := newRootCommand(strings.NewReader(""), "v9.9.9-smoke", ax.NewEntityID)

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
			mcp.WithVersion("v9.9.9-smoke"),
		)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-serveErr:
			if err != nil {
				t.Errorf("Serve returned non-nil on shutdown: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("Serve did not return after cancellation")
		}
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond); dialErr == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	client := sdk.NewClient(&sdk.Implementation{Name: "smoke-client", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             "http://" + addr,
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	tools := map[string]bool{}
	for _, tool := range list.Tools {
		tools[tool.Name] = true
	}
	if !tools[appName+"-"+streamCommandName] {
		t.Errorf("tools/list missing %q; got %v", appName+"-"+streamCommandName, tools)
	}
	if tools[appName+"-mcp-server"] {
		t.Error("mcp-server must be excluded from its own tool list")
	}

	res, err := session.CallTool(ctx, &sdk.CallToolParams{
		Name:      appName + "-" + streamCommandName,
		Arguments: map[string]any{"count": 2.0, "name": "smoke"},
	})
	if err != nil {
		t.Fatalf("tools/call stream: %v", err)
	}
	if res.IsError {
		t.Fatalf("stream call returned IsError")
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *mcp.TextContent", res.Content[0])
	}
	if !strings.Contains(text.Text, `"name":"smoke"`) {
		t.Errorf("stream payload missing name=smoke; got %q", text.Text)
	}
}

// TestQuickstartAgainstBuiltBinary validates the feature 011 quickstart
// end-to-end against the actual built ax-integration binary: it speaks MCP over
// stdio (the canonical subprocess launch model, exercising the mounted
// ax.Execute path and re-entrant dispatch) and over a loopback HTTP transport,
// running initialize -> tools/list -> tools/call against each
// (specs/011-mcp-server-runtime/quickstart.md).
func TestQuickstartAgainstBuiltBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the integration binary")
	}

	binary := filepath.Join(t.TempDir(), "ax-integration")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build integration binary: %v\n%s", err, out)
	}

	t.Run("stdio", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client := sdk.NewClient(&sdk.Implementation{Name: "qs-stdio", Version: "v0"}, nil)
		session, err := client.Connect(ctx, &sdk.CommandTransport{
			Command: exec.Command(binary, "mcp-server"),
		}, nil)
		if err != nil {
			t.Fatalf("connect over stdio: %v", err)
		}
		defer func() { _ = session.Close() }()

		assertQuickstartSession(ctx, t, session)
	})

	t.Run("http", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve port: %v", err)
		}
		addr := listener.Addr().String()
		_ = listener.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server := exec.CommandContext(ctx, binary, "mcp-server",
			"--transport=http", "--addr="+addr)
		server.Stderr = os.Stderr
		if err := server.Start(); err != nil {
			t.Fatalf("start http server: %v", err)
		}
		defer func() {
			_ = server.Process.Kill()
			_ = server.Wait()
		}()

		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if conn, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond); dialErr == nil {
				_ = conn.Close()
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		client := sdk.NewClient(&sdk.Implementation{Name: "qs-http", Version: "v0"}, nil)
		session, err := client.Connect(ctx, &sdk.StreamableClientTransport{
			Endpoint:             "http://" + addr,
			DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			t.Fatalf("connect over http: %v", err)
		}
		defer func() { _ = session.Close() }()

		assertQuickstartSession(ctx, t, session)
	})
}

// assertQuickstartSession runs the quickstart's initialize -> tools/list ->
// tools/call sequence and asserts a real version, the expected tool set
// (reserved mcp-server, __schema, and completion excluded), and a successful
// call returning the command payload.
func assertQuickstartSession(ctx context.Context, t *testing.T, session *sdk.ClientSession) {
	t.Helper()

	if version := session.InitializeResult().ServerInfo.Version; version == "" ||
		version == "dev" || version == "unknown" {
		t.Fatalf("initialize reported a placeholder version %q", version)
	}

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	tools := map[string]bool{}
	for _, tool := range list.Tools {
		tools[tool.Name] = true
	}
	if !tools[appName+"-"+streamCommandName] {
		t.Errorf("tools/list missing %q; got %v", appName+"-"+streamCommandName, tools)
	}
	if tools[appName+"-mcp-server"] {
		t.Error("mcp-server must be excluded from its own tool list")
	}
	if tools[appName+"-__schema"] {
		t.Error("__schema must be excluded from the tool list")
	}
	if tools[appName+"-completion"] {
		t.Error("completion must be excluded from the tool list")
	}

	res, err := session.CallTool(ctx, &sdk.CallToolParams{
		Name:      appName + "-" + streamCommandName,
		Arguments: map[string]any{"count": 2.0, "name": "quickstart"},
	})
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	if res.IsError {
		t.Fatalf("stream call returned IsError")
	}
	text, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *mcp.TextContent", res.Content[0])
	}
	if !strings.Contains(text.Text, `"name":"quickstart"`) {
		t.Errorf("stream payload missing name=quickstart; got %q", text.Text)
	}
}
