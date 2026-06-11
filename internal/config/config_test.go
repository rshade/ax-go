package config

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

func TestPatchPreservesCommentsAndFormatting(t *testing.T) {
	existing := []byte(`{
	// server settings
	"host": "localhost",
	"port": 8080,
}`)
	patch := []byte(`[{"op":"replace","path":"/port","value":9090}]`)

	got, err := Patch(existing, patch)
	if err != nil {
		t.Fatalf("Patch returned error: %v", err)
	}

	if !strings.Contains(string(got), "// server settings") {
		t.Fatalf("Patch stripped comments; got:\n%s", got)
	}
	if !strings.Contains(string(got), `"host": "localhost"`) {
		t.Fatalf("Patch dropped unchanged field; got:\n%s", got)
	}
	if !strings.Contains(string(got), "9090") {
		t.Fatalf("Patch did not apply replace; got:\n%s", got)
	}
}

func TestPatchInvalidHujsonReturnsHujsonParseError(t *testing.T) {
	_, err := Patch([]byte("{"), []byte(`[]`))
	if err == nil {
		t.Fatal("Patch returned nil error for invalid Hujson")
	}

	var parseErr *HujsonParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error type = %T, want *HujsonParseError", err)
	}
	if parseErr.Unwrap() == nil {
		t.Fatal("HujsonParseError.Unwrap() = nil, want underlying parse error")
	}
}

func TestPatchInvalidPatchDocumentReturnsPatchApplyError(t *testing.T) {
	cases := []struct {
		name  string
		patch string
	}{
		{"not an array", `{"op":"add","path":"/x","value":1}`},
		{"target not found", `[{"op":"remove","path":"/nonexistent"}]`},
		{"missing op field", `[{"path":"/x","value":1}]`},
		{"unknown op", `[{"op":"frobnicate","path":"/x"}]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Patch([]byte(`{"a":1}`), []byte(tc.patch))
			if err == nil {
				t.Fatal("Patch returned nil error")
			}

			var patchErr *PatchApplyError
			if !errors.As(err, &patchErr) {
				t.Fatalf("error type = %T, want *PatchApplyError", err)
			}
			if patchErr.Unwrap() == nil {
				t.Fatal("PatchApplyError.Unwrap() = nil, want underlying error")
			}
		})
	}
}

func TestPatchEmptyPatchIsNoOp(t *testing.T) {
	existing := []byte(`{
	// a comment
	"name": "ax",
}`)
	got, err := Patch(existing, []byte(`[]`))
	if err != nil {
		t.Fatalf("Patch returned error: %v", err)
	}

	if !strings.Contains(string(got), "// a comment") {
		t.Fatalf("Patch stripped comments on no-op; got:\n%s", got)
	}
	if !strings.Contains(string(got), `"name"`) {
		t.Fatalf("Patch dropped field on no-op; got:\n%s", got)
	}
}

func TestPatchRoundTrip(t *testing.T) {
	existing := []byte(`{"name":"ax","replicas":3}`)

	replacePatch := []byte(`[{"op":"replace","path":"/replicas","value":5}]`)
	patched, err := Patch(existing, replacePatch)
	if err != nil {
		t.Fatalf("Patch returned error: %v", err)
	}

	if !strings.Contains(string(patched), "5") {
		t.Fatalf("Patch did not apply replace; got:\n%s", patched)
	}

	revertPatch := []byte(`[{"op":"replace","path":"/replicas","value":3}]`)
	roundTripped, err := Patch(patched, revertPatch)
	if err != nil {
		t.Fatalf("revert Patch returned error: %v", err)
	}

	if !strings.Contains(string(roundTripped), "3") {
		t.Fatalf("round-trip did not restore original value; got:\n%s", roundTripped)
	}
}

func TestReadBoundedHonorsContextCancelation(t *testing.T) {
	t.Run("already canceled aborts before first read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		reader := &readCalledReader{}
		_, err := ReadBounded(ctx, reader, 1024)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReadBounded error = %v, want context.Canceled", err)
		}
		if reader.called {
			t.Fatal("ReadBounded read from source after context was already canceled")
		}
		assertNotTooLargeError(t, err)
	})

	t.Run("deadline expires between chunks", func(t *testing.T) {
		expired := make(chan struct{})
		ctx := controlledDeadlineContext{expired: expired}

		reader := &controlledChunkReader{
			chunks: [][]byte{
				[]byte(`{`),
				[]byte(`}`),
			},
			expireAfterFirstChunk: expired,
		}
		_, err := ReadBounded(ctx, reader, 1024)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ReadBounded error = %v, want context.DeadlineExceeded", err)
		}
		assertNotTooLargeError(t, err)
	})
}

func TestReadBoundedEnforcesLimitAtReadBoundary(t *testing.T) {
	const capBytes int64 = 64

	t.Run("oversized input trips at cap plus one", func(t *testing.T) {
		reader := newTripwireReader(t, strings.NewReader(strings.Repeat(" ", int(capBytes*100))), capBytes+1)

		_, err := ReadBounded(context.Background(), reader, capBytes)
		var tooLarge TooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("ReadBounded error = %v, want TooLargeError", err)
		}
		if reader.read > capBytes+1 {
			t.Fatalf("ReadBounded read %d bytes, want <= %d", reader.read, capBytes+1)
		}
	})

	t.Run("input exactly at cap is accepted", func(t *testing.T) {
		reader := newTripwireReader(t, strings.NewReader(strings.Repeat(" ", int(capBytes))), capBytes+1)

		got, err := ReadBounded(context.Background(), reader, capBytes)
		if err != nil {
			t.Fatalf("ReadBounded returned error: %v", err)
		}
		if int64(len(got)) != capBytes {
			t.Fatalf("ReadBounded length = %d, want %d", len(got), capBytes)
		}
		if reader.read > capBytes+1 {
			t.Fatalf("ReadBounded read %d bytes, want <= %d", reader.read, capBytes+1)
		}
	})

	t.Run("cap above ceiling is invalid before reading", func(t *testing.T) {
		for _, maxBytes := range []int64{MaxConfigBytesCeiling + 1, math.MaxInt64} {
			reader := &readCalledReader{}

			_, err := ReadBounded(context.Background(), reader, maxBytes)
			var invalid InvalidMaxBytesError
			if !errors.As(err, &invalid) {
				t.Fatalf("ReadBounded(%d) error = %v, want InvalidMaxBytesError", maxBytes, err)
			}
			if reader.called {
				t.Fatalf("ReadBounded(%d) read from source before rejecting invalid cap", maxBytes)
			}
		}
	})

	t.Run("cap exactly at ceiling is accepted", func(t *testing.T) {
		got, err := ReadBounded(context.Background(), strings.NewReader("{}"), MaxConfigBytesCeiling)
		if err != nil {
			t.Fatalf("ReadBounded returned error: %v", err)
		}
		if string(got) != "{}" {
			t.Fatalf("ReadBounded data = %q, want {}", got)
		}
	})

	t.Run("zero-progress reader returns io.ErrNoProgress", func(t *testing.T) {
		_, err := ReadBounded(context.Background(), zeroProgressReader{}, 1024)
		if !errors.Is(err, io.ErrNoProgress) {
			t.Fatalf("ReadBounded error = %v, want io.ErrNoProgress", err)
		}
		assertNotTooLargeError(t, err)
	})
}

func TestReadBoundedLargeCapGrowth(t *testing.T) {
	const capBytes int64 = 4 << 20 // above the pre-allocation threshold, exercises append growth

	t.Run("input exactly at large cap is intact", func(t *testing.T) {
		payload := strings.Repeat("a", int(capBytes))

		got, err := ReadBounded(context.Background(), strings.NewReader(payload), capBytes)
		if err != nil {
			t.Fatalf("ReadBounded returned error: %v", err)
		}
		if int64(len(got)) != capBytes {
			t.Fatalf("ReadBounded length = %d, want %d", len(got), capBytes)
		}
		if string(got) != payload {
			t.Fatal("ReadBounded data does not match source across buffer growth")
		}
	})

	t.Run("one byte over large cap is oversize", func(t *testing.T) {
		payload := strings.Repeat("a", int(capBytes)+1)

		_, err := ReadBounded(context.Background(), strings.NewReader(payload), capBytes)
		var tooLarge TooLargeError
		if !errors.As(err, &tooLarge) {
			t.Fatalf("ReadBounded error = %v, want TooLargeError", err)
		}
	})
}

type readCalledReader struct {
	called bool
}

func (r *readCalledReader) Read(_ []byte) (int, error) {
	r.called = true
	return 0, io.EOF
}

type zeroProgressReader struct{}

func (zeroProgressReader) Read([]byte) (int, error) {
	return 0, nil
}

type controlledChunkReader struct {
	chunks                [][]byte
	expireAfterFirstChunk chan struct{}
	index                 int
}

func (r *controlledChunkReader) Read(p []byte) (int, error) {
	if r.index >= len(r.chunks) {
		return 0, io.EOF
	}

	chunk := r.chunks[r.index]
	r.index++
	n := copy(p, chunk)
	if r.index == 1 {
		close(r.expireAfterFirstChunk)
	}
	return n, nil
}

type controlledDeadlineContext struct {
	expired <-chan struct{}
}

func (c controlledDeadlineContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c controlledDeadlineContext) Done() <-chan struct{} {
	return c.expired
}

func (c controlledDeadlineContext) Err() error {
	select {
	case <-c.expired:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func (c controlledDeadlineContext) Value(any) any {
	return nil
}

func assertNotTooLargeError(t *testing.T, err error) {
	t.Helper()

	var tooLarge TooLargeError
	if errors.As(err, &tooLarge) {
		t.Fatalf("ReadBounded error = %v, want non-TooLargeError", err)
	}
}

type tripwireReader struct {
	t      *testing.T
	source io.Reader
	limit  int64
	read   int64
}

func newTripwireReader(t *testing.T, source io.Reader, limit int64) *tripwireReader {
	t.Helper()
	return &tripwireReader{t: t, source: source, limit: limit}
}

func (r *tripwireReader) Read(p []byte) (int, error) {
	r.t.Helper()

	n, err := r.source.Read(p)
	r.read += int64(n)
	if r.read > r.limit {
		r.t.Fatalf("reader returned %d bytes, want <= %d", r.read, r.limit)
	}
	return n, err
}
