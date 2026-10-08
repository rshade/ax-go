package mcpserver

import (
	"context"
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
