package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/logging"
)

// TestEmitWritesCorrelatedLine asserts the example emits exactly one JSON line
// carrying its declared labels and the trace-correlation fields. With no active
// span the identifiers are the zero-value valid hex constants rather than absent,
// so a consumer parser never branches on absence.
func TestEmitWritesCorrelatedLine(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewLogger(
		context.Background(),
		logging.WithLoggerWriter(&buf),
		logging.WithLoggerLabels(logging.Labels{
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
//
// This is the assertion that has to be made against the actual entry point rather
// than against emit: main is where the writer defaults are exercised, and a
// default that pointed at stdout would corrupt every agent parsing a command's
// payload while every writer-injected test stayed green.
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
