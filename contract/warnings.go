package contract

import "strings"

// ErrorCodeWarningsAsErrors is the stderr error_code written when --strict
// escalates one or more warnings. The process exits 2.
const ErrorCodeWarningsAsErrors = "warnings_as_errors"

// KnownErrorCodes returns the runtime's own error_code values, sorted.
// Adopter-defined codes are not included. The caller may retain the slice.
func KnownErrorCodes() []string {
	return []string{
		"confirmation_required",
		"internal_error",
		"validation_error",
		ErrorCodeWarningsAsErrors,
	}
}

// WithWarnings returns a copy of env whose warnings are the caller's list
// in that order. A warning with a blank code or a blank message is dropped.
// An empty result omits the field.
func WithWarnings[T any](env Envelope[T], warnings ...Warning) Envelope[T] {
	kept := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		if strings.TrimSpace(warning.Code) == "" || strings.TrimSpace(warning.Message) == "" {
			continue
		}
		kept = append(kept, Warning{Code: warning.Code, Message: warning.Message})
	}
	if len(kept) == 0 {
		env.Warnings = nil
		return env
	}
	env.Warnings = kept
	return env
}
