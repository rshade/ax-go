package logcore

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
)

// orderTrackingSink records the sequence in which Drain was called across a set
// of sinks, so C-09's "in order" clause is asserted rather than assumed.
type orderTrackingSink struct {
	recordingSink

	id    string
	order *[]string
	orMu  *sync.Mutex
	err   error
}

func (s *orderTrackingSink) Drain(ctx context.Context) error {
	s.orMu.Lock()
	*s.order = append(*s.order, s.id)
	s.orMu.Unlock()
	_ = s.recordingSink.Drain(ctx)
	return s.err
}

// notAFlusher is a Logger implementation that does not satisfy the unexported
// flusher interface, exercising C-10's second clause.
type notAFlusher struct{ Logger }

// TestFlushDrainsInOrderAndJoinsErrors covers C-09: every sink is drained, in
// registration order, and the errors are joined so errors.Is finds each one.
func TestFlushDrainsInOrderAndJoinsErrors(t *testing.T) {
	errFirst := errors.New("first sink failed")
	errThird := errors.New("third sink failed")

	var (
		order []string
		orMu  sync.Mutex
	)
	first := &orderTrackingSink{id: "first", order: &order, orMu: &orMu, err: errFirst}
	second := &orderTrackingSink{id: "second", order: &order, orMu: &orMu}
	third := &orderTrackingSink{id: "third", order: &order, orMu: &orMu, err: errThird}

	logger := New(context.Background(), WithWriter(io.Discard), withSinks(first, second, third))

	err := Flush(context.Background(), logger)
	if err == nil {
		t.Fatal("Flush returned nil, want the joined sink errors")
	}
	if !errors.Is(err, errFirst) {
		t.Errorf("errors.Is(err, errFirst) = false; joined error must preserve each sink error")
	}
	if !errors.Is(err, errThird) {
		t.Errorf("errors.Is(err, errThird) = false; joined error must preserve each sink error")
	}

	want := []string{"first", "second", "third"}
	orMu.Lock()
	got := append([]string(nil), order...)
	orMu.Unlock()
	if len(got) != len(want) {
		t.Fatalf("drain order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("drain order = %v, want %v", got, want)
		}
	}
}

// TestFlushDrainsEverySinkDespiteEarlierError asserts a failing sink does not
// short-circuit the ones after it — a drain failure must never leave a later
// sink's buffered entries undelivered.
func TestFlushDrainsEverySinkDespiteEarlierError(t *testing.T) {
	failing := &recordingSink{drainer: func(context.Context) error { return errors.New("boom") }}
	healthy := &recordingSink{}

	logger := New(context.Background(), WithWriter(io.Discard), withSinks(failing, healthy))
	if err := Flush(context.Background(), logger); err == nil {
		t.Fatal("Flush returned nil, want the failing sink's error")
	}
	if healthy.drainCount() != 1 {
		t.Fatalf("healthy sink drained %d times, want 1 despite the earlier failure", healthy.drainCount())
	}
}

// TestFlushReturnsNilForNonFlushableLoggers covers C-10: a nil logger and a
// logger that does not satisfy flusher both yield nil rather than a panic or an
// error.
func TestFlushReturnsNilForNonFlushableLoggers(t *testing.T) {
	cases := []struct {
		name   string
		logger Logger
	}{
		{name: "nil_logger", logger: nil},
		{name: "logger_without_flusher", logger: notAFlusher{}},
		{name: "logger_with_no_sinks", logger: New(context.Background(), WithWriter(io.Discard))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Flush(context.Background(), tc.logger); err != nil {
				t.Fatalf("Flush = %v, want nil", err)
			}
		})
	}
}

// TestFlushNeverPanics covers C-11 (Principle IX: no panic in library code)
// across the paths most likely to produce one: a failing sink, a cancelled
// context, a nil context, and a sink that panics internally is NOT covered —
// a panicking sink is a programmer error in the sink, not in logcore.
func TestFlushNeverPanics(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		sink Sink
	}{
		{
			name: "failing_sink",
			ctx:  context.Background(),
			sink: &recordingSink{drainer: func(context.Context) error { return errors.New("drain failed") }},
		},
		{
			name: "cancelled_context",
			ctx:  cancelled,
			sink: &recordingSink{drainer: func(ctx context.Context) error { return ctx.Err() }},
		},
		{
			// A nil context reaching Flush is a caller mistake, but it must be
			// survivable: shutdown paths are exactly where a half-initialised
			// context shows up, and a panic there loses the buffered entries.
			name: "nil_context",
			ctx:  nil,
			sink: &recordingSink{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Flush panicked: %v", r)
				}
			}()
			logger := New(context.Background(), WithWriter(io.Discard), withSinks(tc.sink))
			_ = Flush(tc.ctx, logger)
		})
	}
}

// TestEmissionNeverPanics covers the C-11 emit half: a sink whose Write reports
// an error must not take the process down, because zerolog's io.Writer path is
// on the caller's hot path.
func TestEmissionNeverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("emission panicked: %v", r)
		}
	}()

	failing := &errorSink{}
	logger := New(context.Background(), WithWriter(io.Discard), withSinks(failing))
	logger.Info(context.Background()).Msg("hello")
}

// errorSink always fails its Write, standing in for a sink whose destination has
// gone away mid-run.
type errorSink struct{}

func (errorSink) Write([]byte) (int, error)   { return 0, errors.New("sink write failed") }
func (errorSink) Drain(context.Context) error { return nil }
func (errorSink) SanctionLabels(Labels)       {}
