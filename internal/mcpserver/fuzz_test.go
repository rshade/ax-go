package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
