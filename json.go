package ax

import (
	"context"
	"io"

	"github.com/rshade/ax-go/contract"
)

// Metadata carries common machine-readable envelope fields.
type Metadata = contract.Metadata

// Envelope is the standard bounded JSON success payload shape.
type Envelope[T any] = contract.Envelope[T]

// NewEnvelope wraps data with standard AX metadata from ctx.
func NewEnvelope[T any](ctx context.Context, data T) Envelope[T] {
	return contract.NewEnvelope(withTraceMetadata(ctx), data)
}

// WriteJSON writes v as strict minified JSON followed by a newline.
func WriteJSON(w io.Writer, v any) error {
	return contract.WriteJSON(w, v)
}

// WriteJSONLine writes a single NDJSON line.
func WriteJSONLine(w io.Writer, v any) error {
	return contract.WriteJSONLine(w, v)
}
