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

// ErrorExitCode maps an error to the deterministic ax-go process exit code.
func ErrorExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}

	var axErr *Error
	if errors.As(err, &axErr) {
		return axErr.ExitCode()
	}

	return ExitInternal
}
