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
// Name and every argument name must match ^[a-zA-Z0-9_.-]+$. Template is
// required; each "{{name}}" in it must name a declared argument, and any other
// brace text (including "{{ name }}" with spaces) is literal. Phase 1 is a
// declaration only: the live mcp-server does not serve prompts yet.
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
// URI is required, at most 2048 bytes, absolute (non-empty scheme), and free
// of whitespace and control characters. Name is required.
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
// It returns nil on success. On a nil cmd, an invalid prompt, or a prompt name
// already declared on cmd, it returns a *contract.Error with error_code
// invalid_schema_declaration (exit 2) whose context names the offending field
// and a stable reason, and leaves cmd unchanged. The same name declared on two
// different commands is caught when the tree is walked: __schema then fails
// with validation_error (exit 2).
func DeclarePrompt(cmd *cobra.Command, prompt Prompt) error {
	return declarationError("prompt", prompt.Name, internalschema.AddPrompt(cmd, toInternalPrompt(prompt)))
}

// DeclareResource validates resource and records a copy of it on cmd, with the
// same ordering, copy, and error contract as DeclarePrompt; resources are keyed
// by URI.
func DeclareResource(cmd *cobra.Command, resource Resource) error {
	return declarationError(
		"resource",
		resource.URI,
		internalschema.AddResource(cmd, internalschema.Resource(resource)),
	)
}

func declarationError(kind, key string, violation *internalschema.Violation) error {
	if violation == nil {
		return nil
	}
	// Declaration runs while the tree is built, before any span exists and
	// without I/O, so there is no caller context to carry a trace ID.
	return contract.NewError(
		context.Background(),
		invalidDeclarationCode,
		fmt.Sprintf("invalid %s declaration %q: %s %s", kind, key, violation.Field, violation.Reason),
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithErrorContext(map[string]any{
			"field":  violation.Field,
			"reason": violation.Reason,
		}),
	)
}

func duplicateDeclarationError(ctx context.Context, conflict *internalschema.Conflict) error {
	label := "prompt name"
	if conflict.Kind == internalschema.KindResource {
		label = "resource URI"
	}
	return contract.NewError(
		ctx,
		"validation_error",
		fmt.Sprintf("duplicate %s %q declared on more than one command", label, conflict.Key),
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithActionableFix("Rename or remove one of the duplicate declarations."),
		contract.WithErrorContext(map[string]any{
			"kind":     conflict.Kind,
			"key":      conflict.Key,
			"commands": conflict.Commands,
		}),
	)
}

func toInternalPrompt(prompt Prompt) internalschema.Prompt {
	args := make([]internalschema.PromptArgument, 0, len(prompt.Arguments))
	for _, arg := range prompt.Arguments {
		args = append(args, internalschema.PromptArgument(arg))
	}
	return internalschema.Prompt{
		Name:        prompt.Name,
		Title:       prompt.Title,
		Description: prompt.Description,
		Arguments:   args,
		Template:    prompt.Template,
	}
}

func fromInternalPrompts(source []internalschema.Prompt) []Prompt {
	if source == nil {
		return nil
	}
	prompts := make([]Prompt, 0, len(source))
	for _, prompt := range source {
		var args []PromptArgument
		for _, arg := range prompt.Arguments {
			args = append(args, PromptArgument(arg))
		}
		prompts = append(prompts, Prompt{
			Name:        prompt.Name,
			Title:       prompt.Title,
			Description: prompt.Description,
			Arguments:   args,
			Template:    prompt.Template,
		})
	}
	return prompts
}

func fromInternalResources(source []internalschema.Resource) []Resource {
	if source == nil {
		return nil
	}
	resources := make([]Resource, 0, len(source))
	for _, resource := range source {
		resources = append(resources, Resource(resource))
	}
	return resources
}

func toMCPPrompts(source []internalschema.Prompt) []MCPPrompt {
	if source == nil {
		return nil
	}
	prompts := make([]MCPPrompt, 0, len(source))
	for _, prompt := range source {
		var args []MCPPromptArgument
		for _, arg := range prompt.Arguments {
			args = append(args, MCPPromptArgument(arg))
		}
		prompts = append(prompts, MCPPrompt{
			Name:        prompt.Name,
			Title:       prompt.Title,
			Description: prompt.Description,
			Arguments:   args,
			Template:    prompt.Template,
		})
	}
	return prompts
}

func toMCPResources(source []internalschema.Resource) []MCPResource {
	if source == nil {
		return nil
	}
	resources := make([]MCPResource, 0, len(source))
	for _, resource := range source {
		resources = append(resources, MCPResource(resource))
	}
	return resources
}
