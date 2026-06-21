package ax

import "github.com/rshade/ax-go/contract"

const (
	// ExitSuccess indicates successful completion.
	ExitSuccess = contract.ExitSuccess
	// ExitInternal indicates an unknown or internal error.
	ExitInternal = contract.ExitInternal
	// ExitValidation indicates invalid input or failed validation.
	ExitValidation = contract.ExitValidation
	// ExitNetwork indicates a network failure or timeout.
	ExitNetwork = contract.ExitNetwork
	// ExitAuth indicates an authentication or permission failure.
	ExitAuth = contract.ExitAuth
)
