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

// Error codes returned by ax-go's command runtime: Execute, Confirm, the
// mcp-server subcommand, and the __schema command. The spellings are frozen
// public contract.
const (
	// ErrorCodeConfirmationRequired reports a confirmation point that Confirm
	// blocked in machine mode because --yes (or the MCP yes argument) was not
	// given. It is returned even under --dry-run. The process exits 2.
	ErrorCodeConfirmationRequired = "confirmation_required"
	// ErrorCodeInternalError reports an error that carried no envelope of its
	// own, wrapped by WriteError, Execute, or the MCP dispatcher, and a
	// recovered panic during MCP tool dispatch. The process exits 1, or 3 when
	// Execute or the dispatcher wraps a context deadline.
	ErrorCodeInternalError = "internal_error"
	// ErrorCodeValidationError reports bad input caught by ax-go: an invalid
	// output mode, a flag value outside its declared enum, an unknown __schema
	// --as format, a duplicate or corrupt prompt or resource declaration found
	// when the tree is walked, an invalid MCP tool call, and mcp-server
	// startup failures. The process exits 2.
	ErrorCodeValidationError = "validation_error"
)

// ErrorCodeInvalidSchemaDeclaration reports a schema declaration
// (DeclareFlagEnum, DeclareFlagExample, DeclareCapability, DeclarePrompt,
// DeclareResource) that the declaring function rejected, including a call on a
// nil command. It is returned while the adopter builds the command tree, before
// any command runs, so it is authoring-time and absent from KnownErrorCodes.
// The process exits 2 if the adopter returns it from a run.
const ErrorCodeInvalidSchemaDeclaration = "invalid_schema_declaration"

// KnownErrorCodes returns every error_code that ax-go library code can return
// from a command run, meaning anything returned between the start of command
// dispatch and the return of Execute or an MCP tool call. That includes codes
// from public helper packages an adopter calls inside a command, such as config.
// It excludes adopter-defined codes, codes returned only while the command tree
// is built (invalid_schema_declaration), and codes emitted only by this
// repository's own gate tools.
//
// Every entry is one of this package's ErrorCode* constants; each emit site in
// ax-go names the same constant, and the module's drift guard fails when an
// emit site uses a code that is not registered here or in the authoring-time
// list.
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
		ErrorCodeConfirmationRequired,
		ErrorCodeInternalError,
		ErrorCodeValidationError,
		ErrorCodeWarningsAsErrors,
	}
}

// authoringTimeErrorCodes returns the registered codes that ax-go returns only
// while the adopter builds the command tree, before Execute dispatches a
// command. Spec 032 FR-001 limits KnownErrorCodes to codes a command run can
// return, so these are excluded from it and from __schema's known_codes: an
// agent driving the built CLI never receives them. They are registered here so
// that emit sites name a constant and the drift guard
// (TestErrorCodesAreRegistered) accepts them.
//
// The result is sorted byte-wise with no duplicates and is a new slice on every
// call.
func authoringTimeErrorCodes() []string {
	return []string{
		ErrorCodeInvalidSchemaDeclaration,
	}
}
