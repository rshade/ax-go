package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/logging"
)

const (
	fieldTraceID = "trace_id"
	fieldSpanID  = "span_id"

	testTraceIDHex = "0102030405060708090a0b0c0d0e0f10"
	testSpanIDHex  = "0102030405060708"
)

// activeSpanContext builds a context carrying a valid, non-zero W3C span context
// without standing up the OpenTelemetry SDK — which this package could not import
// even if it wanted to.
func activeSpanContext(tb testing.TB) context.Context {
	tb.Helper()

	traceID, err := oteltrace.TraceIDFromHex(testTraceIDHex)
	if err != nil {
		tb.Fatalf("TraceIDFromHex: %v", err)
	}
	spanID, err := oteltrace.SpanIDFromHex(testSpanIDHex)
	if err != nil {
		tb.Fatalf("SpanIDFromHex: %v", err)
	}
	return oteltrace.ContextWithSpanContext(
		context.Background(),
		oteltrace.NewSpanContext(oteltrace.SpanContextConfig{TraceID: traceID, SpanID: spanID}),
	)
}

func decodeLine(t *testing.T, line string) map[string]any {
	t.Helper()

	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("log line %q was not JSON: %v", line, err)
	}
	return got
}

// TestNewLoggerNeverReturnsNil covers L-01 across the option shapes a caller can
// reach, including the degenerate ones. A constructor that can return nil forces
// every call site to branch, and no ax-go constructor does.
func TestNewLoggerNeverReturnsNil(t *testing.T) {
	cases := []struct {
		name string
		opts []logging.LoggerOption
	}{
		{name: "no_options", opts: nil},
		{name: "empty_option_slice", opts: []logging.LoggerOption{}},
		{name: "writer_only", opts: []logging.LoggerOption{logging.WithLoggerWriter(io.Discard)}},
		{
			name: "all_options",
			opts: []logging.LoggerOption{
				logging.WithLoggerWriter(io.Discard),
				logging.WithLoggerLevel(zerolog.DebugLevel),
				logging.WithLoggerLabels(logging.Labels{Application: "app"}),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := logging.NewLogger(context.Background(), tc.opts...); got == nil {
				t.Fatal("NewLogger returned nil")
			}
		})
	}
}

// TestNewLoggerDefaults covers L-02: the default writer is stderr and the default
// level is info. The level is asserted through the escape hatch; the writer is
// asserted in TestOutputGoesToDiagnosticStream, which can observe the real
// process streams.
func TestNewLoggerDefaults(t *testing.T) {
	logger := logging.NewLogger(context.Background())

	if got := logger.Zerolog().GetLevel(); got != zerolog.InfoLevel {
		t.Fatalf("default level = %v, want %v", got, zerolog.InfoLevel)
	}
}

// TestOutputGoesToDiagnosticStream covers L-03 and Constitution Principle I: the
// payload stream is reserved for the final machine payload, and a library that
// leaked one log line onto it would corrupt every agent parsing that stream.
//
// The real os.Stdout and os.Stderr are swapped for pipes rather than passing a
// buffer through WithLoggerWriter, because the point is to prove the DEFAULT
// destination is correct — a test that configures the writer cannot observe the
// default it is overriding.
func TestOutputGoesToDiagnosticStream(t *testing.T) {
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}

	origStdout, origStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutW, stderrW //nolint:reassign // redirect the process streams to prove the DEFAULT writer lands on stderr; a writer-injected test cannot observe the default it overrides
	t.Cleanup(func() {
		os.Stdout, os.Stderr = origStdout, origStderr //nolint:reassign // restore the process streams after capture
	})

	logger := logging.NewLogger(context.Background())
	logger.Info(context.Background()).Msg("diagnostic stream")

	if closeErr := stdoutW.Close(); closeErr != nil {
		t.Fatalf("close stdout writer: %v", closeErr)
	}
	if closeErr := stderrW.Close(); closeErr != nil {
		t.Fatalf("close stderr writer: %v", closeErr)
	}

	stdoutBytes, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if len(stdoutBytes) != 0 {
		t.Errorf("log output leaked to the payload stream: %q", stdoutBytes)
	}
	if !strings.Contains(string(stderrBytes), "diagnostic stream") {
		t.Errorf("log line absent from the diagnostic stream, got %q", stderrBytes)
	}
}

// TestTraceCorrelation covers L-04 and the identifier half of L-05: an active
// span contributes its real hex IDs, and no active span yields the zero-value
// valid hex constants rather than an absent or empty field.
func TestTraceCorrelation(t *testing.T) {
	cases := []struct {
		name        string
		ctx         func(testing.TB) context.Context
		wantTraceID string
		wantSpanID  string
	}{
		{
			name:        "active_span",
			ctx:         activeSpanContext,
			wantTraceID: testTraceIDHex,
			wantSpanID:  testSpanIDHex,
		},
		{
			name:        "no_active_span",
			ctx:         func(testing.TB) context.Context { return context.Background() },
			wantTraceID: contract.ZeroTraceID,
			wantSpanID:  contract.ZeroSpanID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.NewLogger(context.Background(), logging.WithLoggerWriter(&buf))
			logger.Info(tc.ctx(t)).Msg("correlated")

			got := decodeLine(t, buf.String())
			if got[fieldTraceID] != tc.wantTraceID {
				t.Errorf("%s = %v, want %q", fieldTraceID, got[fieldTraceID], tc.wantTraceID)
			}
			if got[fieldSpanID] != tc.wantSpanID {
				t.Errorf("%s = %v, want %q", fieldSpanID, got[fieldSpanID], tc.wantSpanID)
			}
		})
	}
}

// TestNoActiveSpanPathIsAllocationFree covers the allocation half of L-05. Agents
// emit liberally, so the no-span path is the one that runs millions of times; it
// is allocation-free because the IDs are package constants rather than freshly
// hex-encoded strings.
func TestNoActiveSpanPathIsAllocationFree(t *testing.T) {
	logger := logging.NewLogger(context.Background(), logging.WithLoggerWriter(io.Discard))
	ctx := context.Background()

	avg := testing.AllocsPerRun(100, func() {
		logger.Info(ctx).Msg("allocation contract")
	})

	if avg != 0 {
		t.Fatalf("no-active-span emission allocated %v times per run, want 0 (L-05)", avg)
	}
}

// TestOptionsAreOrderIndependent covers L-06 for the options this surface owns.
// The cross-surface half — interaction with root ax's WithLokiFromEnv — is
// asserted in the root package, where the option and its sink live.
func TestOptionsAreOrderIndependent(t *testing.T) {
	labels := logging.Labels{Application: "app", Environment: "test"}

	orderings := map[string]func(w io.Writer) []logging.LoggerOption{
		"writer_level_labels": func(w io.Writer) []logging.LoggerOption {
			return []logging.LoggerOption{
				logging.WithLoggerWriter(w),
				logging.WithLoggerLevel(zerolog.DebugLevel),
				logging.WithLoggerLabels(labels),
			}
		},
		"labels_writer_level": func(w io.Writer) []logging.LoggerOption {
			return []logging.LoggerOption{
				logging.WithLoggerLabels(labels),
				logging.WithLoggerWriter(w),
				logging.WithLoggerLevel(zerolog.DebugLevel),
			}
		},
		"level_labels_writer": func(w io.Writer) []logging.LoggerOption {
			return []logging.LoggerOption{
				logging.WithLoggerLevel(zerolog.DebugLevel),
				logging.WithLoggerLabels(labels),
				logging.WithLoggerWriter(w),
			}
		},
	}

	var want string
	for name, build := range orderings {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.NewLogger(context.Background(), build(&buf)...)
			logger.Debug(context.Background()).Msg("ordering")

			if buf.Len() == 0 {
				t.Fatal("no output: WithLoggerLevel did not take effect")
			}
			if want == "" {
				want = buf.String()
				return
			}
			if buf.String() != want {
				t.Fatalf("output %q differs from another ordering's %q", buf.String(), want)
			}
		})
	}
}

// TestEmptyLabelFieldsAreOmitted covers L-07: an unset label field is absent from
// the line entirely, so a consumer can distinguish "not set" from "set to empty".
func TestEmptyLabelFieldsAreOmitted(t *testing.T) {
	cases := []struct {
		name    string
		labels  logging.Labels
		present map[string]string
		absent  []string
	}{
		{
			name:    "all_empty",
			labels:  logging.Labels{},
			present: map[string]string{},
			absent:  []string{"environment", "application", "host", "version"},
		},
		{
			name:    "partial",
			labels:  logging.Labels{Application: "app", Version: "v1"},
			present: map[string]string{"application": "app", "version": "v1"},
			absent:  []string{"environment", "host"},
		},
		{
			name:   "complete",
			labels: logging.Labels{Environment: "prod", Application: "app", Host: "h1", Version: "v1"},
			present: map[string]string{
				"environment": "prod",
				"application": "app",
				"host":        "h1",
				"version":     "v1",
			},
			absent: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.NewLogger(
				context.Background(),
				logging.WithLoggerWriter(&buf),
				logging.WithLoggerLabels(tc.labels),
			)
			logger.Info(context.Background()).Msg("labels")

			got := decodeLine(t, buf.String())
			for key, want := range tc.present {
				if got[key] != want {
					t.Errorf("%s = %v, want %q", key, got[key], want)
				}
			}
			for _, key := range tc.absent {
				if _, ok := got[key]; ok {
					t.Errorf("%s present as %v, want the field omitted entirely", key, got[key])
				}
			}
		})
	}
}

// TestWithLabelsReturnsDerivedLogger covers L-08: the derived logger carries the
// new labels and the original is left untouched, so a request-scoped derivation
// cannot mutate the process-wide logger it came from.
func TestWithLabelsReturnsDerivedLogger(t *testing.T) {
	var buf bytes.Buffer
	base := logging.NewLogger(
		context.Background(),
		logging.WithLoggerWriter(&buf),
		logging.WithLoggerLabels(logging.Labels{Application: "app"}),
	)

	derived := base.WithLabels(logging.Labels{Application: "app", Version: "v2"})
	if derived == nil {
		t.Fatal("WithLabels returned nil")
	}

	derived.Info(context.Background()).Msg("derived")
	derivedLine := decodeLine(t, buf.String())
	if derivedLine["version"] != "v2" {
		t.Fatalf("derived version = %v, want v2", derivedLine["version"])
	}

	buf.Reset()
	base.Info(context.Background()).Msg("base")
	baseLine := decodeLine(t, buf.String())
	if _, ok := baseLine["version"]; ok {
		t.Fatalf("base logger gained version = %v; WithLabels must not mutate its receiver", baseLine["version"])
	}
}

// TestFlushOnNilLoggerReturnsNil covers L-11: Flush is safe to call
// unconditionally, so a shutdown path never has to test before calling.
func TestFlushOnNilLoggerReturnsNil(t *testing.T) {
	if err := logging.Flush(context.Background(), nil); err != nil {
		t.Fatalf("Flush(ctx, nil) = %v, want nil", err)
	}
}

// TestFlushIsANoOpForThisSurface covers the behavioral half of L-12: a logger
// built from this package alone holds no drainable sink, because the only
// buffering destination lives in root ax and is unreachable here.
func TestFlushIsANoOpForThisSurface(t *testing.T) {
	logger := logging.NewLogger(context.Background(), logging.WithLoggerWriter(io.Discard))

	if err := logging.Flush(context.Background(), logger); err != nil {
		t.Fatalf("Flush = %v, want nil for a logging-only logger", err)
	}
}

// TestFlushDocumentsItsNoOpReality covers the DOCUMENTATION half of L-12, which
// FR-012 requires and which no linter can supply.
//
// godoclint's require-doc gates that a doc comment EXISTS; it cannot tell whether
// the comment says the true thing. FR-012's promise — that Flush performs no work
// for consumers of this package alone — is carried entirely by prose, and prose
// that nothing asserts is prose that drifts. Root's documentation_test.go is the
// in-repo precedent for reading a source artifact and asserting its content.
//
// The assertion is deliberately about MEANING, not exact wording: it requires the
// comment to name the no-op behavior and to point at root ax as the place the
// buffering destination lives. A rewrite that keeps both facts passes; a rewrite
// that quietly drops the promise fails.
func TestFlushDocumentsItsNoOpReality(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "logging.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse logging.go: %v", err)
	}

	var doc string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Flush" || fn.Recv != nil {
			continue
		}
		if fn.Doc == nil {
			t.Fatal("Flush has no doc comment")
		}
		doc = fn.Doc.Text()
	}
	if doc == "" {
		t.Fatal("Flush declaration not found in logging.go")
	}

	lower := strings.ToLower(doc)
	requirements := []struct {
		description string
		anyOf       []string
	}{
		{
			description: "states that Flush performs no work for this surface's consumers",
			anyOf:       []string{"no work", "no-op", "nothing to drain"},
		},
		{
			description: "names root ax as where the buffering destination lives",
			anyOf:       []string{"root ax", "root package ax", "package ax"},
		},
	}

	for _, req := range requirements {
		found := false
		for _, phrase := range req.anyOf {
			if strings.Contains(lower, phrase) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf(
				"Flush's doc comment no longer %s (FR-012, L-12); looked for one of %v in:\n%s",
				req.description, req.anyOf, doc,
			)
		}
	}
}
