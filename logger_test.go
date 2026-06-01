package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
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
