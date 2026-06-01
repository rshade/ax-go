package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestWriteErrorEnvelope(t *testing.T) {
	err := NewError(
		context.Background(),
		"validation_error",
		"bad input",
		WithErrorTool("app"),
		WithErrorVersion("v0.1.0"),
		WithActionableFix("fix the input"),
		WithErrorContext(map[string]any{"field": "name"}),
		WithSuggestions("retry with --help"),
		WithErrorExitCode(ExitValidation),
	)

	var stderr bytes.Buffer
	if writeErr := WriteError(&stderr, err); writeErr != nil {
		t.Fatalf("WriteError returned error: %v", writeErr)
	}
	assertGolden(t, "testdata/error_envelope.golden.json", stderr.Bytes())

	var got map[string]any
	if decodeErr := json.Unmarshal(stderr.Bytes(), &got); decodeErr != nil {
		t.Fatalf("stderr did not contain JSON: %v", decodeErr)
	}

	required := map[string]string{
		"error_code":     "validation_error",
		"message":        "bad input",
		"trace_id":       ZeroTraceID,
		"tool":           "app",
		"version":        "v0.1.0",
		"schema_version": ErrorSchemaVersion,
		"actionable_fix": "fix the input",
	}
	for key, want := range required {
		if got[key] != want {
			t.Fatalf("%s = %v, want %q", key, got[key], want)
		}
	}

	if ErrorExitCode(err) != ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", ErrorExitCode(err), ExitValidation)
	}
}
