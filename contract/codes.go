package contract

// ErrorCodeWarningsAsErrors is the stderr error_code written when --strict
// escalates one or more warnings. The process exits 2.
const ErrorCodeWarningsAsErrors = "warnings_as_errors"

// Error codes returned by the public config package. Each maps to exit 2
// (ExitValidation). The spellings are frozen public contract.
const (
	// ErrorCodeConfigInvalid reports config input that is not valid Hujson,
	// or that does not decode into the destination type.
	ErrorCodeConfigInvalid = "config_invalid"
	// ErrorCodeConfigMaxBytesInvalid reports a config read cap that is
	// negative or above the package's maximum config byte ceiling.
	ErrorCodeConfigMaxBytesInvalid = "config_max_bytes_invalid"
	// ErrorCodeConfigOptionInvalid reports a nil config option.
	ErrorCodeConfigOptionInvalid = "config_option_invalid"
	// ErrorCodeConfigPatchInvalid reports a patch that is not a valid RFC 6902
	// document, or a patch operation that failed against the config.
	ErrorCodeConfigPatchInvalid = "config_patch_invalid"
	// ErrorCodeConfigTooLarge reports config input larger than the read cap.
	// It is returned before the input is parsed.
	ErrorCodeConfigTooLarge = "config_too_large"
)

// KnownErrorCodes returns every error_code that ax-go library code can return
// from a command run, meaning anything returned between the start of command
// dispatch and the return of Execute or an MCP tool call. That includes codes
// from public helper packages an adopter calls inside a command, such as config.
// It excludes adopter-defined codes, codes returned only while the command tree
// is built (invalid_schema_declaration), and codes emitted only by this
// repository's own gate tools.
//
// The result is sorted byte-wise with no duplicates and is a new slice on every
// call, so the caller may retain or modify it. __schema publishes it as
// error_envelope.known_codes.
func KnownErrorCodes() []string {
	return []string{
		ErrorCodeConfigInvalid,
		ErrorCodeConfigMaxBytesInvalid,
		ErrorCodeConfigOptionInvalid,
		ErrorCodeConfigPatchInvalid,
		ErrorCodeConfigTooLarge,
		"confirmation_required",
		"internal_error",
		"validation_error",
		ErrorCodeWarningsAsErrors,
	}
}
