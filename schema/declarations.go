package schema

import (
	"context"
	"fmt"

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
// The live mcp-server serves the prompt: prompts/get replaces each declared
// "{{name}}" with the argument's value, and an absent optional argument
// renders as empty text. A template is at most 64 KiB; keep it short, because
// it is part of the __schema contract, and put long reference text in a
// resource's content instead.
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
// DeclareResource. Content is fixed text captured at declaration time and
// served by the live mcp-server's resources/read; no field can hold a callback
// or live state, so resources never expose run records (Constitution
// Principle VI).
//
// URI is required, at most 2048 bytes, free of whitespace and control
// characters, parseable, and must carry a scheme ("app://docs/x", "urn:app:x").
// Name is required. MIMEType may contain spaces ("text/plain; charset=utf-8")
// but no control characters. All text must be valid UTF-8. Content is at most
// 1 MiB; a resource declared without content is still listed, and
// resources/read returns an empty body with the declared MIME type. Content is
// never projected into __schema or __schema --as=mcp, which keeps long
// reference text out of the discovery payload: keep prompt templates short and
// put long reference text in resource content. Declarations on a hidden
// command, or in a hidden subtree, are not projected.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
	Content     string `json:"-"`
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

func declarationError(kind internalschema.Kind, key string, violation *internalschema.Violation) error {
	if violation == nil {
		return nil
	}
	// Declaration runs while the tree is built, before any span exists and
	// without I/O, so there is no caller context to carry a trace ID.
	return contract.NewError(
		context.Background(),
		invalidDeclarationCode,
		fmt.Sprintf(
			"invalid %s declaration %q: %s %s",
			kind,
			internalschema.TruncateKey(key),
			violation.Field,
			violation.Reason,
		),
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithActionableFix(violationFix(violation)),
		contract.WithErrorContext(map[string]any{
			"field":  violation.Field,
			"reason": string(violation.Reason),
		}),
	)
}

// violationFix is declarationFix with the one field-specific override: a
// duplicate among enum values is about canonical equality, not a name or URI.
func violationFix(violation *internalschema.Violation) string {
	if violation.Reason == internalschema.ReasonDuplicate && violation.Field == "values" {
		return `Remove enum values that are equal after canonicalisation, such as "3" and "03".`
	}
	return declarationFix(violation.Reason, violation.Field)
}

func declarationFix(reason internalschema.Reason, field string) string {
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
		return tooLongFix(field)
	case internalschema.ReasonInvalidCharacter:
		return "Remove whitespace and control characters (a MIME type may contain spaces)."
	case internalschema.ReasonCorruptAnnotation:
		return internalschema.CorruptAnnotationFix
	case internalschema.ReasonFlagNotFound:
		return "Define the flag on the command (Flags or PersistentFlags) before declaring it."
	case internalschema.ReasonUnsupportedType:
		return "Declare an enum only on a string, integer, or custom non-slice flag."
	case internalschema.ReasonInvalidValue:
		return "Use a value that parses as the flag's type, written as it would appear on argv."
	case internalschema.ReasonNotInEnum:
		return "Use a value from the flag's enum, and keep a non-empty default inside the enum."
	case internalschema.ReasonNotVocabulary:
		return "Use one of the Capability constants: read-only, create, mutate, delete, external-network, admin."
	default:
		return ""
	}
}

func tooLongFix(field string) string {
	switch field {
	case "content":
		return "Shorten the resource content to at most 1 MiB."
	case "template":
		return "Shorten the template to at most 64 KiB; move long reference text into resource content."
	default:
		return "Shorten the URI to at most 2048 bytes."
	}
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

func fromInternalResource(resource internalschema.Resource) Resource {
	resource.Content = ""
	return Resource(resource)
}

func toMCPResource(resource internalschema.Resource) MCPResource {
	return MCPResource{
		URI:         resource.URI,
		Name:        resource.Name,
		Title:       resource.Title,
		Description: resource.Description,
		MIMEType:    resource.MIMEType,
	}
}

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
