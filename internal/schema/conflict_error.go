package schema

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/rshade/ax-go/contract"
)

// maxKeyInMessage bounds how much of an adopter-supplied key an error message
// echoes, so a too_long URI cannot inflate the envelope on stderr.
const maxKeyInMessage = 128

// CorruptAnnotationFix is the actionable_fix for a hand-written declaration
// annotation that does not project cleanly.
const CorruptAnnotationFix = "Declare prompts and resources only with DeclarePrompt and " +
	"DeclareResource; never write the github.com/rshade/ax-go/schema/* annotations by hand."

// TreeDeclarationError maps a problem found by walking the tree — a duplicate
// key or a corrupt annotation — to the validation_error envelope (exit 2) that
// __schema and the live mcp-server both return, so the two can never word the
// same failure differently.
func TreeDeclarationError(ctx context.Context, conflict *Conflict) error {
	if conflict.Reason == ReasonCorruptAnnotation {
		return contract.NewError(
			ctx,
			"validation_error",
			fmt.Sprintf("corrupt %s annotation on command %q", conflict.Kind, conflict.Commands[0]),
			contract.WithErrorExitCode(contract.ExitValidation),
			contract.WithActionableFix(CorruptAnnotationFix),
			contract.WithErrorContext(map[string]any{
				"kind":     string(conflict.Kind),
				"key":      conflict.Key,
				"reason":   string(conflict.Reason),
				"commands": conflict.Commands,
			}),
		)
	}

	var label string
	switch conflict.Kind {
	case KindPrompt:
		label = "prompt name"
	case KindResource:
		label = "resource URI"
	case KindFlagEnum, KindFlagExample, KindCapability:
		// Flag and capability declarations are per-command and never conflict.
		label = string(conflict.Kind)
	default:
		label = string(conflict.Kind)
	}
	return contract.NewError(
		ctx,
		"validation_error",
		fmt.Sprintf("duplicate %s %q declared on more than one command", label, TruncateKey(conflict.Key)),
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithActionableFix("Rename or remove one of the duplicate declarations."),
		contract.WithErrorContext(map[string]any{
			"kind":     string(conflict.Kind),
			"key":      conflict.Key,
			"commands": conflict.Commands,
		}),
	)
}

// TruncateKey shortens key to at most maxKeyInMessage bytes on a rune
// boundary, marking the cut with an ellipsis.
func TruncateKey(key string) string {
	if len(key) <= maxKeyInMessage {
		return key
	}
	cut := maxKeyInMessage
	for cut > 0 && !utf8.RuneStart(key[cut]) {
		cut--
	}
	return key[:cut] + "…"
}
