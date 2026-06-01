package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

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

func TestExecuteWritesErrorsToStderr(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return NewError(cmd.Context(), "validation_error", "bad input", WithErrorExitCode(ExitValidation))
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
