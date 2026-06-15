// Package testutil provides reusable in-process test helpers for the ax-go
// module. It is importable from any _test.go file within the module but is
// excluded from production binaries because it is under internal/ and is only
// imported by test packages.
package testutil

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
	"time"
)

// MaskedSentinel is the placeholder value substituted for each non-deterministic
// field (trace_id, span_id, idempotency_key) before comparison.
const MaskedSentinel = "MASKED"

// OutputMode selects the comparison strategy for a determinism check.
type OutputMode int

const (
	// ModeBoundedJSON compares the full stdout as a single JSON document.
	ModeBoundedJSON OutputMode = iota

	// ModeNDJSON compares stdout line-by-line as a sequence of JSON objects.
	ModeNDJSON
)

// reMaskAll matches documented non-deterministic metadata field key-value pairs
// in a JSON document. The `([^"]*)` group captures only unescaped values, which
// is sufficient for hex trace/span IDs and idempotency keys.
var reMaskAll = regexp.MustCompile(`"(trace_id|span_id|idempotency_key)":"[^"]*"`)

// maskReplacementTemplate is the ReplaceAll template used by MaskNonDeterministic.
// ${1} is substituted with the captured field name on each match, preserving
// the key while replacing only the value.
const maskReplacementTemplate = `"${1}":"` + MaskedSentinel + `"`

// MaskNonDeterministic replaces the values of documented non-deterministic
// metadata fields (trace_id, span_id, idempotency_key) in b with MaskedSentinel
// in a single pass. b is not modified in place; the returned slice is a new
// allocation.
//
// Only JSON string values that immediately follow the field name key are
// replaced. If a field is absent, the output is unchanged for that field.
func MaskNonDeterministic(b []byte) []byte {
	return reMaskAll.ReplaceAll(b, []byte(maskReplacementTemplate))
}

// CompareOutputs asserts that run1 and run2 are byte-identical after masking
// non-deterministic fields. Failures are reported via t.Errorf.
//
//   - For ModeBoundedJSON: the two masked byte slices are compared as a whole.
//     On mismatch, the byte offset of the first divergence and surrounding
//     context are reported.
//   - For ModeNDJSON: each run's output is split on \n (trailing blank line
//     stripped), and lines are compared in order. Line-count mismatch is
//     reported before any content comparison.
//
// Either run producing empty bytes is treated as a precondition violation and
// is reported via t.Errorf; the comparison is skipped.
func CompareOutputs(t testing.TB, run1, run2 []byte, mode OutputMode) {
	t.Helper()
	if len(run1) == 0 || len(run2) == 0 {
		t.Errorf(
			"CompareOutputs: precondition violated — both run1 and run2 must be non-empty"+
				" (len(run1)=%d, len(run2)=%d)",
			len(run1), len(run2),
		)
		return
	}

	m1 := MaskNonDeterministic(run1)
	m2 := MaskNonDeterministic(run2)

	switch mode {
	case ModeBoundedJSON:
		compareBytes(t, m1, m2)
	case ModeNDJSON:
		lines1 := splitNDJSON(m1)
		lines2 := splitNDJSON(m2)
		if len(lines1) != len(lines2) {
			t.Errorf(
				"CompareOutputs (NDJSON): line count mismatch: run1 has %d lines, run2 has %d lines",
				len(lines1), len(lines2),
			)
			return
		}
		for i := range lines1 {
			if !bytes.Equal(lines1[i], lines2[i]) {
				t.Errorf("CompareOutputs (NDJSON): line %d diverges:\n  run1: %q\n  run2: %q", i, lines1[i], lines2[i])
				return
			}
		}
	}
}

// ValidateTimestamps walks the JSON in data and asserts that every string
// value that parses as RFC 3339 also has a UTC timezone (designator "Z" or
// offset "+00:00"). Non-string values and strings that do not parse as RFC 3339
// are skipped silently. Failures are reported via t.Errorf.
//
// If data is not valid JSON, t.Errorf is called with an unmarshal error.
//
// Absence of timestamp fields is not a failure.
func ValidateTimestamps(t testing.TB, data []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Errorf("ValidateTimestamps: invalid JSON: %v", err)
		return
	}
	walkAny(t, v)
}

// AssertFullyTyped asserts that data can be unmarshalled into the type T
// without error. T should be a concrete struct type (e.g., ax.Envelope[helloPayload])
// that represents the expected envelope shape with no map[string]any or
// interface{} fields in the root or meta sections.
//
// Failures are reported via t.Errorf.
func AssertFullyTyped[T any](t testing.TB, data []byte) {
	t.Helper()
	var target T
	if err := json.Unmarshal(data, &target); err != nil {
		t.Errorf("AssertFullyTyped: failed to unmarshal into %T: %v", target, err)
	}
}

// walkAny recursively walks a JSON value decoded into any and validates any
// string values that parse as RFC 3339 timestamps are in UTC.
// This function is unexported; it is used only by ValidateTimestamps.
func walkAny(t testing.TB, v any) {
	switch val := v.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339, val)
		if err == nil {
			_, offset := parsed.Zone()
			if offset != 0 {
				t.Errorf("timestamp %q is not UTC (zone offset = %d seconds)", val, offset)
			}
		}
	case map[string]any:
		for _, child := range val {
			walkAny(t, child)
		}
	case []any:
		for _, elem := range val {
			walkAny(t, elem)
		}
	}
}

// splitNDJSON splits NDJSON bytes on newlines and strips any trailing newline
// before splitting, avoiding a spurious empty element. Returns one []byte
// slice per line.
func splitNDJSON(b []byte) [][]byte {
	b = bytes.TrimRight(b, "\n")
	if len(b) == 0 {
		return nil
	}
	return bytes.Split(b, []byte("\n"))
}

// divergeContextBytes is the number of bytes of context shown around a
// diverging byte in a CompareOutputs (ModeBoundedJSON) failure report.
const divergeContextBytes = 40

// compareBytes is a helper used by CompareOutputs (ModeBoundedJSON) to find
// the index of the first diverging byte between two slices and report it.
func compareBytes(t testing.TB, m1, m2 []byte) {
	if bytes.Equal(m1, m2) {
		return
	}
	idx := 0
	for idx < len(m1) && idx < len(m2) && m1[idx] == m2[idx] {
		idx++
	}
	start := max(idx-divergeContextBytes, 0)
	end1 := min(idx+divergeContextBytes, len(m1))
	end2 := min(idx+divergeContextBytes, len(m2))
	t.Errorf("output diverges at byte %d:\n  run1[%d:%d]: %q\n  run2[%d:%d]: %q",
		idx,
		start, end1, m1[start:end1],
		start, end2, m2[start:end2],
	)
}
