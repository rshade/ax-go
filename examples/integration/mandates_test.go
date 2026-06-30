package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	ax "github.com/rshade/ax-go"
)

// schemaCommandName is the reserved discovery command ax.Execute mounts; the
// example never declares it, so the tests name it locally.
const schemaCommandName = "__schema"

// runCapture executes the integration command with a pinned entity ID and the
// supplied env lookup, returning the exit code plus captured stdout/stderr. The
// fixed entity ID keeps the success payload deterministic so these tests can
// assert on stable bytes.
func runCapture(t *testing.T, args []string, env func(string) string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runWithEntityID(
		context.Background(),
		args,
		strings.NewReader(""),
		&stdout,
		&stderr,
		env,
		ax.ResolveVersion(version),
		func() (string, error) { return deterministicEntityID, nil },
	)
	return code, stdout.String(), stderr.String()
}

// emptyEnv is an env lookup that reports every variable as unset.
func emptyEnv(string) string { return "" }

// envWith returns an env lookup backed by a fixed map; unlisted keys read empty.
func envWith(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

// nonEmptyLines splits s on newlines and drops blank lines.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestExitCodeMatrix is the single source of truth for Core AX Mandates #2
// (deterministic exit codes 0/1/2/3/4) and #9 (an ax.Error envelope per
// exit-code category). Each error path MUST keep stdout empty (stream
// separation) and emit the ax.Error envelope to stderr.
func TestExitCodeMatrix(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	cases := []struct {
		name           string
		args           []string
		wantCode       int
		wantErrCode    string // "" => success path (payload on stdout)
		wantRetryable  *bool  // nil => do not assert
		wantRetryAfter int64  // expected retry_after_seconds (0 => absent)
	}{
		{
			name:     "success",
			args:     []string{"--format=json", "--idempotency-key=test-key"},
			wantCode: ax.ExitSuccess,
		},
		{
			name:          "validation",
			args:          []string{failCommandName, "--format=json"},
			wantCode:      ax.ExitValidation,
			wantErrCode:   "integration_failure",
			wantRetryable: boolPtr(false),
		},
		{
			name:           "network",
			args:           []string{fetchCommandName, "--format=json"},
			wantCode:       ax.ExitNetwork,
			wantErrCode:    "upstream_unreachable",
			wantRetryable:  boolPtr(true),
			wantRetryAfter: fetchRetryAfterSeconds,
		},
		{
			name:          "auth",
			args:          []string{authzCommandName, "--format=json"},
			wantCode:      ax.ExitAuth,
			wantErrCode:   "permission_denied",
			wantRetryable: boolPtr(false),
		},
		{
			name:        "internal",
			args:        []string{crashCommandName, "--format=json"},
			wantCode:    ax.ExitInternal,
			wantErrCode: "internal_error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCapture(t, tc.args, emptyEnv)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr=%s", code, tc.wantCode, stderr)
			}

			if tc.wantErrCode == "" {
				if stdout == "" {
					t.Fatal("success path wrote no stdout payload")
				}
				return
			}

			if stdout != "" {
				t.Fatalf("error path leaked stdout (stream separation): %q", stdout)
			}

			var gotErr ax.Error
			if err := json.Unmarshal([]byte(stderr), &gotErr); err != nil {
				t.Fatalf("stderr was not an ax.Error envelope: %v; stderr=%s", err, stderr)
			}
			if gotErr.ErrorCode != tc.wantErrCode {
				t.Fatalf("error_code = %q, want %q", gotErr.ErrorCode, tc.wantErrCode)
			}
			if tc.wantRetryable != nil {
				if gotErr.Retryable == nil || *gotErr.Retryable != *tc.wantRetryable {
					t.Fatalf("retryable = %v, want %v", gotErr.Retryable, *tc.wantRetryable)
				}
			}
			if gotErr.RetryAfterSeconds != tc.wantRetryAfter {
				t.Fatalf("retry_after_seconds = %d, want %d", gotErr.RetryAfterSeconds, tc.wantRetryAfter)
			}
		})
	}
}

// TestModePrecedence covers Core AX Mandate #8: output-mode resolution applies
// --format first, then AGENT_MODE. The root payload echoes the resolved mode, so
// these cases are deterministic regardless of the test runner's TTY state.
func TestModePrecedence(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		wantMode string
	}{
		{
			name:     "flag beats AGENT_MODE",
			args:     []string{"--format=human", "--idempotency-key=test-key"},
			env:      map[string]string{"AGENT_MODE": "json"},
			wantMode: ax.ModeHuman.String(),
		},
		{
			name:     "AGENT_MODE used when no flag",
			args:     []string{"--idempotency-key=test-key"},
			env:      map[string]string{"AGENT_MODE": "human"},
			wantMode: ax.ModeHuman.String(),
		},
		{
			name:     "AGENT_MODE json honored",
			args:     []string{"--idempotency-key=test-key"},
			env:      map[string]string{"AGENT_MODE": "json"},
			wantMode: ax.ModeJSON.String(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCapture(t, tc.args, envWith(tc.env))
			if code != ax.ExitSuccess {
				t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr)
			}

			var got ax.Envelope[helloPayload]
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("stdout was not a JSON envelope: %v", err)
			}
			if got.Data.Mode != tc.wantMode {
				t.Fatalf("resolved mode = %q, want %q", got.Data.Mode, tc.wantMode)
			}
		})
	}
}

// TestRunGeneratesIdempotencyKeyWhenAbsent covers Core AX Mandate #6: when no
// --idempotency-key is supplied, ax.Execute auto-generates a UUID v4 and surfaces
// it in the envelope.
func TestRunGeneratesIdempotencyKeyWhenAbsent(t *testing.T) {
	code, stdout, stderr := runCapture(t, []string{"--format=json"}, emptyEnv)
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr)
	}

	var got ax.Envelope[helloPayload]
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout was not a JSON envelope: %v", err)
	}
	key := got.Meta.IdempotencyKey
	if key == "" {
		t.Fatal("no idempotency key surfaced in envelope meta")
	}
	parsed, err := uuid.Parse(key)
	if err != nil {
		t.Fatalf("auto-generated idempotency key %q is not a UUID: %v", key, err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("idempotency key version = %d, want 4", parsed.Version())
	}
}

// TestRunSchemaMCPAdapter covers Core AX Mandate #4: __schema --as=mcp emits the
// MCP-compatible tools list. The static adapter reflects the entire command tree
// (including the reserved __schema command) — distinct from the live mcp-server,
// which filters reserved commands out of tools/list (asserted by
// TestQuickstartAgainstBuiltBinary).
func TestRunSchemaMCPAdapter(t *testing.T) {
	code, stdout, stderr := runCapture(t, []string{"__schema", "--as=mcp"}, emptyEnv)
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr)
	}

	var got ax.MCPSchema
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout was not an MCP schema: %v; stdout=%s", err, stdout)
	}
	names := map[string]bool{}
	for _, tool := range got.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{
		appName + " " + streamCommandName,
		appName + " " + fetchCommandName,
		appName + " " + authzCommandName,
		appName + " " + crashCommandName,
		appName + " " + schemaCommandName,
	} {
		if !names[want] {
			t.Fatalf("mcp tools missing %q; got %v", want, names)
		}
	}
}

// TestLogLinesCarryTraceCorrelation covers Core AX Mandate #10: every stderr log
// line carries trace_id and span_id, and they match the envelope's meta — so a
// log line and the payload it accompanies share one trace context. Without an
// OTel SDK the IDs are the deterministic zero-values; the correlation assertion
// holds either way.
func TestLogLinesCarryTraceCorrelation(t *testing.T) {
	code, stdout, stderr := runCapture(t, []string{"--format=json", "--idempotency-key=test-key"}, emptyEnv)
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr)
	}

	var envelope ax.Envelope[helloPayload]
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("stdout was not a JSON envelope: %v", err)
	}
	wantTrace := envelope.Meta.TraceID
	wantSpan := envelope.Meta.SpanID

	lines := nonEmptyLines(stderr)
	if len(lines) == 0 {
		t.Fatal("no log lines on stderr to correlate")
	}
	for i, line := range lines {
		var logLine map[string]any
		if err := json.Unmarshal([]byte(line), &logLine); err != nil {
			t.Fatalf("log line %d was not JSON: %v", i, err)
		}
		traceID, ok := logLine["trace_id"].(string)
		if !ok {
			t.Fatalf("log line %d missing trace_id: %s", i, line)
		}
		spanID, ok := logLine["span_id"].(string)
		if !ok {
			t.Fatalf("log line %d missing span_id: %s", i, line)
		}
		if traceID != wantTrace {
			t.Fatalf("log line %d trace_id = %q, want envelope %q", i, traceID, wantTrace)
		}
		if spanID != wantSpan {
			t.Fatalf("log line %d span_id = %q, want envelope %q", i, spanID, wantSpan)
		}
	}
}
