package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	var got Error
	if decodeErr := json.Unmarshal(stderr.Bytes(), &got); decodeErr != nil {
		t.Fatalf("stderr did not contain JSON: %v", decodeErr)
	}
	if got.ErrorCode != "validation_error" {
		t.Fatalf("ErrorCode = %q, want validation_error", got.ErrorCode)
	}
	if got.TraceID != ZeroTraceID {
		t.Fatalf("TraceID = %q, want %q", got.TraceID, ZeroTraceID)
	}
	if got.Tool != "app" || got.Version != "v0.1.0" {
		t.Fatalf("tool/version = %q/%q, want app/v0.1.0", got.Tool, got.Version)
	}
	if got.SchemaVersion != ErrorSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", got.SchemaVersion, ErrorSchemaVersion)
	}
	if ErrorExitCode(err) != ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", ErrorExitCode(err), ExitValidation)
	}
	if !bytes.HasSuffix(stderr.Bytes(), []byte("\n")) {
		t.Fatalf("WriteError output %q, want trailing newline", stderr.String())
	}
}

func TestNewErrorUsesMetadataTraceID(t *testing.T) {
	ctx := WithMetadata(context.Background(), Metadata{TraceID: "0102030405060708090a0b0c0d0e0f10"})
	err := NewError(ctx, "validation_error", "bad input")
	if err.TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Fatalf("TraceID = %q, want metadata trace", err.TraceID)
	}
}

func TestErrorExitCodeContextErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "wrapped deadline exceeded",
			err:  fmt.Errorf("read config: %w", context.DeadlineExceeded),
			want: ExitNetwork,
		},
		{
			name: "wrapped canceled",
			err:  fmt.Errorf("read config: %w", context.Canceled),
			want: ExitInternal,
		},
		{
			name: "plain non context error",
			err:  errors.New("plain failure"),
			want: ExitInternal,
		},
		{
			name: "envelope exit code wins over cause chain",
			err: NewError(
				context.Background(),
				"config_invalid",
				"decode failed",
				WithErrorExitCode(ExitValidation),
				WithErrorCause(fmt.Errorf("read config: %w", context.DeadlineExceeded)),
			),
			want: ExitValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorExitCode(tt.err); got != tt.want {
				t.Fatalf("ErrorExitCode = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestErrorCauseChain(t *testing.T) {
	sentinel := errors.New("underlying decode failure")
	err := NewError(
		context.Background(),
		"config_invalid",
		"decode failed",
		WithErrorCause(sentinel),
	)
	if !errors.Is(err, sentinel) {
		t.Fatal("errors.Is(err, sentinel) = false, want cause reachable")
	}
}
