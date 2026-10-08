package schema

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	internalschema "github.com/rshade/ax-go/internal/schema"
)

const invalidDeclarationCode = "invalid_schema_declaration"

// Prompt is a static, server-vended workflow template declared on a command
// with DeclarePrompt. It appears on its declaring command in __schema and in
// the top-level prompts array of __schema --as=mcp.
//
// Name and every argument name must match ^[a-zA-Z0-9_.-]+$, and argument
// names are unique within the prompt. Template is required; each "{{name}}" in
// it must name a declared argument, and any other brace text (including
// "{{ name }}" with spaces) is literal. All text must be valid UTF-8.
// Declarations on a hidden command, or in a hidden subtree, are not projected.
// Phase 1 is a declaration only: the live mcp-server does not serve prompts
// yet.
type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Template    string           `json:"template"`
}

// PromptArgument is one declared input of a Prompt. Names are unique within a
// prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Resource is static, read-only reference context declared on a command with
// DeclareResource. It carries metadata only — there is deliberately no content
// field, and no field can hold a callback or live state: resources never
// expose run records (Constitution Principle VI).
//
// URI is required, at most 2048 bytes, free of whitespace and control
// characters, parseable, and must carry a scheme ("app://docs/x", "urn:app:x").
// Name is required. MIMEType may contain spaces ("text/plain; charset=utf-8")
// but no control characters. All text must be valid UTF-8. Declarations on a
// hidden command, or in a hidden subtree, are not projected.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

// MCPPrompt is a Prompt in the __schema --as=mcp adapter: the MCP prompts/list
// fields plus the ax extension field template.
type MCPPrompt struct {
	Name        string              `json:"name"`
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Arguments   []MCPPromptArgument `json:"arguments,omitempty"`
	Template    string              `json:"template"`
}

// MCPPromptArgument is a PromptArgument in the MCP adapter shape.
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCPResource is a Resource in the MCP resources/list shape (camelCase
// mimeType).
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// DeclarePrompt validates prompt and records a copy of it on cmd, after any
// prompts already declared there; later changes to prompt do not affect the
// record. Other annotations on cmd are preserved.
//
// It returns nil on success. On a nil cmd, an invalid prompt, a prompt name
// already declared on cmd, or an existing prompt annotation that was written by
// hand and does not decode cleanly (reason corrupt_annotation), it returns a
// *contract.Error with error_code invalid_schema_declaration (exit 2) whose
// context names the offending field and a stable reason, and leaves cmd
// unchanged. The same name declared on two different commands is caught when
// the tree is walked: __schema then fails with validation_error (exit 2).
func DeclarePrompt(cmd *cobra.Command, prompt Prompt) error {
	return declarationError(
		internalschema.KindPrompt,
		prompt.Name,
		internalschema.AddPrompt(cmd, toInternalPrompt(prompt)),
	)
}

// DeclareResource validates resource and records a copy of it on cmd, with the
// same ordering, copy, and error contract as DeclarePrompt; resources are keyed
// by URI.
func DeclareResource(cmd *cobra.Command, resource Resource) error {
	return declarationError(
		internalschema.KindResource,
		resource.URI,
		internalschema.AddResource(cmd, internalschema.Resource(resource)),
	)
}

// maxKeyInMessage bounds how much of an adopter-supplied key an error message
// echoes, so a too_long URI cannot inflate the envelope on stderr.
const maxKeyInMessage = 128

func declarationError(kind internalschema.Kind, key string, violation *internalschema.Violation) error {
	if violation == nil {
		return nil
	}
	// Declaration runs while the tree is built, before any span exists and
	// without I/O, so there is no caller context to carry a trace ID.
	return contract.NewError(
		context.Background(),
		invalidDeclarationCode,
		fmt.Sprintf("invalid %s declaration %q: %s %s", kind, truncateKey(key), violation.Field, violation.Reason),
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithActionableFix(declarationFix(violation.Reason)),
		contract.WithErrorContext(map[string]any{
			"field":  violation.Field,
			"reason": string(violation.Reason),
		}),
	)
}

func declarationFix(reason internalschema.Reason) string {
	switch reason {
	case internalschema.ReasonNilCommand:
		return "Declare on a non-nil *cobra.Command."
	case internalschema.ReasonRequired:
		return "Set the required field."
	case internalschema.ReasonInvalidCharset:
		return "Use only letters, digits, '_', '.', and '-' (^[a-zA-Z0-9_.-]+$)."
	case internalschema.ReasonInvalidUTF8:
		return "Use valid UTF-8 text."
	case internalschema.ReasonDuplicate:
		return "Use a name or URI not already declared on this command."
	case internalschema.ReasonUndeclaredPlaceholder:
		return "Declare every {{placeholder}} in the template as an argument."
	case internalschema.ReasonNotAbsolute:
		return "Give the URI a scheme, such as app://docs/name or urn:app:name."
	case internalschema.ReasonMalformed:
		return "Fix the URI so it parses, including any percent-escapes."
	case internalschema.ReasonTooLong:
		return "Shorten the URI to at most 2048 bytes."
	case internalschema.ReasonInvalidCharacter:
		return "Remove whitespace and control characters (a MIME type may contain spaces)."
	case internalschema.ReasonCorruptAnnotation:
		return corruptAnnotationFix
	default:
		return ""
	}
}

const corruptAnnotationFix = "Declare prompts and resources only with DeclarePrompt and " +
	"DeclareResource; never write the github.com/rshade/ax-go/schema/* annotations by hand."

// treeDeclarationError maps a problem found by walking the tree — a duplicate
// key or a corrupt annotation — to the validation_error envelope __schema
// returns (exit 2).
func treeDeclarationError(ctx context.Context, conflict *internalschema.Conflict) error {
	if conflict.Reason == internalschema.ReasonCorruptAnnotation {
		return contract.NewError(
			ctx,
			"validation_error",
			fmt.Sprintf("corrupt %s annotation on command %q", conflict.Kind, conflict.Commands[0]),
			contract.WithErrorExitCode(contract.ExitValidation),
			contract.WithActionableFix(corruptAnnotationFix),
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
	case internalschema.KindPrompt:
		label = "prompt name"
	case internalschema.KindResource:
		label = "resource URI"
	default:
		label = string(conflict.Kind)
	}
	return contract.NewError(
		ctx,
		"validation_error",
		fmt.Sprintf("duplicate %s %q declared on more than one command", label, truncateKey(conflict.Key)),
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithActionableFix("Rename or remove one of the duplicate declarations."),
		contract.WithErrorContext(map[string]any{
			"kind":     string(conflict.Kind),
			"key":      conflict.Key,
			"commands": conflict.Commands,
		}),
	)
}

func truncateKey(key string) string {
	if len(key) <= maxKeyInMessage {
		return key
	}
	cut := maxKeyInMessage
	for cut > 0 && !utf8.RuneStart(key[cut]) {
		cut--
	}
	return key[:cut] + "…"
}

func toInternalPrompt(prompt Prompt) internalschema.Prompt {
	return internalschema.Prompt{
		Name:        prompt.Name,
		Title:       prompt.Title,
		Description: prompt.Description,
		Arguments: convertSlice(
			prompt.Arguments,
			func(a PromptArgument) internalschema.PromptArgument { return internalschema.PromptArgument(a) },
		),
		Template: prompt.Template,
	}
}

func fromInternalPrompt(prompt internalschema.Prompt) Prompt {
	return Prompt{
		Name:        prompt.Name,
		Title:       prompt.Title,
		Description: prompt.Description,
		Arguments: convertSlice(
			prompt.Arguments,
			func(a internalschema.PromptArgument) PromptArgument { return PromptArgument(a) },
		),
		Template: prompt.Template,
	}
}

func toMCPPrompt(prompt internalschema.Prompt) MCPPrompt {
	return MCPPrompt{
		Name:        prompt.Name,
		Title:       prompt.Title,
		Description: prompt.Description,
		Arguments: convertSlice(
			prompt.Arguments,
			func(a internalschema.PromptArgument) MCPPromptArgument { return MCPPromptArgument(a) },
		),
		Template: prompt.Template,
	}
}

func fromInternalResource(resource internalschema.Resource) Resource { return Resource(resource) }

func toMCPResource(resource internalschema.Resource) MCPResource { return MCPResource(resource) }

// convertSlice maps src element-wise, preserving nil so omitempty fields stay
// absent for a command that declares nothing.
func convertSlice[S, D any](src []S, convert func(S) D) []D {
	if src == nil {
		return nil
	}
	dst := make([]D, 0, len(src))
	for _, item := range src {
		dst = append(dst, convert(item))
	}
	return dst
}
