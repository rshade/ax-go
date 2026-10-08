package mcp

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

// TestBuildAggregatesDeclarationsBeyondCallableTools pins that prompts and
// resources come from every non-hidden command — including non-runnable groups
// and excluded commands that never become tools — in walk then declaration
// order, keeping the first of a duplicated key.
func TestBuildAggregatesDeclarationsBeyondCallableTools(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	group := &cobra.Command{Use: "group"}
	leaf := &cobra.Command{Use: "leaf", RunE: noopRunE}
	excluded := &cobra.Command{Use: "serve", RunE: noopRunE}
	hidden := &cobra.Command{Use: "secret", Hidden: true, RunE: noopRunE}
	MarkExcluded(excluded)
	group.AddCommand(leaf)
	root.AddCommand(group, excluded, hidden)

	declare := func(cmd *cobra.Command, name, uri string) {
		t.Helper()
		if v := internalschema.AddPrompt(cmd, internalschema.Prompt{Name: name, Template: cmd.Name()}); v != nil {
			t.Fatalf("AddPrompt: %+v", v)
		}
		if v := internalschema.AddResource(cmd, internalschema.Resource{URI: uri, Name: name}); v != nil {
			t.Fatalf("AddResource: %+v", v)
		}
	}
	declare(root, "root-prompt", "app://root")
	declare(group, "group-prompt", "app://group")
	declare(leaf, "shared", "app://shared")
	declare(excluded, "shared", "app://shared")
	declare(hidden, "hidden-prompt", "app://hidden")

	schema := Build(root)

	var prompts []string
	for _, prompt := range schema.Prompts {
		prompts = append(prompts, prompt.Name+":"+prompt.Template)
	}
	wantPrompts := []string{"root-prompt:app", "group-prompt:group", "shared:leaf"}
	if !slices.Equal(prompts, wantPrompts) {
		t.Fatalf("prompts = %v, want %v", prompts, wantPrompts)
	}

	var uris []string
	for _, resource := range schema.Resources {
		uris = append(uris, resource.URI)
	}
	wantURIs := []string{"app://root", "app://group", "app://shared"}
	if !slices.Equal(uris, wantURIs) {
		t.Fatalf("resource URIs = %v, want %v", uris, wantURIs)
	}

	var tools []string
	for _, tool := range schema.Tools {
		tools = append(tools, tool.Name)
	}
	if !slices.Equal(tools, []string{"app-group-leaf"}) {
		t.Fatalf("tools = %v, want only the callable leaf", tools)
	}
}

func TestBuildWithoutDeclarationsLeavesCollectionsNil(t *testing.T) {
	root := &cobra.Command{Use: "app", RunE: noopRunE}
	schema := Build(root)
	if schema.Prompts != nil || schema.Resources != nil {
		t.Fatalf("Prompts = %v, Resources = %v, want both nil", schema.Prompts, schema.Resources)
	}
}
