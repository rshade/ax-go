package ax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	// ErrorSchemaVersion is the current SemVer version of the error envelope.
	ErrorSchemaVersion = "1.0.0"
)

// Error is the ADR-0002 structured error envelope emitted to stderr.
type Error struct {
	ErrorCode     string         `json:"error_code"`
	Message       string         `json:"message"`
	TraceID       string         `json:"trace_id"`
	Tool          string         `json:"tool"`
	Version       string         `json:"version"`
	SchemaVersion string         `json:"schema_version"`
	ActionableFix string         `json:"actionable_fix,omitempty"`
	Context       map[string]any `json:"context,omitempty"`
	Suggestions   []string       `json:"suggestions,omitempty"`

	exitCode int
	cause    error
}

// NewError builds a structured error envelope using trace information from ctx.
func NewError(ctx context.Context, code, message string, opts ...ErrorOption) *Error {
	e := &Error{
		ErrorCode:     code,
		Message:       message,
		TraceID:       TraceIDFromContext(ctx),
		SchemaVersion: ErrorSchemaVersion,
		exitCode:      ExitInternal,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Error returns the human-readable error message.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ExitCode returns the deterministic process exit code associated with e.
func (e *Error) ExitCode() int {
	if e == nil || e.exitCode == 0 {
		return ExitInternal
	}
	return e.exitCode
}

// Unwrap returns the underlying cause attached via WithErrorCause, or nil when
// no cause was attached. It lets errors.Is and errors.As traverse from the
// envelope to the source error (for example, a config decode failure) without
// the cause ever appearing in the JSON envelope.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// ErrorOption configures a structured Error.
type ErrorOption func(*Error)

// WithErrorTool sets the emitting tool name.
func WithErrorTool(tool string) ErrorOption {
	return func(e *Error) {
		e.Tool = tool
	}
}

// WithErrorVersion sets the emitting tool version.
func WithErrorVersion(version string) ErrorOption {
	return func(e *Error) {
		e.Version = version
	}
}

// WithActionableFix sets a best-effort remediation hint.
func WithActionableFix(fix string) ErrorOption {
	return func(e *Error) {
		e.ActionableFix = fix
	}
}

// WithErrorContext merges domain-specific context fields into the envelope.
func WithErrorContext(fields map[string]any) ErrorOption {
	return func(e *Error) {
		if len(fields) == 0 {
			return
		}
		if e.Context == nil {
			e.Context = make(map[string]any, len(fields))
		}
		for key, value := range fields {
			e.Context[key] = value
		}
	}
}

// WithSuggestions sets optional candidate recovery actions.
func WithSuggestions(suggestions ...string) ErrorOption {
	return func(e *Error) {
		e.Suggestions = append(e.Suggestions, suggestions...)
	}
}

// WithErrorExitCode sets the deterministic process exit code.
func WithErrorExitCode(code int) ErrorOption {
	return func(e *Error) {
		e.exitCode = code
	}
}

// WithErrorCause attaches the underlying source error to the envelope so
// errors.Is and errors.As reach it through Unwrap. The cause is never
// serialized into the JSON envelope; it exists only for in-process callers.
// Never attach a context.Canceled or context.DeadlineExceeded cause: context
// errors are returned raw (FR-010) so their sentinels map to exit codes only
// when no explicit envelope classification exists.
func WithErrorCause(err error) ErrorOption {
	return func(e *Error) {
		e.cause = err
	}
}

// WriteError writes err as a strict minified JSON error envelope followed by a newline.
func WriteError(w io.Writer, err error) error {
	if err == nil {
		return nil
	}

	var axErr *Error
	if !errors.As(err, &axErr) {
		axErr = NewError(context.Background(), "internal_error", err.Error())
	}

	payload, marshalErr := json.Marshal(axErr)
	if marshalErr != nil {
		return fmt.Errorf("marshal error envelope: %w", marshalErr)
	}
	if _, writeErr := w.Write(append(payload, '\n')); writeErr != nil {
		return writeErr
	}
	return nil
}

// ErrorExitCode maps an error to the deterministic ax-go process exit code:
// nil maps to ExitSuccess (0); an *Error anywhere in the chain maps to its
// explicit ExitCode, winning over any sentinel buried in its cause chain; a
// non-envelope error wrapping context.DeadlineExceeded maps to ExitNetwork (3)
// and one wrapping context.Canceled to ExitInternal (1); anything else maps to
// ExitInternal (1).
func ErrorExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}

	var axErr *Error
	if errors.As(err, &axErr) {
		return axErr.ExitCode()
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return ExitNetwork
	}
	if errors.Is(err, context.Canceled) {
		return ExitInternal
	}

	return ExitInternal
}
