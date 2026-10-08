package mcpserver

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rshade/ax-go/schema"
)

const decideRendered = "Decide: Should we ship?\nContext: \nLiteral {x} and {{ spaced }} stay."

func TestPromptsListMatchesSchemaProjection(t *testing.T) {
	root := declaredRoot(t)
	want := schema.BuildMCPSchema(root).Prompts
	if len(want) != 2 {
		t.Fatalf("fixture projects %d prompts, want 2 (hidden subtree pruned)", len(want))
	}

	forEachTransport(t, root, Config{}, func(t *testing.T, session *sdk.ClientSession) {
		res, err := session.ListPrompts(context.Background(), nil)
		if err != nil {
			t.Fatalf("prompts/list: %v", err)
		}
		got := make([]schema.MCPPrompt, 0, len(res.Prompts))
		for _, p := range res.Prompts {
			prompt := schema.MCPPrompt{Name: p.Name, Title: p.Title, Description: p.Description}
			for _, a := range p.Arguments {
				prompt.Arguments = append(prompt.Arguments, schema.MCPPromptArgument{
					Name: a.Name, Title: a.Title, Description: a.Description, Required: a.Required,
				})
			}
			got = append(got, prompt)
		}
		for i := range want {
			want[i].Template = ""
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("prompts/list = %+v, want __schema --as=mcp prompts (template aside) %+v", got, want)
		}
	})
}

func TestPromptsGetRendersTemplate(t *testing.T) {
	forEachTransport(t, declaredRoot(t), Config{}, func(t *testing.T, session *sdk.ClientSession) {
		cases := []struct {
			name string
			args map[string]string
			want string
		}{
			{
				name: "absent optional renders empty",
				args: map[string]string{"question": "Should we ship?"},
				want: decideRendered,
			},
			{
				name: "all arguments",
				args: map[string]string{"question": "Q", "context": "C"},
				want: "Decide: Q\nContext: C\nLiteral {x} and {{ spaced }} stay.",
			},
			{
				name: "values are never re-expanded",
				args: map[string]string{"question": "{{context}}", "context": "C"},
				want: "Decide: {{context}}\nContext: C\nLiteral {x} and {{ spaced }} stay.",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				res, err := session.GetPrompt(
					context.Background(),
					&sdk.GetPromptParams{Name: "decide", Arguments: tc.args},
				)
				if err != nil {
					t.Fatalf("prompts/get: %v", err)
				}
				if len(res.Messages) != 1 || res.Messages[0].Role != "user" {
					t.Fatalf("messages = %+v, want exactly one user message", res.Messages)
				}
				text, ok := res.Messages[0].Content.(*sdk.TextContent)
				if !ok || text.Text != tc.want {
					t.Fatalf("content = %+v, want text %q", res.Messages[0].Content, tc.want)
				}
			})
		}
	})
}

func TestPromptsGetRejectsBadCallsWithoutPartialRender(t *testing.T) {
	forEachTransport(t, declaredRoot(t), Config{}, func(t *testing.T, session *sdk.ClientSession) {
		cases := []struct {
			name   string
			prompt string
			args   map[string]string
		}{
			{name: "missing required", prompt: "decide", args: map[string]string{"context": "C"}},
			{name: "no arguments at all", prompt: "decide"},
			{name: "undeclared argument", prompt: "decide", args: map[string]string{"question": "Q", "bogus": "x"}},
			{name: "argument on a prompt with none", prompt: "aaa-plain", args: map[string]string{"x": "y"}},
			{name: "unknown prompt", prompt: "nope"},
			{name: "hidden subtree prompt", prompt: "hidden-prompt"},
			{
				name:   "oversized value",
				prompt: "decide",
				args:   map[string]string{"question": strings.Repeat("a", maxPromptArgumentBytes+1)},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				res, err := session.GetPrompt(
					context.Background(),
					&sdk.GetPromptParams{Name: tc.prompt, Arguments: tc.args},
				)
				if res != nil {
					t.Fatalf("prompts/get returned a result %+v, want none", res)
				}
				var rpcErr *jsonrpc.Error
				if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeInvalidParams {
					t.Fatalf("prompts/get error = %v, want an invalid-params protocol error", err)
				}
				if strings.Contains(err.Error(), "aaaaaaaa") {
					t.Fatalf("error echoes the argument value: %.200s", err)
				}
			})
		}
	})
}

func TestPromptsGetIsDeterministic(t *testing.T) {
	forEachTransport(t, declaredRoot(t), Config{}, func(t *testing.T, session *sdk.ClientSession) {
		var first string
		for i := range 10 {
			res, err := session.GetPrompt(context.Background(), &sdk.GetPromptParams{
				Name: "decide", Arguments: map[string]string{"question": "Should we ship?"},
			})
			if err != nil {
				t.Fatalf("prompts/get: %v", err)
			}
			text := res.Messages[0].Content.(*sdk.TextContent).Text
			if i == 0 {
				first = text
			} else if text != first {
				t.Fatalf("call %d rendered %q, want %q", i, text, first)
			}
		}
	})
}
