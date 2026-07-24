package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/contract"
)

// TestEmitWritesCorrelatedLine mirrors its counterpart in examples/logging. The
// two assertions are deliberately identical, because this program exists to be
// identical apart from the surface it imports; a divergence between the two tests
// would mean the size ratio is measuring more than the import boundary.
func TestEmitWritesCorrelatedLine(t *testing.T) {
	var buf bytes.Buffer
	log := ax.NewLogger(
		context.Background(),
		ax.WithLoggerWriter(&buf),
		ax.WithLoggerLabels(ax.Labels{
			Application: appName,
			Environment: envName,
		}),
	)

	emit(context.Background(), log)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("emitted %d lines, want 1: %q", len(lines), buf.String())
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("log line was not JSON: %v", err)
	}

	want := map[string]any{
		"application": appName,
		"environment": envName,
		"stage":       "startup",
		"message":     "ready",
		"level":       "info",
		"trace_id":    contract.ZeroTraceID,
		"span_id":     contract.ZeroSpanID,
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %v, want %v", key, got[key], value)
		}
	}
}

// TestMainWritesOnlyToDiagnosticStream runs the real main and asserts the payload
// stream stays untouched (Constitution Principle I).
func TestMainWritesOnlyToDiagnosticStream(t *testing.T) {
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutW, stderrW //nolint:reassign // redirect the process streams to prove the DEFAULT writer lands on stderr; a writer-injected test cannot observe the default it overrides
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origStdout, origStderr //nolint:reassign // restore the process streams after capture
	})

	main()

	if closeErr := stdoutW.Close(); closeErr != nil {
		t.Fatalf("close stdout writer: %v", closeErr)
	}
	if closeErr := stderrW.Close(); closeErr != nil {
		t.Fatalf("close stderr writer: %v", closeErr)
	}

	stdoutBytes, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if len(stdoutBytes) != 0 {
		t.Errorf("log output leaked to the payload stream: %q", stdoutBytes)
	}
	if !strings.Contains(string(stderrBytes), `"message":"ready"`) {
		t.Errorf("log line absent from the diagnostic stream, got %q", stderrBytes)
	}
}
