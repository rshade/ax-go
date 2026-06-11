package ax

import (
	"context"
	"strings"
	"testing"
)

// FuzzPatchConfig verifies that PatchConfig never panics for arbitrary input.
//
// It seeds the fuzzer with valid, comment-bearing, and structurally varied
// Hujson inputs so the engine explores past the outermost parser layer into
// the patch-application paths. All return paths (success, parse error, and
// patch error) are valid outcomes — only panics are failures.
func FuzzPatchConfig(f *testing.F) {
	seeds := []struct {
		existing string
		patch    string
	}{
		{
			existing: `{"name":"ax","replicas":3}`,
			patch:    `[{"op":"replace","path":"/replicas","value":5}]`,
		},
		{
			existing: `{
	// comment preserved
	"host": "localhost",
	"port": 8080,
}`,
			patch: `[{"op":"replace","path":"/port","value":9090}]`,
		},
		{
			existing: `{"a":{"b":{"c":1}}}`,
			patch:    `[{"op":"add","path":"/a/b/d","value":"new"}]`,
		},
		{
			existing: `{"items":[1,2,3]}`,
			patch:    `[{"op":"remove","path":"/items/1"}]`,
		},
		{
			existing: `{}`,
			patch:    `[]`,
		},
		{
			existing: `{"x":1}`,
			patch:    `[{"op":"test","path":"/x","value":1},{"op":"replace","path":"/x","value":2}]`,
		},
		// Invalid Hujson — parse errors are valid outcomes, not panics.
		{existing: `{`, patch: `[]`},
		{existing: ``, patch: `[]`},
		// Invalid patch documents.
		{existing: `{"a":1}`, patch: `not json`},
		{existing: `{"a":1}`, patch: `{}`},
	}

	for _, s := range seeds {
		f.Add(s.existing, s.patch)
	}

	f.Fuzz(func(t *testing.T, existing, patch string) {
		// Must not panic regardless of input; error outcome is irrelevant.
		_, _ = PatchConfig(context.Background(), strings.NewReader(existing), []byte(patch))
	})
}
