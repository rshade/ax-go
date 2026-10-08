package mcpserver

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

// registerResources serves every declared resource over resources/list and
// resources/read. The caller passes the set from the same aggregation
// __schema --as=mcp projects (internalschema.CollectDeclarations), so the two
// cannot diverge, and nothing is registered when none is declared so a CLI without resources keeps
// the handshake it had before (spec 030 FR-008, SC-002).
func registerResources(server *sdk.Server, resources []internalschema.Resource) {
	for _, resource := range resources {
		server.AddResource(&sdk.Resource{
			URI:         resource.URI,
			Name:        resource.Name,
			Title:       resource.Title,
			Description: resource.Description,
			MIMEType:    resource.MIMEType,
		}, staticResourceHandler(resource))
	}
}

// staticResourceHandler returns the declared content verbatim. The content was
// captured at declaration time, so every read of a URI returns identical bytes
// and no live state can reach a client (Constitution VI).
func staticResourceHandler(resource internalschema.Resource) sdk.ResourceHandler {
	return func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
		return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
			URI:      resource.URI,
			MIMEType: resource.MIMEType,
			Text:     resource.Content,
		}}}, nil
	}
}
