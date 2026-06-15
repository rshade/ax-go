# Harness API Contract

**Package**: `github.com/rshade/ax-go/internal/testutil`
**File**: `internal/testutil/determinism.go`

---

## Exported Symbols

### Constants

```go
// MaskedSentinel is the placeholder value substituted for each non-deterministic
// field (trace_id, span_id, idempotency_key) before comparison.
const MaskedSentinel = "MASKED"
```

---

### Types

```go
// OutputMode selects the comparison strategy for a determinism check.
type OutputMode int

const (
    // ModeBoundedJSON compares the full stdout as a single JSON document.
    ModeBoundedJSON OutputMode = iota

    // ModeNDJSON compares stdout line-by-line as a sequence of JSON objects.
    ModeNDJSON
)
```

---

### Functions

```go
// MaskNonDeterministic replaces the values of the three documented
// non-deterministic fields (trace_id, span_id, idempotency_key) in b with
// MaskedSentinel. b is not modified in place; the returned slice is a new
// allocation.
//
// Only JSON string values that immediately follow the field name key are
// replaced. If a field is absent, the output is unchanged for that field.
func MaskNonDeterministic(b []byte) []byte
```

```go
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
func CompareOutputs(t testing.TB, run1, run2 []byte, mode OutputMode)
```

```go
// ValidateTimestamps walks the JSON in data and asserts that every string
// value that parses as RFC 3339 also has a UTC timezone (designator "Z" or
// offset "+00:00"). Non-string values and strings that do not parse as RFC 3339
// are skipped silently. Failures are reported via t.Errorf.
//
// If data is not valid JSON, t.Errorf is called with an unmarshal error.
//
// Absence of timestamp fields is not a failure.
func ValidateTimestamps(t testing.TB, data []byte)
```

```go
// AssertFullyTyped asserts that data can be unmarshalled into the type T
// without error. T should be a concrete struct type (e.g., ax.Envelope[helloPayload])
// that represents the expected envelope shape with no map[string]any or
// interface{} fields in the root or meta sections.
//
// Failures are reported via t.Errorf.
func AssertFullyTyped[T any](t testing.TB, data []byte)
```

---

## Usage Contract

### Calling `CompareOutputs`

The caller is responsible for:
1. Executing the command twice with **identical** arguments and a **pinned idempotency key**.
2. Capturing both `stdout` buffers.
3. Verifying both commands exited with `ExitSuccess` before calling `CompareOutputs`.

`CompareOutputs` does NOT re-execute commands. It only compares what it is given.

### Pinning the idempotency key

Always pass `--idempotency-key=<fixed>` when testing determinism. Without it, the key is
auto-generated (UUID v4) and will differ across runs, making the masked comparison fail unless
`idempotency_key` masking covers it — which it does. However, pinning the key is still preferred
because it eliminates a masking dependency and makes the test intent clear.

### Validating NDJSON line counts

For `ModeNDJSON`, `CompareOutputs` reports a line-count mismatch before any per-line comparison.
The caller does not need to check line counts independently.

### Timestamp and type checks

`ValidateTimestamps` and `AssertFullyTyped` are independent helpers. They operate on a single
run's output (not a pair) and can be called after the determinism comparison or independently.

---

## Non-Goals

- The harness does NOT execute commands itself (no subprocess, no `os/exec`).
- The harness does NOT modify `stderr` or exit codes (FR-010).
- The harness does NOT enforce that the command produces output; it only compares what it receives.
  The caller must verify non-empty output (FR-009) before calling `CompareOutputs`.
