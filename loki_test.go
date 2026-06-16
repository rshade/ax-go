package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLokiNoop_NoEnvVar validates that WithLokiFromEnv is a no-op when
// AX_LOKI_URL is unset, leaving zerologLogger.sinks empty. FR-002 / no-op path.
func TestLokiNoop_NoEnvVar(t *testing.T) {
	t.Setenv("AX_LOKI_URL", "")
	var buf bytes.Buffer
	l := NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)
	zl, ok := l.(zerologLogger)
	if !ok {
		t.Fatal("NewLogger did not return zerologLogger")
	}
	if len(zl.sinks) != 0 {
		t.Fatalf("expected 0 sinks when AX_LOKI_URL is unset, got %d", len(zl.sinks))
	}
}

// TestLokiImportIsolation confirms that github.com/rshade/ax-go and its transitive
// dependencies contain no import path with the substring "loki", enforcing the
// import-isolation constraint from the design (D1/D10 in research.md).
func TestLokiImportIsolation(t *testing.T) {
	cmd := exec.Command("go", "list", "-json", "-deps", "github.com/rshade/ax-go")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list failed: %v", err)
	}
	// go list -json -deps emits multiple concatenated JSON objects; parse each.
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var pkg struct {
			ImportPath string `json:"ImportPath"`
		}
		if err := dec.Decode(&pkg); err != nil {
			t.Fatalf("decode package: %v", err)
		}
		if strings.Contains(pkg.ImportPath, "loki") {
			t.Errorf("found loki import: %s", pkg.ImportPath)
		}
	}
}

// TestLokiWriter_PushesOnFlush validates core push functionality: with AX_LOKI_URL
// set, a log line written to the logger must result in a POST to /loki/api/v1/push
// after ax.Flush. The POST body must be valid JSON with a "streams" key
// (US1 / FR-003).
func TestLokiWriter_PushesOnFlush(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/loki/api/v1/push" {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	var buf bytes.Buffer
	l := NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)
	l.Info(context.Background()).Msg("push test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Flush(ctx, l); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	mu.Lock()
	got := len(bodies)
	var firstBody []byte
	if got > 0 {
		firstBody = bodies[0]
	}
	mu.Unlock()

	if got == 0 {
		t.Fatal("expected at least one POST to /loki/api/v1/push, got 0")
	}
	var parsed map[string]any
	if err := json.Unmarshal(firstBody, &parsed); err != nil {
		t.Fatalf("push body is not valid JSON: %v\nbody: %s", err, firstBody)
	}
	if _, ok := parsed["streams"]; !ok {
		t.Fatalf("push body missing \"streams\" key: %s", firstBody)
	}
}

// TestLokiWriter_NetworkFailure validates fail-open behavior: when the Loki server
// returns 503, the logger must not panic and Flush must return nil (network failures
// are silent). FR-005 / SC-002: non-blocking, fail-open.
func TestLokiWriter_NetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	var buf bytes.Buffer
	l := NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)
	l.Info(context.Background()).Msg("network failure test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Flush(ctx, l); err != nil {
		t.Fatalf("Flush must return nil on network failure, got: %v", err)
	}
}

// TestLokiWriter_StatusErrorUsesConfiguredWriter validates that non-2xx Loki
// responses are reported through the logger's configured writer, not the
// process's real stderr. This preserves ax.Execute's stderr capture contract.
func TestLokiWriter_StatusErrorUsesConfiguredWriter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	var buf bytes.Buffer
	l := NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)
	l.Info(context.Background()).Msg("status error writer test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Flush(ctx, l); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "ax: loki push returned 503") {
		t.Fatalf("expected Loki status error in configured writer, got: %q", buf.String())
	}
}

// TestLokiWriter_BufferFull validates non-blocking Write behavior: writing 300 log
// lines (more than the 256-entry channel buffer) must not block any goroutine.
// All 300 Write calls must return within the test deadline. FR-004 / SC-003.
func TestLokiWriter_BufferFull(t *testing.T) {
	// unblock is closed by the test cleanup to let handler goroutines exit before
	// srv.Close() waits on them. This avoids a deadlock where Close() waits for
	// active handlers that are themselves waiting for the connection to close.
	unblock := make(chan struct{})

	// Server that blocks each request until unblock is closed, simulating a
	// slow/stuck Loki endpoint. The background goroutine's HTTP requests will
	// hang here, proving that Write() is non-blocking regardless.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-unblock:
		case <-r.Context().Done():
		}
	}))
	defer func() {
		close(unblock) // unblock all pending handler goroutines
		srv.Close()    // now safe: no active handlers blocking Close
	}()

	t.Setenv("AX_LOKI_URL", srv.URL)
	var buf bytes.Buffer
	l := NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 300 {
			l.Info(context.Background()).Msg("buffer full test")
		}
	}()

	deadline, ok := t.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	select {
	case <-done:
		// All writes returned — pass.
	case <-time.After(time.Until(deadline) - 500*time.Millisecond):
		t.Fatal("Write calls blocked: goroutine did not return before deadline")
	}

	// Flush with a short timeout to avoid blocking the test.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = Flush(ctx, l)
}

// TestLokiWriter_Race validates concurrent safety: 10 goroutines each writing 50
// log lines while Flush is called concurrently must not produce data races.
// Requires -race. SC-004 / Constitution Principle IX (resource safety).
func TestLokiWriter_Race(t *testing.T) {
	var received int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = io.ReadAll(r.Body)
			atomic.AddInt64(&received, 1)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	// Use io.Discard rather than bytes.Buffer: bytes.Buffer is not goroutine-safe
	// and 10 concurrent goroutines writing to the same logger would race on it.
	// This test validates HTTP delivery and race-freedom, not log output content.
	l := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
	)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				l.Info(context.Background()).Msg("race test")
			}
		}()
	}

	// Flush concurrently with the writes.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		_ = Flush(ctx, l)
	}()

	wg.Wait()
}

// FuzzLokiWrite exercises lokiWriter.Write with arbitrary byte sequences.
// Write must always return (len(p), nil) and must never panic.
// Constitution Principle VII: fuzz targets for every parser/write surface.
func FuzzLokiWrite(f *testing.F) {
	f.Add([]byte(`{"level":"info","message":"hello"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte("not json at all"))
	f.Fuzz(func(t *testing.T, p []byte) {
		lw := &lokiWriter{
			ch:            make(chan lokiEntry, lokiChannelCap),
			flushRequests: make(chan chan struct{}),
			done:          make(chan struct{}),
		}
		n, err := lw.Write(p)
		if err != nil {
			t.Errorf("Write returned error: %v", err)
		}
		if n != len(p) {
			t.Errorf("Write returned n=%d, want %d", n, len(p))
		}
	})
}

// TestLokiAuth_BearerToken validates that when AX_LOKI_AUTH_TOKEN is set,
// push requests to Loki carry an Authorization: Bearer <token> header.
// US3 / FR-007: authenticated Loki push.
func TestLokiAuth_BearerToken(t *testing.T) {
	const token = "test-secret-token"

	var mu sync.Mutex
	var authHeaders []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			mu.Lock()
			authHeaders = append(authHeaders, r.Header.Get("Authorization"))
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	t.Setenv("AX_LOKI_AUTH_TOKEN", token)

	var buf bytes.Buffer
	l := NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)
	l.Info(context.Background()).Msg("auth test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = Flush(ctx, l)

	mu.Lock()
	headers := authHeaders
	mu.Unlock()

	if len(headers) == 0 {
		t.Fatal("expected at least one request with Authorization header, got none")
	}
	want := "Bearer " + token
	if headers[0] != want {
		t.Errorf("Authorization header = %q, want %q", headers[0], want)
	}
}

// TestLokiAuth_InsecureURLWarning validates that WithLokiFromEnv emits a warning
// message to the logger's configured writer when AX_LOKI_URL uses plain HTTP
// with a non-loopback host. US3 / FR-008: secure transport enforcement.
func TestLokiAuth_InsecureURLWarning(t *testing.T) {
	t.Setenv("AX_LOKI_URL", "http://example.com:3100")

	var buf bytes.Buffer
	_ = NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)

	got := buf.String()
	if !strings.Contains(got, "insecure") && !strings.Contains(got, "http") {
		t.Errorf("expected insecure URL warning in writer output, got: %q", got)
	}
}

// TestLokiCardinality_StreamLabelsOnly5Keys validates the FR-009 cardinality
// contract: Loki stream labels contain at most the five permitted low-cardinality
// fields; trace_id, span_id, and all other fields appear only in the log-line
// body, never as stream keys.
func TestLokiCardinality_StreamLabelsOnly5Keys(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	l := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLoggerLabels(Labels{
			Environment: "prod",
			Application: "app",
			Host:        "h1",
			Version:     "1.0",
		}),
		WithLokiFromEnv(),
	)
	l.Info(context.Background()).Msg("cardinality test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = Flush(ctx, l)

	mu.Lock()
	captured := bodies
	mu.Unlock()

	if len(captured) == 0 {
		t.Fatal("expected at least one push request, got 0")
	}

	// Permitted stream label keys. trace_id and span_id must NOT appear.
	permitted := map[string]bool{
		"environment": true,
		"application": true,
		"host":        true,
		"version":     true,
		"level":       true,
	}

	for _, body := range captured {
		var pushBody lokiPushBody
		if err := json.Unmarshal(body, &pushBody); err != nil {
			t.Fatalf("push body is not valid JSON: %v\nbody: %s", err, body)
		}
		for _, stream := range pushBody.Streams {
			for key := range stream.Stream {
				if !permitted[key] {
					t.Errorf("stream label key %q is not in permitted set %v", key, permitted)
				}
			}
			if _, has := stream.Stream["trace_id"]; has {
				t.Error("trace_id must not appear as a stream label key")
			}
			if _, has := stream.Stream["span_id"]; has {
				t.Error("span_id must not appear as a stream label key")
			}
		}
	}
}

// TestLokiLevelExtraction validates extractLevel across all zerolog level strings
// and edge cases including missing level, empty input, and malformed JSON.
// US4 / FR-009: level extraction drives stream grouping for cardinality discipline.
func TestLokiLevelExtraction(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"debug", `{"level":"debug","message":"hi"}`, "debug"},
		{"info", `{"level":"info","message":"hi"}`, "info"},
		{"warn", `{"level":"warn","message":"hi"}`, "warn"},
		{"error", `{"level":"error","message":"hi"}`, "error"},
		{"panic", `{"level":"panic","message":"hi"}`, "panic"},
		{"no level field", `{"message":"no level"}`, "unknown"},
		{"empty input", ``, "unknown"},
		{"malformed JSON", `not json`, "unknown"},
		{"level value empty string", `{"level":""}`, "unknown"},
		{"level key but no closing quote", `{"level":"info`, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractLevel(tc.input)
			if got != tc.want {
				t.Errorf("extractLevel(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// collectPushTimestamps decodes captured Loki push bodies and returns every
// per-entry timestamp string across all streams, preserving order.
func collectPushTimestamps(t *testing.T, bodies [][]byte) []string {
	t.Helper()
	var timestamps []string
	for _, body := range bodies {
		var pb lokiPushBody
		if err := json.Unmarshal(body, &pb); err != nil {
			t.Fatalf("push body is not valid JSON: %v\nbody: %s", err, body)
		}
		for _, s := range pb.Streams {
			for _, v := range s.Values {
				timestamps = append(timestamps, v[0])
			}
		}
	}
	return timestamps
}

// collectPushStreamLabels decodes captured Loki push bodies and returns every
// stream label map across all streams.
func collectPushStreamLabels(t *testing.T, bodies [][]byte) []map[string]string {
	t.Helper()
	var labels []map[string]string
	for _, body := range bodies {
		var pb lokiPushBody
		if err := json.Unmarshal(body, &pb); err != nil {
			t.Fatalf("push body is not valid JSON: %v\nbody: %s", err, body)
		}
		for _, stream := range pb.Streams {
			labels = append(labels, stream.Stream)
		}
	}
	return labels
}

func hasStreamLabels(streams []map[string]string, want Labels) bool {
	for _, stream := range streams {
		if want.Environment != "" && stream[lokiLabelEnvironment] != want.Environment {
			continue
		}
		if want.Application != "" && stream[lokiLabelApplication] != want.Application {
			continue
		}
		if want.Host != "" && stream[lokiLabelHost] != want.Host {
			continue
		}
		if want.Version != "" && stream[lokiLabelVersion] != want.Version {
			continue
		}
		return true
	}
	return false
}

// TestLokiWriter_PerEntryTimestamps validates that each log entry carries its own
// enqueue-time timestamp rather than a single batch-wide flush timestamp. Two
// entries written a few milliseconds apart must end up with distinct timestamps
// in the push body (regression guard for the batch-wide time.Now() defect).
func TestLokiWriter_PerEntryTimestamps(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	l := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
	)
	l.Info(context.Background()).Msg("first")
	time.Sleep(5 * time.Millisecond)
	l.Info(context.Background()).Msg("second")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = Flush(ctx, l)

	mu.Lock()
	captured := bodies
	mu.Unlock()

	timestamps := collectPushTimestamps(t, captured)
	if len(timestamps) < 2 {
		t.Fatalf("expected at least 2 pushed entries, got %d", len(timestamps))
	}
	allSame := true
	for _, ts := range timestamps {
		if ts != timestamps[0] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Fatalf("all %d entries share timestamp %q; expected per-entry timestamps", len(timestamps), timestamps[0])
	}
}

// TestLokiStreamLabelsFollowLoggerLabelsOptionOrder validates that Loki stream
// labels are extracted from the emitted log line, not snapshotted when
// WithLokiFromEnv runs. Low-cardinality labels added by a later option must be
// indexed in Loki as well as present in the JSON body.
func TestLokiStreamLabelsFollowLoggerLabelsOptionOrder(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	want := Labels{Environment: "prod", Application: "option-order"}
	l := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
		WithLoggerLabels(want),
	)
	l.Info(context.Background()).Msg("option-order labels")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = Flush(ctx, l)

	mu.Lock()
	captured := bodies
	mu.Unlock()
	streams := collectPushStreamLabels(t, captured)
	if !hasStreamLabels(streams, want) {
		t.Fatalf("stream labels did not include %+v; got %#v", want, streams)
	}
}

// TestLokiStreamLabelsFollowDerivedLoggerLabels validates that labels added by
// Logger.WithLabels are also propagated into the Loki stream label map.
func TestLokiStreamLabelsFollowDerivedLoggerLabels(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	base := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
	)
	want := Labels{Environment: "prod", Application: "derived"}
	derived := base.WithLabels(want)
	derived.Info(context.Background()).Msg("derived labels")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = Flush(ctx, derived)

	mu.Lock()
	captured := bodies
	mu.Unlock()
	streams := collectPushStreamLabels(t, captured)
	if !hasStreamLabels(streams, want) {
		t.Fatalf("stream labels did not include %+v; got %#v", want, streams)
	}
}

// TestLokiWriter_WithLabelsFlush validates that a logger derived via WithLabels
// retains its Loki sink so that ax.Flush drains buffered entries. Regression
// guard for WithLabels dropping the sinks slice (which made Flush a silent no-op
// on derived loggers).
func TestLokiWriter_WithLabelsFlush(t *testing.T) {
	var mu sync.Mutex
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			mu.Lock()
			count++
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	base := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
	)
	derived := base.WithLabels(Labels{Environment: "prod"})
	derived.Info(context.Background()).Msg("via derived logger")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Flush(ctx, derived); err != nil {
		t.Fatalf("Flush on derived logger returned error: %v", err)
	}

	mu.Lock()
	got := count
	mu.Unlock()
	if got == 0 {
		t.Fatal("Flush on WithLabels-derived logger drained nothing; sinks not propagated")
	}
}

// TestLokiWriter_FlushNonDestructive validates that Flush drains pending entries
// without shutting down the Loki sink. Later writes must still be delivered by a
// later Flush call.
func TestLokiWriter_FlushNonDestructive(t *testing.T) {
	var mu sync.Mutex
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			mu.Lock()
			count++
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	l := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l.Info(context.Background()).Msg("first")
	if err := Flush(ctx, l); err != nil {
		t.Fatalf("first Flush returned error: %v", err)
	}
	l.Info(context.Background()).Msg("second")
	if err := Flush(ctx, l); err != nil {
		t.Fatalf("second Flush returned error: %v", err)
	}

	mu.Lock()
	got := count
	mu.Unlock()
	if got < 2 {
		t.Fatalf("expected at least two push requests across two Flush calls, got %d", got)
	}
}

// TestLokiURL_NormalizesPushPath validates that AX_LOKI_URL values with trailing
// slashes or base paths produce a correct push path (no double slashes, base
// path preserved). Regression guard for naive string concatenation.
func TestLokiURL_NormalizesPushPath(t *testing.T) {
	cases := []struct {
		name     string
		suffix   string
		wantPath string
	}{
		{"no trailing slash", "", "/loki/api/v1/push"},
		{"trailing slash", "/", "/loki/api/v1/push"},
		{"base path", "/base", "/base/loki/api/v1/push"},
		{"base path trailing slash", "/base/", "/base/loki/api/v1/push"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var paths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			t.Setenv("AX_LOKI_URL", srv.URL+tc.suffix)
			l := NewLogger(context.Background(),
				WithLoggerWriter(io.Discard),
				WithLokiFromEnv(),
			)
			l.Info(context.Background()).Msg("path test")

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = Flush(ctx, l)

			mu.Lock()
			got := paths
			mu.Unlock()
			if len(got) == 0 {
				t.Fatal("expected at least one request, got 0")
			}
			if got[0] != tc.wantPath {
				t.Errorf("push path = %q, want %q", got[0], tc.wantPath)
			}
		})
	}
}

// TestLokiURL_RejectsNonHTTPScheme validates that a non-http(s) AX_LOKI_URL
// scheme is rejected: no sink is created and a warning is emitted. Regression
// guard for url.Parse accepting arbitrary schemes (e.g. ftp://).
func TestLokiURL_RejectsNonHTTPScheme(t *testing.T) {
	t.Setenv("AX_LOKI_URL", "ftp://example.com:3100")
	var buf bytes.Buffer
	l := NewLogger(context.Background(),
		WithLoggerWriter(&buf),
		WithLokiFromEnv(),
	)
	zl, ok := l.(zerologLogger)
	if !ok {
		t.Fatal("NewLogger did not return zerologLogger")
	}
	if len(zl.sinks) != 0 {
		t.Fatalf("expected no sink for non-http(s) scheme, got %d", len(zl.sinks))
	}
	if !strings.Contains(buf.String(), "scheme") {
		t.Errorf("expected a scheme warning in writer output, got: %q", buf.String())
	}
}

// TestLokiWriter_ContextCancelStopsGoroutine validates that cancelling the
// context passed to NewLogger stops the background run goroutine (it closes its
// done channel). Regression guard for the goroutine ignoring the logger context
// and only exiting via Flush.
func TestLokiWriter_ContextCancelStopsGoroutine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("AX_LOKI_URL", srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	l := NewLogger(ctx,
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
	)
	zl, ok := l.(zerologLogger)
	if !ok || len(zl.sinks) == 0 {
		t.Fatal("expected a Loki sink to be present")
	}
	lw, ok := zl.sinks[0].(*lokiWriter)
	if !ok {
		t.Fatalf("sink is %T, want *lokiWriter", zl.sinks[0])
	}

	l.Info(context.Background()).Msg("ctx cancel test")
	cancel()

	select {
	case <-lw.done:
		// goroutine exited in response to context cancellation — pass.
	case <-time.After(3 * time.Second):
		t.Fatal("run goroutine did not exit after context cancellation")
	}
}
