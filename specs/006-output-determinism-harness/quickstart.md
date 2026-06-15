# Quickstart: Using the Output-Determinism Harness

**Feature branch**: `006-output-determinism-harness`

---

## Prerequisites

- The harness helper lives at `internal/testutil/determinism.go` (package `testutil`).
- Import it from any `_test.go` file within the module:

```go
import "github.com/rshade/ax-go/internal/testutil"
```

---

## Scenario 1 — Check a bounded JSON command

```go
func TestMyCommandIsDeterministic(t *testing.T) {
    invoke := func(stdout, stderr io.Writer) int {
        return run(
            context.Background(),
            []string{"my-command", "--format=json", "--idempotency-key=test-key"},
            strings.NewReader(""),
            stdout,
            stderr,
            func(string) string { return "" },
        )
    }

    var out1, err1 bytes.Buffer
    if code := invoke(&out1, &err1); code != ax.ExitSuccess {
        t.Fatalf("run 1: exit %d; stderr=%s", code, err1.String())
    }
    if out1.Len() == 0 {
        t.Fatal("run 1: stdout is empty")
    }

    var out2, err2 bytes.Buffer
    if code := invoke(&out2, &err2); code != ax.ExitSuccess {
        t.Fatalf("run 2: exit %d; stderr=%s", code, err2.String())
    }
    if out2.Len() == 0 {
        t.Fatal("run 2: stdout is empty")
    }

    testutil.CompareOutputs(t, out1.Bytes(), out2.Bytes(), testutil.ModeBoundedJSON)
}
```

---

## Scenario 2 — Check an NDJSON streaming command

```go
func TestMyStreamCommandIsDeterministic(t *testing.T) {
    invoke := func(stdout, stderr io.Writer) int {
        return run(
            context.Background(),
            []string{"stream", "--format=json", "--idempotency-key=test-key", "--count=3"},
            strings.NewReader(""),
            stdout,
            stderr,
            func(string) string { return "" },
        )
    }

    var out1, err1 bytes.Buffer
    if code := invoke(&out1, &err1); code != ax.ExitSuccess {
        t.Fatalf("run 1: exit %d; stderr=%s", code, err1.String())
    }

    var out2, err2 bytes.Buffer
    if code := invoke(&out2, &err2); code != ax.ExitSuccess {
        t.Fatalf("run 2: exit %d; stderr=%s", code, err2.String())
    }

    testutil.CompareOutputs(t, out1.Bytes(), out2.Bytes(), testutil.ModeNDJSON)
}
```

---

## Scenario 3 — Add timestamp validation

```go
func TestMyCommandTimestamps(t *testing.T) {
    var stdout bytes.Buffer
    var stderr bytes.Buffer

    if code := run(
        context.Background(),
        []string{"my-command", "--format=json", "--idempotency-key=test-key"},
        strings.NewReader(""),
        &stdout,
        &stderr,
        func(string) string { return "" },
    ); code != ax.ExitSuccess {
        t.Fatalf("exit %d; stderr=%s", code, stderr.String())
    }

    testutil.ValidateTimestamps(t, stdout.Bytes())
}
```

---

## Scenario 4 — Assert a fully-typed envelope

```go
func TestMyCommandEnvelopeIsFullyTyped(t *testing.T) {
    var stdout bytes.Buffer
    var stderr bytes.Buffer

    if code := run(
        context.Background(),
        []string{"my-command", "--format=json", "--idempotency-key=test-key"},
        strings.NewReader(""),
        &stdout,
        &stderr,
        func(string) string { return "" },
    ); code != ax.ExitSuccess {
        t.Fatalf("exit %d; stderr=%s", code, stderr.String())
    }

    testutil.AssertFullyTyped[ax.Envelope[myPayload]](t, stdout.Bytes())
}
```

---

## Masking only (low-level)

If you need the masked bytes for a custom assertion:

```go
masked := testutil.MaskNonDeterministic(stdout.Bytes())
```

---

## What gets masked

| JSON field | Location | Example original | After masking |
|-----------|----------|-----------------|---------------|
| `trace_id` | `meta` object | `"trace_id":"4bf92f3577b34da6..."` | `"trace_id":"MASKED"` |
| `span_id` | `meta` object | `"span_id":"00f067aa0ba902b7"` | `"span_id":"MASKED"` |
| `idempotency_key` | `meta` object | `"idempotency_key":"test-key"` | `"idempotency_key":"MASKED"` |

Fields absent from the output are ignored.

---

## Run the tests

```bash
go test -race ./examples/integration/... ./internal/testutil/...
```

To run only the determinism tests:

```bash
go test -race -run TestDeterminism ./examples/integration/...
```
