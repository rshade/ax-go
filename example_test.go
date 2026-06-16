package ax_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ax "github.com/rshade/ax-go"
)

// ExampleParseConfig shows the read-side Hujson asymmetry: comments and trailing
// commas are accepted on input, and the result decodes into a normal struct.
func ExampleParseConfig() {
	const hujson = `{
		// comments and trailing commas are allowed on reads
		"name": "ax",
		"replicas": 3,
	}`

	var cfg struct {
		Name     string `json:"name"`
		Replicas int    `json:"replicas"`
	}
	if err := ax.ParseConfig(
		context.Background(),
		strings.NewReader(hujson),
		&cfg,
		ax.WithMaxConfigBytes(1<<10),
	); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%s x%d\n", cfg.Name, cfg.Replicas)
	// Output: ax x3
}

// ExampleParseConfigFile reads and decodes a Hujson configuration file from
// disk, applying the same 1 MiB read cap as ParseConfig.
func ExampleParseConfigFile() {
	dir, err := os.MkdirTemp("", "ax-config")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "config.hujson")
	if err := os.WriteFile(path, []byte(`{"name": "ax"}`), 0o600); err != nil {
		fmt.Println("error:", err)
		return
	}

	var cfg struct {
		Name string `json:"name"`
	}
	if err := ax.ParseConfigFile(context.Background(), path, &cfg); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(cfg.Name)
	// Output: ax
}

// ExampleNewError builds a structured error envelope with an actionable fix and
// a deterministic exit code, then reads the exit code back out.
func ExampleNewError() {
	err := ax.NewError(
		context.Background(),
		"config_too_large",
		"config exceeds maximum size of 1048576 bytes",
		ax.WithActionableFix("reduce the config or raise the limit with WithMaxConfigBytes"),
		ax.WithErrorExitCode(ax.ExitValidation),
	)

	fmt.Println(err)
	fmt.Println(ax.ErrorExitCode(err))
	// Output:
	// config exceeds maximum size of 1048576 bytes
	// 2
}

// ExampleError shows how a consumer classifies a failure: recover the *ax.Error
// envelope with errors.As, then branch on the stable error_code and exit code
// without parsing human-facing text.
func ExampleError() {
	err := ax.NewError(
		context.Background(),
		"config_max_bytes_invalid",
		"config max bytes must be between 0 and 1073741824",
		ax.WithErrorExitCode(ax.ExitValidation),
	)

	var axErr *ax.Error
	if errors.As(err, &axErr) {
		fmt.Println(axErr.ErrorCode)
		fmt.Println(axErr.ExitCode())
	}
	// Output:
	// config_max_bytes_invalid
	// 2
}

// ExampleNewEnvelope wraps a payload in the standard success envelope, carrying
// trace metadata from the context. With no active span the IDs are the zero
// W3C values, so the output is deterministic.
func ExampleNewEnvelope() {
	type result struct {
		ID string `json:"id"`
	}

	env := ax.NewEnvelope(context.Background(), result{ID: "abc"})
	if err := ax.WriteJSON(os.Stdout, env); err != nil {
		fmt.Println("error:", err)
	}
	// Output: {"data":{"id":"abc"},"meta":{"trace_id":"00000000000000000000000000000000","span_id":"0000000000000000"}}
}

// ExampleEnvelope shows the envelope shape directly: a typed Data field and a
// Metadata block. span_id is omitted when empty.
func ExampleEnvelope() {
	env := ax.Envelope[string]{
		Data: "hello",
		Meta: ax.Metadata{TraceID: ax.ZeroTraceID},
	}
	if err := ax.WriteJSON(os.Stdout, env); err != nil {
		fmt.Println("error:", err)
	}
	// Output: {"data":"hello","meta":{"trace_id":"00000000000000000000000000000000"}}
}

// ExampleMode shows agent-mode resolution: with no --format flag, no AGENT_MODE,
// and a non-TTY stdout, ax resolves to machine-readable JSON.
func ExampleMode() {
	mode, err := ax.ResolveMode("", "", false)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(mode)
	// Output: json
}

// ExampleNewIdempotencyKey returns a UUID v4 string in canonical 36-character
// form, surfaced in the output envelope so retries are safe.
func ExampleNewIdempotencyKey() {
	key := ax.NewIdempotencyKey()
	fmt.Println(len(key))
	// Output: 36
}

// ExampleNewEntityID returns a UUID v7 resource identifier in canonical
// 36-character form.
func ExampleNewEntityID() {
	id, err := ax.NewEntityID()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(len(id))
	// Output: 36
}

// ExampleResolveVersion shows the deterministic injected-version path used by
// release builds. When the injected value is empty, ResolveVersion falls back to
// the running binary's build metadata and finally to "0.0.0-unknown".
func ExampleResolveVersion() {
	fmt.Println(ax.ResolveVersion("v1.2.3"))
	// Output: v1.2.3
}

// ExampleStartTelemetry installs W3C propagation and configures the telemetry
// lifecycle with explicit stderr, service identity, and shutdown budget options.
func ExampleStartTelemetry() {
	var stderr bytes.Buffer
	ctx, telemetry, err := ax.StartTelemetry(
		context.Background(),
		ax.WithTelemetryEnv(func(string) string { return "" }),
		ax.WithTelemetryStderr(&stderr),
		ax.WithTelemetryServiceName("example"),
		ax.WithTelemetryServiceVersion("v1.2.3"),
		ax.WithTelemetryShutdownBudget(50*time.Millisecond),
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	if err := telemetry.Shutdown(context.Background()); err != nil {
		fmt.Println("shutdown:", err)
		return
	}

	fmt.Println(telemetry != nil)
	fmt.Println(ax.TraceIDFromContext(ctx))
	fmt.Println(stderr.Len())
	// Output:
	// true
	// 00000000000000000000000000000000
	// 0
}

// ExamplePatchConfig shows the comment-preserving write path: RFC 6902
// patch operations mutate the Hujson AST in place so user comments survive.
func ExamplePatchConfig() {
	const existing = `{
	// service endpoint
	"host": "localhost",
	"port": 8080,
}`
	patch := []byte(`[{"op":"replace","path":"/port","value":9090}]`)

	patched, err := ax.PatchConfig(
		context.Background(),
		strings.NewReader(existing),
		patch,
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(strings.Contains(string(patched), "// service endpoint"))
	fmt.Println(strings.Contains(string(patched), "9090"))
	// Output:
	// true
	// true
}

// ExamplePatchConfigFile reads a Hujson config file, applies RFC 6902 patch
// operations, and writes the result back atomically, preserving user comments.
func ExamplePatchConfigFile() {
	dir, err := os.MkdirTemp("", "ax-patch")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "config.hujson")
	initial := []byte(`{
	// production endpoint
	"host": "prod.example.com",
	"port": 443,
}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		fmt.Println("error:", err)
		return
	}

	patch := []byte(`[{"op":"replace","path":"/port","value":8443}]`)
	if err := ax.PatchConfigFile(context.Background(), path, patch); err != nil {
		fmt.Println("error:", err)
		return
	}

	result, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(strings.Contains(string(result), "// production endpoint"))
	fmt.Println(strings.Contains(string(result), "8443"))
	// Output:
	// true
	// true
}

// ExampleWithLokiFromEnv shows the no-op path: when AX_LOKI_URL is unset,
// WithLokiFromEnv adds no Loki sink and the logger behaves identically to a
// logger without it. No network connection is attempted. CLI authors add this
// option once; it activates only when operators set AX_LOKI_URL at runtime.
func ExampleWithLokiFromEnv() {
	os.Unsetenv("AX_LOKI_URL") // ensure deterministic no-op output
	var buf bytes.Buffer
	logger := ax.NewLogger(
		context.Background(),
		ax.WithLoggerWriter(&buf),
		ax.WithLokiFromEnv(),
	)
	logger.Info(context.Background()).Msg("no loki sink active")
	// Output:
}

// ExampleFlush shows the shutdown pattern: Flush drains any buffered Loki log
// entries before the process exits. When no Loki sink is active (AX_LOKI_URL
// unset), Flush is a no-op that returns nil immediately.
func ExampleFlush() {
	os.Unsetenv("AX_LOKI_URL") // ensure no-op for this example
	var buf bytes.Buffer
	logger := ax.NewLogger(
		context.Background(),
		ax.WithLoggerWriter(&buf),
		ax.WithLokiFromEnv(),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = ax.Flush(ctx, logger)
	// Output:
}
