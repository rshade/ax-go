package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

// maxPromptArgumentBytes bounds each prompts/get argument value so a client
// cannot make the server build an unbounded string (Constitution IX). It equals
// the template cap, so a value can fill a template-sized slot.
const maxPromptArgumentBytes = 64 << 10

// registerPrompts serves every declared prompt over prompts/list and
// prompts/get. The caller passes the set from the same aggregation
// __schema --as=mcp projects (internalschema.CollectDeclarations), so the two
// cannot diverge, and nothing is registered when none is declared so a CLI
// without prompts keeps the handshake it had before (spec 030 FR-001, FR-002).
func registerPrompts(server *sdk.Server, prompts []internalschema.Prompt) {
	for _, prompt := range prompts {
		arguments := make([]*sdk.PromptArgument, 0, len(prompt.Arguments))
		for _, argument := range prompt.Arguments {
			arguments = append(arguments, &sdk.PromptArgument{
				Name:        argument.Name,
				Title:       argument.Title,
				Description: argument.Description,
				Required:    argument.Required,
			})
		}
		server.AddPrompt(&sdk.Prompt{
			Name:        prompt.Name,
			Title:       prompt.Title,
			Description: prompt.Description,
			Arguments:   arguments,
		}, promptHandler(prompt))
	}
}

// promptHandler validates the call's arguments against the declaration and
// then renders the template as one user-role text message. Validation runs
// before rendering and the rendering is pure, so a rejected call never yields a
// partial render and equal inputs yield identical bytes (Principle II). The
// server renders text only; it never runs it (Principle VI).
func promptHandler(prompt internalschema.Prompt) sdk.PromptHandler {
	declared := make(map[string]bool, len(prompt.Arguments))
	for _, argument := range prompt.Arguments {
		declared[argument.Name] = argument.Required
	}
	return func(_ context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
		values := req.Params.Arguments
		for name, value := range values {
			if _, ok := declared[name]; !ok {
				return nil, invalidParams(
					"unknown argument %q for prompt %q",
					internalschema.TruncateKey(name),
					prompt.Name,
				)
			}
			if len(value) > maxPromptArgumentBytes {
				return nil, invalidParams("argument %q for prompt %q exceeds %d bytes",
					internalschema.TruncateKey(name), prompt.Name, maxPromptArgumentBytes)
			}
		}
		for _, argument := range prompt.Arguments {
			if argument.Required {
				if _, ok := values[argument.Name]; !ok {
					return nil, invalidParams("missing required argument %q for prompt %q", argument.Name, prompt.Name)
				}
			}
		}
		return &sdk.GetPromptResult{
			Description: prompt.Description,
			Messages: []*sdk.PromptMessage{{
				Role:    "user",
				Content: &sdk.TextContent{Text: internalschema.RenderTemplate(prompt.Template, values)},
			}},
		}, nil
	}
}

// invalidParams builds the protocol-level invalid-params error tools/call's
// neighbours use for caller mistakes. Messages name arguments, never values, so
// agent-supplied text is not echoed into error output.
func invalidParams(format string, args ...any) error {
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: fmt.Sprintf(format, args...)}
}
