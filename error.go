package ax

import (
	"context"
	"io"

	"github.com/rshade/ax-go/contract"
)

const (
	// ErrorSchemaVersion is the current SemVer version of the error envelope.
	ErrorSchemaVersion = contract.ErrorSchemaVersion
)

// Error is the structured error envelope emitted to stderr.
type Error = contract.Error

// ErrorOption configures a structured Error.
type ErrorOption = contract.ErrorOption

// NewError builds a structured error envelope using trace information from ctx.
func NewError(ctx context.Context, code, message string, opts ...ErrorOption) *Error {
	return contract.NewError(withTraceMetadata(ctx), code, message, opts...)
}

// WithErrorTool sets the emitting tool name.
func WithErrorTool(tool string) ErrorOption {
	return contract.WithErrorTool(tool)
}

// WithErrorVersion sets the emitting tool version.
func WithErrorVersion(version string) ErrorOption {
	return contract.WithErrorVersion(version)
}

// WithActionableFix sets a best-effort remediation hint.
func WithActionableFix(fix string) ErrorOption {
	return contract.WithActionableFix(fix)
}

// WithErrorContext merges domain-specific context fields into the envelope.
func WithErrorContext(fields map[string]any) ErrorOption {
	return contract.WithErrorContext(fields)
}

// WithSuggestions sets optional candidate recovery actions.
func WithSuggestions(suggestions ...string) ErrorOption {
	return contract.WithSuggestions(suggestions...)
}

// WithErrorExitCode sets the deterministic process exit code.
func WithErrorExitCode(code int) ErrorOption {
	return contract.WithErrorExitCode(code)
}

// WithErrorCause attaches the underlying source error to the envelope.
func WithErrorCause(err error) ErrorOption {
	return contract.WithErrorCause(err)
}

// WriteError writes err as a strict minified JSON error envelope followed by a newline.
func WriteError(w io.Writer, err error) error {
	return contract.WriteError(w, err)
}

// ErrorExitCode maps an error to the deterministic ax-go process exit code.
func ErrorExitCode(err error) int {
	return contract.ErrorExitCode(err)
}
