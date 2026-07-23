package logcore

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

// recordingSink implements Sink only. It is the C-02 negative case: New must
// leave it alone rather than rejecting it, because the sink seam stays fully
// generic (a future file-rotator has no label concept).
type recordingSink struct {
	mu      sync.Mutex
	written []string
	drains  int
	drainer func(context.Context) error
}

func (s *recordingSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.written = append(s.written, string(p))
	return len(p), nil
}

func (s *recordingSink) Drain(ctx context.Context) error {
	s.mu.Lock()
	s.drains++
	drainer := s.drainer
	s.mu.Unlock()
	if drainer != nil {
		return drainer(ctx)
	}
	return nil
}

func (s *recordingSink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.written...)
}

func (s *recordingSink) drainCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drains
}

// sanctioningSink implements both Sink and LabelSanctioner, standing in for
// lokiWriter without importing anything Loki-specific.
type sanctioningSink struct {
	recordingSink

	labelMu    sync.Mutex
	sanctioned []Labels
}

func (s *sanctioningSink) SanctionLabels(labels Labels) {
	s.labelMu.Lock()
	defer s.labelMu.Unlock()
	s.sanctioned = append(s.sanctioned, labels)
}

func (s *sanctioningSink) sanctions() []Labels {
	s.labelMu.Lock()
	defer s.labelMu.Unlock()
	return append([]Labels(nil), s.sanctioned...)
}

// withSinks is a test-local Option that registers additional sinks, standing in
// for root ax's WithLokiFromEnv without importing it.
func withSinks(sinks ...Sink) Option {
	return func(cfg *Config) {
		cfg.AdditionalSinks = append(cfg.AdditionalSinks, sinks...)
	}
}

func decodeLine(t *testing.T, line string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("log line %q was not JSON: %v", line, err)
	}
	return got
}

// TestNewAppliesOptionsBeforeSanctioning covers C-01: every Option is applied
// before any sink is sanctioned, so option order is irrelevant. The sink must
// observe the FINAL label set no matter where the label option sits.
func TestNewAppliesOptionsBeforeSanctioning(t *testing.T) {
	labels := Labels{Application: "app", Environment: "test"}

	cases := []struct {
		name string
		opts func(sink Sink) []Option
	}{
		{
			name: "sink_registered_before_labels",
			opts: func(sink Sink) []Option {
				return []Option{withSinks(sink), WithLabels(labels)}
			},
		},
		{
			name: "sink_registered_after_labels",
			opts: func(sink Sink) []Option {
				return []Option{WithLabels(labels), withSinks(sink)}
			},
		},
		{
			name: "labels_overwritten_by_later_option",
			opts: func(sink Sink) []Option {
				return []Option{
					WithLabels(Labels{Application: "stale"}),
					withSinks(sink),
					WithLabels(labels),
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &sanctioningSink{}
			opts := append([]Option{WithWriter(io.Discard)}, tc.opts(sink)...)
			if got := New(context.Background(), opts...); got == nil {
				t.Fatal("New returned nil")
			}

			sanctions := sink.sanctions()
			if len(sanctions) != 1 {
				t.Fatalf("SanctionLabels called %d times, want 1", len(sanctions))
			}
			if sanctions[0] != labels {
				t.Fatalf("sanctioned %+v, want %+v", sanctions[0], labels)
			}
		})
	}
}

// TestNewSanctionsOnlyLabelSanctioners covers C-02, the FR-010 generic-seam
// regression guard: a sink implementing only Sink is written to normally and is
// never rejected, while one implementing LabelSanctioner is sanctioned. If a
// future refactor reintroduces a concrete-type assertion, the plain sink stops
// receiving output or construction starts failing, and this test catches it.
func TestNewSanctionsOnlyLabelSanctioners(t *testing.T) {
	plain := &recordingSink{}
	sanctioning := &sanctioningSink{}
	labels := Labels{Application: "app"}

	logger := New(
		context.Background(),
		WithWriter(io.Discard),
		withSinks(plain, sanctioning),
		WithLabels(labels),
	)
	logger.Info(context.Background()).Msg("hello")

	if got := len(sanctioning.sanctions()); got != 1 {
		t.Fatalf("LabelSanctioner sink sanctioned %d times, want 1", got)
	}
	if got := len(plain.lines()); got != 1 {
		t.Fatalf("plain Sink received %d lines, want 1 (it must not be rejected)", got)
	}
	if got := len(sanctioning.lines()); got != 1 {
		t.Fatalf("sanctioning Sink received %d lines, want 1", got)
	}
}

// TestNewFansOutThroughMultiWriter covers C-03: with sinks present the primary
// writer and every sink receive byte-identical output.
func TestNewFansOutThroughMultiWriter(t *testing.T) {
	var primary bytes.Buffer
	first := &recordingSink{}
	second := &recordingSink{}

	logger := New(
		context.Background(),
		WithWriter(&primary),
		withSinks(first, second),
	)
	logger.Info(context.Background()).Msg("fan out")

	want := primary.String()
	if want == "" {
		t.Fatal("primary writer received nothing")
	}
	for name, sink := range map[string]*recordingSink{"first": first, "second": second} {
		lines := sink.lines()
		if len(lines) != 1 {
			t.Fatalf("%s sink received %d lines, want 1", name, len(lines))
		}
		if lines[0] != want {
			t.Fatalf("%s sink got %q, want byte-identical %q", name, lines[0], want)
		}
	}
}

// TestNewWithoutSinksDoesNotWrapWriter covers the C-03 complement: with no sinks
// the configured writer is used directly, so the no-sink path adds no fan-out
// cost.
func TestNewWithoutSinksDoesNotWrapWriter(t *testing.T) {
	var primary bytes.Buffer
	logger := New(context.Background(), WithWriter(&primary))
	logger.Info(context.Background()).Msg("direct")

	if got := strings.Count(primary.String(), "\n"); got != 1 {
		t.Fatalf("primary writer got %d lines, want 1", got)
	}
}

// TestApplyLabelsOmitsEmptyFields covers C-07: an empty Labels field is omitted
// entirely rather than emitted as an empty string, so consumers can distinguish
// "not set" from "set to empty".
func TestApplyLabelsOmitsEmptyFields(t *testing.T) {
	cases := []struct {
		name    string
		labels  Labels
		present map[string]string
		absent  []string
	}{
		{
			name:    "all_empty",
			labels:  Labels{},
			present: map[string]string{},
			absent:  []string{labelFieldEnvironment, labelFieldApplication, labelFieldHost, labelFieldVersion},
		},
		{
			name:    "only_application",
			labels:  Labels{Application: "app"},
			present: map[string]string{labelFieldApplication: "app"},
			absent:  []string{labelFieldEnvironment, labelFieldHost, labelFieldVersion},
		},
		{
			name:   "all_populated",
			labels: Labels{Environment: "prod", Application: "app", Host: "h1", Version: "v1.2.3"},
			present: map[string]string{
				labelFieldEnvironment: "prod",
				labelFieldApplication: "app",
				labelFieldHost:        "h1",
				labelFieldVersion:     "v1.2.3",
			},
			absent: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(context.Background(), WithWriter(&buf), WithLabels(tc.labels))
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

// TestWithLabelsDerivesAndReSanctions covers C-08: the derived logger carries
// sinks forward (so a later Flush still drains them) and re-sanctions the new
// label pairs on every LabelSanctioner sink.
func TestWithLabelsDerivesAndReSanctions(t *testing.T) {
	var buf bytes.Buffer
	sink := &sanctioningSink{}
	base := Labels{Application: "app"}
	derivedLabels := Labels{Application: "app", Version: "v2"}

	logger := New(context.Background(), WithWriter(&buf), withSinks(sink), WithLabels(base))
	derived := logger.WithLabels(derivedLabels)

	if derived == nil {
		t.Fatal("WithLabels returned nil")
	}

	sanctions := sink.sanctions()
	if len(sanctions) != 2 {
		t.Fatalf("SanctionLabels called %d times, want 2 (construction + derivation)", len(sanctions))
	}
	if sanctions[1] != derivedLabels {
		t.Fatalf("derived sanction %+v, want %+v", sanctions[1], derivedLabels)
	}

	buf.Reset()
	derived.Info(context.Background()).Msg("derived")
	got := decodeLine(t, buf.String())
	if got[labelFieldVersion] != "v2" {
		t.Fatalf("version = %v, want v2 on the derived logger", got[labelFieldVersion])
	}

	// Sinks carried forward: the derived logger's emission reached the sink, and
	// Flush drains it.
	if len(sink.lines()) == 0 {
		t.Fatal("derived logger did not carry its sinks forward")
	}
	if err := Flush(context.Background(), derived); err != nil {
		t.Fatalf("Flush on derived logger: %v", err)
	}
	if sink.drainCount() != 1 {
		t.Fatalf("derived logger drained %d sinks, want 1", sink.drainCount())
	}
}

// TestNewDefaults covers the construction defaults the public surfaces inherit:
// a non-nil logger, stderr as the default writer, and info as the default level.
func TestNewDefaults(t *testing.T) {
	logger := New(context.Background())
	if logger == nil {
		t.Fatal("New returned nil")
	}
	if got := logger.Zerolog().GetLevel(); got != zerolog.InfoLevel {
		t.Fatalf("default level = %v, want %v", got, zerolog.InfoLevel)
	}
}

// TestNewSkipsNilOptions pins that a nil entry in the variadic option list does
// not panic. New never returns an error, so nil options are skipped rather than
// rejected the way config.applyOptions fails closed.
func TestNewSkipsNilOptions(t *testing.T) {
	var buf bytes.Buffer
	logger := New(context.Background(), nil, WithWriter(&buf), nil, WithLevel(zerolog.InfoLevel))
	if logger == nil {
		t.Fatal("New returned nil with nil options in the list")
	}
	logger.Info(context.Background()).Msg("ok")
	if !strings.Contains(buf.String(), "ok") {
		t.Fatalf("expected emit through non-nil options, got %q", buf.String())
	}
}

// TestWithLevelFiltersBelowThreshold covers the level option end to end.
func TestWithLevelFiltersBelowThreshold(t *testing.T) {
	cases := []struct {
		name    string
		level   zerolog.Level
		emit    func(Logger, context.Context)
		wantOut bool
	}{
		{
			name:    "debug_filtered_at_info",
			level:   zerolog.InfoLevel,
			emit:    func(l Logger, ctx context.Context) { l.Debug(ctx).Msg("x") },
			wantOut: false,
		},
		{
			name:    "debug_emitted_at_debug",
			level:   zerolog.DebugLevel,
			emit:    func(l Logger, ctx context.Context) { l.Debug(ctx).Msg("x") },
			wantOut: true,
		},
		{
			name:    "warn_emitted_at_info",
			level:   zerolog.InfoLevel,
			emit:    func(l Logger, ctx context.Context) { l.Warn(ctx).Msg("x") },
			wantOut: true,
		},
		{
			name:    "error_emitted_at_info",
			level:   zerolog.InfoLevel,
			emit:    func(l Logger, ctx context.Context) { l.Error(ctx).Msg("x") },
			wantOut: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := New(context.Background(), WithWriter(&buf), WithLevel(tc.level))
			tc.emit(logger, context.Background())

			if gotOut := buf.Len() > 0; gotOut != tc.wantOut {
				t.Fatalf("output present = %v, want %v (got %q)", gotOut, tc.wantOut, buf.String())
			}
		})
	}
}

// TestZerologReturnsIndependentHandle asserts the escape hatch hands back a
// usable handle without letting the caller mutate the logger's own copy.
func TestZerologReturnsIndependentHandle(t *testing.T) {
	var buf bytes.Buffer
	logger := New(context.Background(), WithWriter(&buf))

	handle := logger.Zerolog()
	if handle == nil {
		t.Fatal("Zerolog returned nil")
	}
	*handle = handle.Level(zerolog.Disabled)

	logger.Info(context.Background()).Msg("still emitted")
	if buf.Len() == 0 {
		t.Fatal("mutating the returned handle disabled the original logger")
	}
}
