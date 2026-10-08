package mcpserver

import (
	"context"
	"slices"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// preserveDeclarationOrder makes prompts/list and resources/list return entries
// in declaration-walk order, the order __schema --as=mcp projects them in. The
// SDK pages its listings sorted by name or URI so cursors stay stable; this
// middleware restores the walk order within each page. The served sets are
// small and fixed, far below the SDK page size, so one page holds every entry.
func preserveDeclarationOrder(server *sdk.Server, promptNames, resourceURIs []string) {
	if len(promptNames) == 0 && len(resourceURIs) == 0 {
		return
	}
	promptRank := rankOf(promptNames)
	resourceRank := rankOf(resourceURIs)
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil {
				return res, err
			}
			switch listing := res.(type) {
			case *sdk.ListPromptsResult:
				sortByRank(listing.Prompts, promptRank, func(p *sdk.Prompt) string { return p.Name })
			case *sdk.ListResourcesResult:
				sortByRank(listing.Resources, resourceRank, func(r *sdk.Resource) string { return r.URI })
			}
			return res, nil
		}
	})
}

func rankOf(keys []string) map[string]int {
	rank := make(map[string]int, len(keys))
	for i, key := range keys {
		rank[key] = i
	}
	return rank
}

func sortByRank[T any](items []T, rank map[string]int, key func(T) string) {
	slices.SortStableFunc(items, func(a, b T) int { return rank[key(a)] - rank[key(b)] })
}
