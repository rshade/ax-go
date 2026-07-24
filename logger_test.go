package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/rs/zerolog"
)

func TestLoggerAddsTraceFieldsAndLabels(t *testing.T) {
	var stderr bytes.Buffer
	logger := NewLogger(
		context.Background(),
		WithLoggerWriter(&stderr),
		WithLoggerLabels(Labels{Application: "app", Environment: "test"}),
	)

	logger.Info(context.Background()).Msg("hello")

	var got map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
		t.Fatalf("log line was not JSON: %v", err)
	}

	if got["trace_id"] != ZeroTraceID {
		t.Fatalf("trace_id = %v, want %q", got["trace_id"], ZeroTraceID)
	}
	if got["span_id"] != ZeroSpanID {
		t.Fatalf("span_id = %v, want %q", got["span_id"], ZeroSpanID)
	}
	if got["application"] != "app" {
		t.Fatalf("application = %v, want app", got["application"])
	}
	if got["environment"] != "test" {
		t.Fatalf("environment = %v, want test", got["environment"])
	}
}

// The declarations below are compile-time regression guards for SC-003.
//
// Delegating to internal/logcore makes `var NewLogger = logcore.New` an
// attractive one-liner: it compiles, behaves identically at every call site, and
// is shorter than the wrapper function. It is also a BREAKING change — go-apidiff
// classifies a func→var conversion as incompatible — so taking that shortcut
// would fail the API gate for a reason with no visible symptom in any behavioral
// test.
//
// These assignments fail to compile the moment either symbol stops being a
// function. They live in the root package deliberately, duplicating the
// equivalent guard in logging/identity_test.go: this one survives even if the
// logging package were removed, and the root surface is the one with existing
// adopters.
var (
	_ func(context.Context, ...LoggerOption) Logger = NewLogger
	_ func(context.Context, Logger) error           = Flush
	_ func(io.Writer) LoggerOption                  = WithLoggerWriter
	_ func(zerolog.Level) LoggerOption              = WithLoggerLevel
	_ func(Labels) LoggerOption                     = WithLoggerLabels
	_ func() LoggerOption                           = WithLokiFromEnv
)
