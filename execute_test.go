package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/cli"
)

func TestExecuteLogLinesCarryRootSpanContextWithoutCollector(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var runTraceID string
	var runSpanID string

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runTraceID = TraceIDFromContext(cmd.Context())
			runSpanID = SpanIDFromContext(cmd.Context())
			logger := NewLogger(cmd.Context(), WithLoggerWriter(cmd.ErrOrStderr()))
			logger.Info(cmd.Context()).Str("event", "first").Msg("ran")
			logger.Info(cmd.Context()).Str("event", "second").Msg("ran")
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if runTraceID == ZeroTraceID {
		t.Fatalf("TraceIDFromContext during run = %q, want non-zero", runTraceID)
	}
	if runSpanID == ZeroSpanID {
		t.Fatalf("SpanIDFromContext during run = %q, want non-zero", runSpanID)
	}

	records := decodeLogRecords(t, stderr.String())
	if len(records) != 2 {
		t.Fatalf("log records = %d, want 2; stderr=%s", len(records), stderr.String())
	}
	for _, record := range records {
		if record["trace_id"] != runTraceID {
			t.Fatalf("log trace_id = %v, want active trace %q", record["trace_id"], runTraceID)
		}
		if record["span_id"] != runSpanID {
			t.Fatalf("log span_id = %v, want active span %q", record["span_id"], runSpanID)
		}
	}
	if strings.Contains(stdout.String(), runTraceID) {
		t.Fatalf("stdout contains trace_id %q: %s", runTraceID, stdout.String())
	}
	if strings.Contains(stdout.String(), runSpanID) {
		t.Fatalf("stdout contains span_id %q: %s", runSpanID, stdout.String())
	}
}

func TestExecuteContinuesInboundTraceparent(t *testing.T) {
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var runTraceID string

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			runTraceID = TraceIDFromContext(cmd.Context())
			logger := NewLogger(cmd.Context(), WithLoggerWriter(cmd.ErrOrStderr()))
			logger.Info(cmd.Context()).Msg("ran")
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithEnv(func(key string) string {
			if key == "TRACEPARENT" {
				return "00-" + traceID + "-00f067aa0ba902b7-01"
			}
			return ""
		}),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if runTraceID != traceID {
		t.Fatalf("TraceIDFromContext during run = %q, want inbound trace %q", runTraceID, traceID)
	}
	records := decodeLogRecords(t, stderr.String())
	if len(records) != 1 {
		t.Fatalf("log records = %d, want 1; stderr=%s", len(records), stderr.String())
	}
	if records[0]["trace_id"] != traceID {
		t.Fatalf("log trace_id = %v, want inbound trace %q", records[0]["trace_id"], traceID)
	}
	if strings.Contains(stdout.String(), traceID) {
		t.Fatalf("stdout contains trace_id %q: %s", traceID, stdout.String())
	}
}

func TestExecuteTelemetryFailOpen(t *testing.T) {
	baselineStdout, _, baselineCode := executeTelemetryCommand(
		t,
		map[string]string{},
		defaultTelemetryShutdownTimeout,
	)

	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "malformed endpoint",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "://bad-endpoint",
			},
		},
		{
			name: "unreachable collector",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:1",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := executeTelemetryCommand(t, tc.env, 100*time.Millisecond)

			if code != baselineCode {
				t.Fatalf(
					"Execute exit code = %d, want baseline %d; stderr=%s",
					code,
					baselineCode,
					stderr,
				)
			}
			if !bytes.Equal(stdout, baselineStdout) {
				t.Fatalf(
					"stdout changed under telemetry failure\nbaseline: %s\ngot: %s",
					baselineStdout,
					stdout,
				)
			}
			if !strings.Contains(stderr, "ax: otel") {
				t.Fatalf("stderr = %q, want telemetry diagnostic", stderr)
			}
		})
	}
}

func TestExecuteInjectsAXContext(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode, ok := ModeFromContext(cmd.Context())
			if !ok {
				t.Fatal("mode missing from context")
			}
			key, ok := IdempotencyKeyFromContext(cmd.Context())
			if !ok {
				t.Fatal("idempotency key missing from context")
			}
			return WriteJSON(cmd.OutOrStdout(), map[string]any{
				"mode":    mode,
				"dry_run": DryRunFromContext(cmd.Context()),
				"key":     key,
			})
		},
	}
	root.SetArgs([]string{"--format=json", "--dry-run", "--idempotency-key=abc"})

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithStdoutIsTTY(true),
		WithEnv(func(string) string { return "" }),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not JSON: %v", err)
	}
	if got["mode"] != string(ModeJSON) {
		t.Fatalf("mode = %v, want %q", got["mode"], ModeJSON)
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true", got["dry_run"])
	}
	if got["key"] != "abc" {
		t.Fatalf("key = %v, want abc", got["key"])
	}
}

func TestExecuteResolvesApprovalAndBlocksWithoutWritingStdout(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantEffect bool
	}{
		{name: "without approval", args: []string{"--format=json"}, wantCode: ExitValidation},
		{
			name:       "with approval",
			args:       []string{"--format=json", "--yes"},
			wantCode:   ExitSuccess,
			wantEffect: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			effect := false
			root := &cobra.Command{Use: "app", RunE: func(cmd *cobra.Command, _ []string) error {
				outcome, err := Confirm(cmd.Context(), "apply the change")
				if err != nil {
					return err
				}
				if outcome == ConfirmationApproved {
					effect = true
				}
				return WriteJSON(cmd.OutOrStdout(), struct {
					OK bool `json:"ok"`
				}{OK: true})
			}}
			root.SetArgs(tt.args)

			code := Execute(context.Background(), root,
				WithStdout(&stdout), WithStderr(&stderr), WithVersion("v1"),
				WithEnv(func(string) string { return "" }), WithStdoutIsTTY(false))
			if code != tt.wantCode {
				t.Fatalf(
					"Execute exit code = %d, want %d; stderr=%s",
					code,
					tt.wantCode,
					stderr.String(),
				)
			}
			if effect != tt.wantEffect {
				t.Errorf("effect = %v, want %v", effect, tt.wantEffect)
			}
			if !tt.wantEffect && stdout.Len() != 0 {
				t.Fatalf("blocked stdout = %q, want empty", stdout.String())
			}
			if tt.wantEffect && stderr.Len() != 0 {
				t.Fatalf("approved stderr = %q, want empty", stderr.String())
			}
			if !tt.wantEffect {
				var got Error
				if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
					t.Fatalf("blocked stderr is not JSON: %v", err)
				}
				if got.ErrorCode != "confirmation_required" || got.ActionableFix == "" {
					t.Fatalf("blocked envelope = %+v", got)
				}
			}
		})
	}
}

func TestExecuteApprovalAndDryRunAreOrthogonal(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantCode     int
		wantApproved bool
		wantDryRun   bool
		wantEffect   bool
	}{
		{name: "blocked real", args: []string{"--format=json"}, wantCode: ExitValidation},
		{
			name:     "blocked dry-run",
			args:     []string{"--format=json", "--dry-run"},
			wantCode: ExitValidation,
		},
		{
			name:         "approved real",
			args:         []string{"--format=json", "--yes"},
			wantCode:     ExitSuccess,
			wantApproved: true,
			wantEffect:   true,
		},
		{
			name:         "approved dry-run",
			args:         []string{"--format=json", "--yes", "--dry-run"},
			wantCode:     ExitSuccess,
			wantApproved: true,
			wantDryRun:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			effect := false
			root := &cobra.Command{Use: "app", RunE: func(cmd *cobra.Command, _ []string) error {
				outcome, err := Confirm(cmd.Context(), "apply the change")
				if err != nil {
					return err
				}
				if outcome == ConfirmationApproved && !DryRunFromContext(cmd.Context()) {
					effect = true
				}
				return WriteJSON(cmd.OutOrStdout(), struct {
					Approved bool `json:"approved"`
					DryRun   bool `json:"dry_run"`
				}{Approved: outcome == ConfirmationApproved, DryRun: DryRunFromContext(cmd.Context())})
			}}
			root.SetArgs(tt.args)
			code := Execute(
				context.Background(),
				root,
				WithStdout(&stdout),
				WithStderr(&stderr),
				WithEnv(
					func(string) string { return "" },
				),
				WithStdoutIsTTY(false),
				WithVersion("v1"),
			)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr=%s", code, tt.wantCode, stderr.String())
			}
			if effect != tt.wantEffect {
				t.Errorf("effect = %v, want %v", effect, tt.wantEffect)
			}
			if tt.wantCode == ExitSuccess {
				var got struct {
					Approved bool `json:"approved"`
					DryRun   bool `json:"dry_run"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
					t.Fatalf("stdout is not JSON: %v", err)
				}
				if got.Approved != tt.wantApproved || got.DryRun != tt.wantDryRun {
					t.Fatalf(
						"payload = %+v, want approved=%v dry_run=%v",
						got,
						tt.wantApproved,
						tt.wantDryRun,
					)
				}
			}
		})
	}
}

func TestExecutePreservesPipedStdinAndAuthorYesCollision(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var read bool
	root := &cobra.Command{Use: "app", RunE: func(cmd *cobra.Command, _ []string) error {
		buf := make([]byte, 4)
		_, _ = cmd.InOrStdin().Read(buf)
		read = true
		return WriteJSON(cmd.OutOrStdout(), struct {
			OK bool `json:"ok"`
		}{OK: true})
	}}
	root.PersistentFlags().Bool(cli.FlagYes, true, "author approval")
	root.SetArgs([]string{"--format=json"})
	code := Execute(
		context.Background(),
		root,
		WithStdin(strings.NewReader("data")),
		WithStdout(&stdout),
		WithStderr(
			&stderr,
		),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
		WithVersion("v1"),
	)
	if code != ExitSuccess || !read || stdout.Len() == 0 || stderr.Len() != 0 {
		t.Fatalf(
			"code=%d read=%v stdout=%q stderr=%q",
			code,
			read,
			stdout.String(),
			stderr.String(),
		)
	}
}

// TestExecuteRoutesGuardAuditLinesToConfiguredStderr proves Guard's internal
// NewLogger(ctx) call (which passes no WithWriter option) picks up the writer
// Execute resolved via WithStderr, rather than escaping to the raw process
// os.Stderr. Before logcore.WithDiagnosticWriter was threaded through
// Execute's ctx, this buffer stayed empty even though the audit lines really
// fired — they landed on the real process stderr instead.
func TestExecuteRoutesGuardAuditLinesToConfiguredStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, guardErr := Guard(cmd.Context(), func(context.Context) error { return nil })
			if guardErr != nil {
				return guardErr
			}
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}

	records := decodeLogRecords(t, stderr.String())
	if len(records) != 2 {
		t.Fatalf(
			"log records captured on the configured stderr = %d, want 2 (Guard start + outcome); stderr=%q",
			len(records),
			stderr.String(),
		)
	}
	if records[0]["message"] != "ax: about to run effect" {
		t.Fatalf("records[0] message = %v, want %q", records[0]["message"], "ax: about to run effect")
	}
	if records[1]["message"] != "ax: effect succeeded" {
		t.Fatalf("records[1] message = %v, want %q", records[1]["message"], "ax: effect succeeded")
	}
}

// TestExecuteRebindsPreContextedSubcommand verifies that Execute's diagnostic
// writer remains authoritative when an adopting CLI cached a context on the
// selected Cobra subcommand before execution.
func TestExecuteRebindsPreContextedSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	root := &cobra.Command{Use: "app"}
	sub := &cobra.Command{
		Use: "sub",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := Guard(cmd.Context(), func(context.Context) error { return nil })
			if err != nil {
				return err
			}
			return WriteJSON(cmd.OutOrStdout(), struct {
				OK bool `json:"ok"`
			}{OK: true})
		},
	}
	sub.SetContext(context.Background())
	root.AddCommand(sub)
	root.SetArgs([]string{"sub"})

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	records := decodeLogRecords(t, stderr.String())
	if len(records) != 2 {
		t.Fatalf("configured stderr audit records = %d, want 2; stderr=%q", len(records), stderr.String())
	}
}

func decodeLogRecords(t *testing.T, logs string) []map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(logs), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line was not JSON: %v; line=%q", err, line)
		}
		records = append(records, record)
	}
	return records
}

func TestExecuteWritesErrorsToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return NewError(
				cmd.Context(),
				"validation_error",
				"bad input",
				WithErrorExitCode(ExitValidation),
			)
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithVersion("v0.1.0"),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitValidation {
		t.Fatalf("Execute exit code = %d, want %d", code, ExitValidation)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var got map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("stderr was not JSON: %v", err)
	}
	if got["error_code"] != "validation_error" {
		t.Fatalf("error_code = %v, want validation_error", got["error_code"])
	}
	if got["tool"] != "app" {
		t.Fatalf("tool = %v, want app", got["tool"])
	}
	if got["version"] != "v0.1.0" {
		t.Fatalf("version = %v, want v0.1.0", got["version"])
	}
}

// TestExecuteResolvesVersionWhenWithVersionOmitted verifies that Execute never
// ships an empty version to agent-visible surfaces: when the caller does not
// pass WithVersion, the error envelope and __schema must carry the
// ResolveVersion build-info/vcs fallback, which is non-empty by contract.
func TestExecuteResolvesVersionWhenWithVersionOmitted(t *testing.T) {
	t.Run("error envelope", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		root := &cobra.Command{
			Use: "app",
			RunE: func(cmd *cobra.Command, _ []string) error {
				return NewError(
					cmd.Context(),
					"validation_error",
					"bad input",
					WithErrorExitCode(ExitValidation),
				)
			},
		}

		code := Execute(
			context.Background(),
			root,
			WithStdout(&stdout),
			WithStderr(&stderr),
			WithEnv(func(string) string { return "" }),
			WithStdoutIsTTY(false),
		)

		if code != ExitValidation {
			t.Fatalf("Execute exit code = %d, want %d", code, ExitValidation)
		}
		var got map[string]any
		if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
			t.Fatalf("stderr was not JSON: %v", err)
		}
		if got["version"] == "" || got["version"] == nil {
			t.Fatalf(
				"error envelope version = %v, want non-empty ResolveVersion fallback",
				got["version"],
			)
		}
	})

	t.Run("schema", func(t *testing.T) {
		var stdout bytes.Buffer
		var stderr bytes.Buffer

		root := &cobra.Command{
			Use:   "app",
			Short: "test app",
		}
		root.SetArgs([]string{"__schema"})

		code := Execute(
			context.Background(),
			root,
			WithStdout(&stdout),
			WithStderr(&stderr),
			WithEnv(func(string) string { return "" }),
			WithStdoutIsTTY(false),
		)

		if code != ExitSuccess {
			t.Fatalf(
				"Execute exit code = %d, want %d; stderr=%s",
				code,
				ExitSuccess,
				stderr.String(),
			)
		}
		var got Schema
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("stdout was not schema JSON: %v", err)
		}
		if got.Version == "" {
			t.Fatal("__schema version is empty, want non-empty ResolveVersion fallback")
		}
	})
}

// TestExecuteDoesNotMutateCallerError verifies the non-mutation guarantee:
// normalizing a caller-returned *Error (filling trace_id, tool, version) must
// not modify the caller's value, while the emitted envelope still carries the
// normalized fields and the exit code still maps from the original error.
func TestExecuteDoesNotMutateCallerError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	callerErr := NewError(
		context.Background(),
		"validation_error",
		"bad input",
		WithErrorExitCode(ExitValidation),
	)
	// NewError stamps ZeroTraceID for a span-less context at construction;
	// normalization must leave that value — and the empty Tool/Version — as-is.
	wantTraceID := callerErr.TraceID

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return callerErr
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithVersion("v0.1.0"),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitValidation {
		t.Fatalf("Execute exit code = %d, want %d", code, ExitValidation)
	}
	if callerErr.TraceID != wantTraceID {
		t.Fatalf(
			"caller error TraceID mutated to %q, want unchanged %q",
			callerErr.TraceID,
			wantTraceID,
		)
	}
	if callerErr.Tool != "" {
		t.Fatalf("caller error Tool mutated to %q, want unchanged (empty)", callerErr.Tool)
	}
	if callerErr.Version != "" {
		t.Fatalf("caller error Version mutated to %q, want unchanged (empty)", callerErr.Version)
	}

	var got map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("stderr was not JSON: %v", err)
	}
	if got["tool"] != "app" {
		t.Fatalf("envelope tool = %v, want normalized value app", got["tool"])
	}
	if got["version"] != "v0.1.0" {
		t.Fatalf("envelope version = %v, want normalized value v0.1.0", got["version"])
	}
	if got["trace_id"] == "" || got["trace_id"] == nil {
		t.Fatalf("envelope trace_id = %v, want normalized non-empty value", got["trace_id"])
	}
}

// TestExecuteErrorSpanStatusDescriptionCarriesMessage verifies that a failed
// command sets the root span's error status description to the error message,
// not an empty string. The AX_OTEL_DEBUG exporter serializes the span status
// to stderr, where the description must appear.
func TestExecuteErrorSpanStatusDescriptionCarriesMessage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return NewError(
				cmd.Context(),
				"validation_error",
				"bad input",
				WithErrorExitCode(ExitValidation),
			)
		},
	}

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithEnv(func(key string) string {
			if key == "AX_OTEL_DEBUG" {
				return "1"
			}
			return ""
		}),
		WithStdoutIsTTY(false),
	)

	if code != ExitValidation {
		t.Fatalf("Execute exit code = %d, want %d", code, ExitValidation)
	}
	if !strings.Contains(stderr.String(), `"Description": "bad input"`) {
		t.Fatalf(
			"stderr = %q, want span status description carrying the error message",
			stderr.String(),
		)
	}
}

func TestExecuteSchemaCommandWritesStdout(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app __schema",
	}
	root.SetArgs([]string{"__schema"})

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithVersion("v0.1.0"),
		WithEnv(func(string) string { return "" }),
		WithStdoutIsTTY(false),
	)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got Schema
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout was not schema JSON: %v", err)
	}
	if got.Tool != "app" {
		t.Fatalf("Tool = %q, want app", got.Tool)
	}
}
