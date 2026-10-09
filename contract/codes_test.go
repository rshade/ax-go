package contract

import (
	"slices"
	"testing"
)

func TestKnownErrorCodesProperties(t *testing.T) {
	codes := KnownErrorCodes()

	if !slices.IsSorted(codes) {
		t.Errorf("KnownErrorCodes() = %q, want byte-wise sorted", codes)
	}
	if compacted := slices.Compact(slices.Clone(codes)); len(compacted) != len(codes) {
		t.Errorf("KnownErrorCodes() = %q, want no duplicates", codes)
	}
	if slices.Contains(codes, "invalid_schema_declaration") {
		t.Error("KnownErrorCodes() lists invalid_schema_declaration, an authoring-time code")
	}

	want := slices.Clone(codes)
	codes[0] = "mutated"
	if got := KnownErrorCodes(); !slices.Equal(got, want) {
		t.Errorf("KnownErrorCodes() after caller mutation = %q, want %q", got, want)
	}
}
