package ax

const (
	// ExitSuccess indicates successful completion.
	ExitSuccess = 0
	// ExitInternal indicates an unknown or internal error.
	ExitInternal = 1
	// ExitValidation indicates invalid input or failed validation.
	ExitValidation = 2
	// ExitNetwork indicates a network failure or timeout.
	ExitNetwork = 3
	// ExitAuth indicates an authentication or permission failure.
	ExitAuth = 4
)
