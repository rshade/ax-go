package mcpserver

import (
	"context"
	"io"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

// skillBody returns a skill-sized document (about 6.8 KB), the size class spec 030
// SC-006 requires the live server to carry byte for byte.
func skillBody() string { return strings.Repeat("0123456789abcdef\n", 400) }

// declaredRoot extends fixedRoot with prompts and resources on the root, a
// leaf, and a group, plus declarations inside a hidden subtree that must never
// be served.
func declaredRoot(t *testing.T) *cobra.Command {
	t.Helper()
	root := fixedRoot()
	find := func(use string) *cobra.Command {
		for _, cmd := range root.Commands() {
			if cmd.Name() == use {
				return cmd
			}
		}
		t.Fatalf("fixture command %q not found", use)
		return nil
	}

	addResource := func(cmd *cobra.Command, resource internalschema.Resource) {
		t.Helper()
		if v := internalschema.AddResource(cmd, resource); v != nil {
			t.Fatalf("AddResource %s: %+v", resource.URI, v)
		}
	}
	addResource(root, internalschema.Resource{
		URI: "demo://docs/skill", Name: "skill", Title: "Skill", Description: "the protocol",
		MIMEType: "text/markdown", Content: skillBody(),
	})
	addResource(find("group"), internalschema.Resource{
		URI: "demo://docs/empty", Name: "empty", MIMEType: "text/plain",
	})
	addResource(find("secret"), internalschema.Resource{URI: "demo://docs/hidden", Name: "hidden", Content: "x"})
	addResource(
		find("admin").Commands()[0],
		internalschema.Resource{URI: "demo://docs/deep", Name: "deep", Content: "x"},
	)

	addPrompt := func(cmd *cobra.Command, prompt internalschema.Prompt) {
		t.Helper()
		if v := internalschema.AddPrompt(cmd, prompt); v != nil {
			t.Fatalf("AddPrompt %s: %+v", prompt.Name, v)
		}
	}
	addPrompt(root, internalschema.Prompt{
		Name: "decide", Title: "Decide", Description: "run the protocol",
		Arguments: []internalschema.PromptArgument{
			{Name: "question", Title: "Question", Description: "what to decide", Required: true},
			{Name: "context", Description: "extra context"},
		},
		Template: "Decide: {{question}}\nContext: {{context}}\nLiteral {x} and {{ spaced }} stay.",
	})
	addPrompt(find("greet"), internalschema.Prompt{Name: "aaa-plain", Template: "no arguments here"})
	addPrompt(find("secret"), internalschema.Prompt{Name: "hidden-prompt", Template: "x"})
	return root
}

// forEachTransport runs fn against a stdio-path (in-memory) session and an HTTP
// session over one server configuration, so every served-surface assertion
// covers both transports (FR-018).
func forEachTransport(
	t *testing.T,
	root *cobra.Command,
	cfg Config,
	fn func(t *testing.T, session *sdk.ClientSession),
) {
	t.Helper()
	// Each subtest delegates its assertions to fn, which every caller supplies;
	// the type-free slop rule cannot see through the callback.
	// ast-grep-ignore: subtest-asserts-nothing
	t.Run("stdio", func(t *testing.T) {
		fn(t, newInMemorySession(t, newServerWithConfig(t, context.Background(), root, cfg)))
	})
	// ast-grep-ignore: subtest-asserts-nothing
	t.Run("http", func(t *testing.T) {
		fn(t, connectHTTP(t, serveHTTPForServer(t, newServerWithConfig(t, context.Background(), root, cfg))))
	})
}

func newServerWithConfig(t *testing.T, ctx context.Context, root *cobra.Command, cfg Config) *sdk.Server {
	t.Helper()
	if cfg.Version == "" {
		cfg.Version = testServerVersion
	}
	cfg.ServerName = root.Name()
	cfg.Stderr = io.Discard
	return newMCPServer(newDispatcher(ctx, root, cfg), cfg)
}
