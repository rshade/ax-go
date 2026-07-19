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

	"github.com/spf13/cobra"

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

// ExampleWithRetryable shows a producer attaching machine-readable recovery
// guidance to a failure: an explicit retry-safety signal plus a relative backoff
// hint, so an agent can decide to retry (and how long to wait) without parsing
// human-facing text. The envelope is rendered to a buffer here for illustration;
// in a real CLI it is written to stderr.
func ExampleWithRetryable() {
	err := ax.NewError(
		context.Background(),
		"network_timeout",
		"upstream timed out",
		ax.WithErrorTool("app"),
		ax.WithErrorVersion("v0.1.0"),
		ax.WithErrorExitCode(ax.ExitNetwork),
		ax.WithRetryable(true),
		ax.WithRetryAfterSeconds(30),
	)

	var buf bytes.Buffer
	_ = ax.WriteError(&buf, err)
	fmt.Print(buf.String())
	// Output:
	// {"error_code":"network_timeout","message":"upstream timed out","trace_id":"00000000000000000000000000000000","tool":"app","version":"v0.1.0","schema_version":"1.0.0","retryable":true,"retry_after_seconds":30}
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

// ExampleGuard shows the skip-only guard: the effect runs normally, but under
// --dry-run it is suppressed entirely and Guard reports executed=false. The
// suppression line Guard writes to stderr is not shown here (examples capture
// stdout only).
func ExampleGuard() {
	effect := func(context.Context) error {
		fmt.Println("writing report")
		return nil
	}

	// Real run: the effect executes.
	executed, err := ax.Guard(context.Background(), effect)
	fmt.Printf("executed=%v err=%v\n", executed, err)

	// Dry-run: the effect is skipped.
	dryRun := ax.WithDryRun(context.Background(), true)
	executed, err = ax.Guard(dryRun, effect)
	fmt.Printf("executed=%v err=%v\n", executed, err)
	// Output:
	// writing report
	// executed=true err=<nil>
	// executed=false err=<nil>
}

// ExamplePerform shows the rehearse/commit pair: a real run performs commit,
// while --dry-run runs the read-only rehearse preview instead (surfacing the
// same validation errors) without performing the mutation.
func ExamplePerform() {
	rehearse := func(context.Context) error {
		fmt.Println("validating only")
		return nil
	}
	commit := func(context.Context) error {
		fmt.Println("committing")
		return nil
	}

	// Real run: commit executes, rehearse is ignored.
	_ = ax.Perform(context.Background(), rehearse, commit)

	// Dry-run: rehearse executes, commit is skipped.
	dryRun := ax.WithDryRun(context.Background(), true)
	_ = ax.Perform(dryRun, rehearse, commit)
	// Output:
	// committing
	// validating only
}

// ExampleExecute wraps a Cobra root command with the full AX lifecycle —
// telemetry, mode resolution, and idempotency-key injection — and maps the
// result to a deterministic exit code instead of exiting the process. The
// command reads the resolved values back out of its context. With an explicit
// --idempotency-key and a buffered, non-TTY stdout the run is fully
// deterministic: the payload is the only stdout output and stderr stays empty.
func ExampleExecute() {
	var stdout, stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, _ := ax.ModeFromContext(cmd.Context())
			key, _ := ax.IdempotencyKeyFromContext(cmd.Context())
			return ax.WriteJSON(cmd.OutOrStdout(), struct {
				Mode   ax.Mode `json:"mode"`
				DryRun bool    `json:"dry_run"`
				Key    string  `json:"key"`
			}{
				Mode:   mode,
				DryRun: ax.DryRunFromContext(cmd.Context()),
				Key:    key,
			})
		},
	}
	root.SetArgs([]string{"--dry-run", "--idempotency-key=abc"})

	code := ax.Execute(
		context.Background(),
		root,
		ax.WithStdout(&stdout),
		ax.WithStderr(&stderr),
		ax.WithEnv(func(string) string { return "" }),
		ax.WithStdoutIsTTY(false),
	)

	fmt.Println("exit:", code)
	fmt.Print(stdout.String())
	fmt.Println("stderr bytes:", stderr.Len())
	// Output:
	// exit: 0
	// {"mode":"json","dry_run":true,"key":"abc"}
	// stderr bytes: 0
}

// ExampleBuildSchema reflects a Cobra command tree into the versioned,
// ax-native schema document the reserved __schema command emits on stdout.
// The output is strict minified JSON with no timestamps or generated IDs, so
// it is byte-identical across runs.
func ExampleBuildSchema() {
	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app run",
	}
	root.Flags().String("config", "", "config file")

	s := ax.BuildSchema(root, ax.WithSchemaVersion("v0.1.0"))
	if err := ax.WriteJSON(os.Stdout, s); err != nil {
		fmt.Println("error:", err)
	}
	// Output: {"schema_version":"1.0.0","tool":"app","version":"v0.1.0","mode_detection":"--format flag \u003e AGENT_MODE env \u003e TTY detection","command":{"use":"app","short":"test app","example":"app run","flags":[{"name":"config","type":"string","usage":"config file"}]},"error_envelope":{"schema_version":"1.0.0","required":["error_code","message","trace_id","tool","version","schema_version"],"optional":["actionable_fix","context","suggestions"]}}
}

// ExampleBuildMCPSchema adapts the command tree to the MCP tools-list shape
// emitted by __schema --as=mcp: each callable command becomes a tool whose
// inputSchema describes its flags as a JSON Schema object.
func ExampleBuildMCPSchema() {
	root := &cobra.Command{
		Use:   "app",
		Short: "test app",
	}
	root.Flags().String("config", "", "config file")

	mcpSchema := ax.BuildMCPSchema(root)
	fmt.Println(mcpSchema.Tools[0].Name)
	fmt.Println(mcpSchema.Tools[0].Description)
	fmt.Println(mcpSchema.Tools[0].InputSchema["type"])
	// Output:
	// app
	// test app
	// object
}

// ExampleSchema shows the ax-native reflective schema tree: tool identity, the
// build-injected version, the mode-detection rule, the command tree, and the
// error-envelope contract. The schema carries its own schema_version, so it
// can evolve independently of the tool version.
func ExampleSchema() {
	root := &cobra.Command{
		Use:   "app",
		Short: "test app",
	}

	s := ax.BuildSchema(root, ax.WithSchemaVersion("v0.1.0"))
	fmt.Println(s.SchemaVersion)
	fmt.Println(s.Tool)
	fmt.Println(s.Version)
	fmt.Println(s.ModeDetection)
	// Output:
	// 1.0.0
	// app
	// v0.1.0
	// --format flag > AGENT_MODE env > TTY detection
}

// ExampleNewLogger builds the canonical structured logger: zerolog-backed,
// writing to the configured writer (stderr by default), with trace_id and
// span_id stamped on every line for trace correlation. With no active span
// the IDs are the zero W3C values, so the output is deterministic.
func ExampleNewLogger() {
	var buf bytes.Buffer
	logger := ax.NewLogger(context.Background(), ax.WithLoggerWriter(&buf))
	logger.Info(context.Background()).Msg("hello")
	fmt.Print(buf.String())
	// Output: {"level":"info","trace_id":"00000000000000000000000000000000","span_id":"0000000000000000","message":"hello"}
}

// ExampleLogger shows the Logger surface beyond construction: WithLabels
// derives a logger that stamps low-cardinality labels on every line while
// keeping trace correlation. Keep labels low-cardinality (environment,
// application, host, version) — they are indexed in Loki.
func ExampleLogger() {
	var buf bytes.Buffer
	logger := ax.NewLogger(context.Background(), ax.WithLoggerWriter(&buf))
	labeled := logger.WithLabels(ax.Labels{Application: "app", Version: "v1.2.3"})
	labeled.Info(context.Background()).Msg("started")
	fmt.Print(buf.String())
	// Output: {"level":"info","application":"app","version":"v1.2.3","trace_id":"00000000000000000000000000000000","span_id":"0000000000000000","message":"started"}
}

// ExampleTelemetry shows the lifecycle guard on Telemetry.Shutdown: it is
// nil-safe, so a CLI can defer Shutdown unconditionally on the handle —
// including a zero handle when setup was skipped — without a nil-pointer
// panic.
func ExampleTelemetry() {
	var telemetry *ax.Telemetry
	fmt.Println(telemetry.Shutdown(context.Background()))
	// Output: <nil>
}
