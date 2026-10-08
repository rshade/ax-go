package mcpserver

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rshade/ax-go/schema"
)

func TestResourcesListMatchesSchemaProjection(t *testing.T) {
	root := declaredRoot(t)
	want := schema.BuildMCPSchema(root).Resources
	if len(want) != 2 {
		t.Fatalf("fixture projects %d resources, want 2 (hidden subtree pruned)", len(want))
	}

	forEachTransport(t, root, Config{}, func(t *testing.T, session *sdk.ClientSession) {
		res, err := session.ListResources(context.Background(), nil)
		if err != nil {
			t.Fatalf("resources/list: %v", err)
		}
		got := make([]schema.MCPResource, 0, len(res.Resources))
		for _, r := range res.Resources {
			got = append(got, schema.MCPResource{
				URI: r.URI, Name: r.Name, Title: r.Title, Description: r.Description, MIMEType: r.MIMEType,
			})
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("resources/list = %+v, want __schema --as=mcp resources %+v", got, want)
		}
	})
}

func TestResourcesReadReturnsDeclaredContentByteForByte(t *testing.T) {
	forEachTransport(t, declaredRoot(t), Config{}, func(t *testing.T, session *sdk.ClientSession) {
		for range 3 {
			res, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "demo://docs/skill"})
			if err != nil {
				t.Fatalf("resources/read: %v", err)
			}
			if len(res.Contents) != 1 {
				t.Fatalf("contents = %+v, want exactly one entry", res.Contents)
			}
			c := res.Contents[0]
			if c.URI != "demo://docs/skill" || c.MIMEType != "text/markdown" || c.Text != skillBody() || c.Blob != nil {
				t.Fatalf("contents[0] = {uri %q mime %q len %d blob %v}, want the declared 6.8 KB markdown",
					c.URI, c.MIMEType, len(c.Text), c.Blob)
			}
		}
	})
}

func TestResourcesReadWithoutContentIsEmptyWithDeclaredMIMEType(t *testing.T) {
	forEachTransport(t, declaredRoot(t), Config{}, func(t *testing.T, session *sdk.ClientSession) {
		res, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "demo://docs/empty"})
		if err != nil {
			t.Fatalf("resources/read: %v", err)
		}
		if len(res.Contents) != 1 || res.Contents[0].Text != "" || res.Contents[0].MIMEType != "text/plain" {
			t.Fatalf("contents = %+v, want one empty text/plain entry", res.Contents)
		}
	})
}

func TestResourcesReadRejectsUndeclaredAndHiddenURIs(t *testing.T) {
	forEachTransport(t, declaredRoot(t), Config{}, func(t *testing.T, session *sdk.ClientSession) {
		for _, uri := range []string{"demo://docs/missing", "demo://docs/hidden", "demo://docs/deep"} {
			res, err := session.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: uri})
			if err == nil || res != nil {
				t.Errorf("resources/read %s = (%+v, %v), want a protocol error and no content", uri, res, err)
			}
		}
	})
}

func TestListsStayInDeclarationOrderPastOneSDKPage(t *testing.T) {
	const count = sdk.DefaultPageSize + 50
	root := fixedRoot()
	for i := count - 1; i >= 0; i-- {
		name := fmt.Sprintf("p%05d", i)
		mustAddPromptTo(t, root, name)
		mustAddResourceTo(t, root, "demo://docs/"+name)
	}

	session := newInMemorySession(t, newServerWithConfig(t, context.Background(), root, Config{}))
	prompts, err := session.ListPrompts(context.Background(), nil)
	if err != nil || len(prompts.Prompts) != count || prompts.NextCursor != "" {
		t.Fatalf("prompts/list = %d entries, cursor %q, err %v; want all %d in one page",
			len(prompts.Prompts), prompts.NextCursor, err, count)
	}
	resources, err := session.ListResources(context.Background(), nil)
	if err != nil || len(resources.Resources) != count || resources.NextCursor != "" {
		t.Fatalf("resources/list = %d entries, cursor %q, err %v; want all %d in one page",
			len(resources.Resources), resources.NextCursor, err, count)
	}
	wantPrompts := schema.BuildMCPSchema(root).Prompts
	for i, p := range prompts.Prompts {
		if p.Name != wantPrompts[i].Name {
			t.Fatalf("prompts/list[%d] = %s, want %s (declaration order)", i, p.Name, wantPrompts[i].Name)
		}
	}
	wantResources := schema.BuildMCPSchema(root).Resources
	for i, r := range resources.Resources {
		if r.URI != wantResources[i].URI {
			t.Fatalf("resources/list[%d] = %s, want %s (declaration order)", i, r.URI, wantResources[i].URI)
		}
	}
}
