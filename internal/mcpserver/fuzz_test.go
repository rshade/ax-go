package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

// FuzzDecodeArguments exercises the tool-argument decoder (a parser surface):
// it must never panic and must reject non-object payloads without crashing
// (Principle VII).
func FuzzDecodeArguments(f *testing.F) {
	for _, seed := range []string{
		``,
		`{}`,
		`null`,
		`{"name":"x","count":3,"flag":true}`,
		`{"nested":{"a":1}}`,
		`[1,2,3]`,
		`"a string"`,
		`{`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		args, err := decodeArguments(json.RawMessage(raw))
		if err == nil && args == nil {
			t.Fatal("decodeArguments returned nil map with nil error")
		}
	})
}

// FuzzExtractTraceContext exercises W3C trace-context extraction from request
// metadata (a parser surface): a malformed traceparent must degrade gracefully,
// never panic (Principle VII, D7).
func FuzzExtractTraceContext(f *testing.F) {
	for _, seed := range []string{
		"",
		"00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		"garbage",
		"00-invalid",
		"ff-ffffffffffffffffffffffffffffffff-ffffffffffffffff-ff",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, traceparent string) {
		req := &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{
			Meta: sdk.Meta{traceParentKey: traceparent},
		}}
		_ = extractTraceContext(context.Background(), req)
	})
}

// FuzzPromptsGet drives the prompts/get handler with arbitrary argument text
// (a parser surface, Principle VII): it must never panic, a rejected call must
// carry no result, an accepted call must render deterministically, and a value
// must never be re-expanded as a placeholder.
func FuzzPromptsGet(f *testing.F) {
	for _, seed := range [][3]string{
		{"Should we ship?", "ctx", "bogus"},
		{"{{context}}", "{{question}}", ""},
		{"", "", "x"},
		{"{{{{a}}", "}}{{", "question"},
		{"é\n日本", "\x00", "context"},
	} {
		f.Add(seed[0], seed[1], seed[2])
	}

	prompt := internalschema.Prompt{
		Name: "decide",
		Arguments: []internalschema.PromptArgument{
			{Name: "question", Required: true},
			{Name: "context"},
		},
		Template: "Q: {{question}}\nC: {{context}}\n{{ literal }}",
	}
	handler := promptHandler(prompt)
	f.Fuzz(func(t *testing.T, question, extra, unknown string) {
		args := map[string]string{"question": question, "context": extra}
		if unknown != "" && unknown != "question" && unknown != "context" {
			args[unknown] = "x"
		}
		req := &sdk.GetPromptRequest{Params: &sdk.GetPromptParams{Name: prompt.Name, Arguments: args}}

		first, err := handler(context.Background(), req)
		if err != nil {
			if first != nil {
				t.Fatal("rejected call returned a result")
			}
			return
		}
		if _, undeclared := args[unknown]; undeclared && unknown != "" && unknown != "question" &&
			unknown != "context" {
			t.Fatalf("undeclared argument %q was accepted", unknown)
		}
		second, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("second identical call failed: %v", err)
		}
		got := first.Messages[0].Content.(*sdk.TextContent).Text
		if again := second.Messages[0].Content.(*sdk.TextContent).Text; again != got {
			t.Fatalf("non-deterministic render: %q vs %q", got, again)
		}
		if want := internalschema.RenderTemplate(prompt.Template, args); got != want {
			t.Fatalf("handler rendered %q, RenderTemplate says %q", got, want)
		}
	})
}
