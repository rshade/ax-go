package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

	code := run(
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

	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
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
