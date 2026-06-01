package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	ax "github.com/rshade/ax-go"
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
	if logLine["application"] != "ax-integration" {
		t.Fatalf("application label = %v, want ax-integration", logLine["application"])
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
		[]string{"fail", "--format=json"},
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
	if got.Tool != "ax-integration" {
		t.Fatalf("tool = %q, want ax-integration", got.Tool)
	}
	if got.Version != version {
		t.Fatalf("version = %q, want %q", got.Version, version)
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
	if got.Tool != "ax-integration" {
		t.Fatalf("tool = %q, want ax-integration", got.Tool)
	}
	if len(got.Command.Commands) == 0 {
		t.Fatal("schema did not include subcommands")
	}
}
