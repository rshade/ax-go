package contract

import (
	"context"
	"encoding/json"
	"io"
)

const (
	// ZeroTraceID is a valid zero-value W3C trace ID for no-active-span cases.
	ZeroTraceID = "00000000000000000000000000000000"
	// ZeroSpanID is a valid zero-value W3C span ID for no-active-span cases.
	ZeroSpanID = "0000000000000000"
)

// Metadata carries common machine-readable envelope fields.
type Metadata struct {
	TraceID        string `json:"trace_id"`
	SpanID         string `json:"span_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	DryRun         bool   `json:"dry_run,omitempty"`
}

// Envelope is the standard bounded JSON success payload shape.
type Envelope[T any] struct {
	Data T        `json:"data"`
	Meta Metadata `json:"meta"`
}

// NewEnvelope wraps data with standard AX metadata from ctx.
func NewEnvelope[T any](ctx context.Context, data T) Envelope[T] {
	return Envelope[T]{
		Data: data,
		Meta: MetadataFromContext(ctx),
	}
}

// WriteJSON writes v as strict minified JSON followed by a newline.
func WriteJSON(w io.Writer, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(payload, '\n'))
	return err
}

// WriteJSONLine writes a single NDJSON line.
func WriteJSONLine(w io.Writer, v any) error {
	return WriteJSON(w, v)
}

func normalizeMetadata(metadata Metadata) Metadata {
	if metadata.TraceID == "" {
		metadata.TraceID = ZeroTraceID
	}
	if metadata.SpanID == "" {
		metadata.SpanID = ZeroSpanID
	}
	return metadata
}
