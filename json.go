package ax

import (
	"context"
	"encoding/json"
	"io"
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
	traceID, spanID := traceIDs(ctx)
	meta := Metadata{
		TraceID: traceID,
		SpanID:  spanID,
		DryRun:  DryRunFromContext(ctx),
	}
	if key, ok := IdempotencyKeyFromContext(ctx); ok {
		meta.IdempotencyKey = key
	}
	return Envelope[T]{
		Data: data,
		Meta: meta,
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
