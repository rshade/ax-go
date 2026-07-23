package ax

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rshade/ax-go/logging"
)

// These tests cover the log-shipping guarantees across the generic sink seam that
// replaced the two *lokiWriter type assertions in logger.go (FR-010).
//
// They live in a new file rather than in loki_test.go on purpose: keeping
// loki_test.go's diff to the mechanical sink-access change is what makes SC-005
// ("the log-shipping suite passes with no substantive modification") reviewable
// at a glance. New capability assertions belong beside it, not inside it.
//
// Importing the public logging package from a root-package test is safe and
// creates no cycle: logging imports internal/logcore, which never imports root ax.

// lokiCapture is a fake Loki endpoint that records every push body.
type lokiCapture struct {
	mu     sync.Mutex
	bodies [][]byte
	server *httptest.Server
}

func newLokiCapture(t *testing.T) *lokiCapture {
	t.Helper()

	c := &lokiCapture{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/push" {
			body, _ := io.ReadAll(r.Body)
			c.mu.Lock()
			c.bodies = append(c.bodies, body)
			c.mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(c.server.Close)
	t.Setenv("AX_LOKI_URL", c.server.URL)
	return c
}

func (c *lokiCapture) captured() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.bodies...)
}

// flushAndCollect drains the logger and returns the stream label maps that
// reached the fake endpoint.
func flushAndCollect(t *testing.T, c *lokiCapture, l Logger) []map[string]string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Flush(ctx, l); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return collectPushStreamLabels(t, c.captured())
}

// TestLokiPromotionIsOptionOrderIndependentAcrossSurfaces covers FR-009
// acceptance 3 through BOTH public surfaces.
//
// The regression it guards against is subtle and silent. WithLokiFromEnv captures
// the ADDRESS of the config's Writer field (&cfg.Writer) and registers its sink
// during option application, while label sanctioning happens after every option
// has run. If the extraction had copied the config by value anywhere, or moved
// sanctioning earlier, promotion would start depending on where WithLokiFromEnv
// sat in the option list — and the only symptom would be labels quietly missing
// from Loki streams, with no error and no failing unit test elsewhere.
//
// The logging.NewLogger half additionally proves the alias chain end to end: a
// root-manufactured option, driving root-owned machinery, invoked through the
// isolated constructor.
func TestLokiPromotionIsOptionOrderIndependentAcrossSurfaces(t *testing.T) {
	want := Labels{Environment: "prod", Application: "seam-order", Version: "v1.2.3"}

	cases := []struct {
		name  string
		build func(w io.Writer) Logger
	}{
		{
			name: "root/loki_before_labels",
			build: func(w io.Writer) Logger {
				return NewLogger(context.Background(),
					WithLoggerWriter(w), WithLokiFromEnv(), WithLoggerLabels(want))
			},
		},
		{
			name: "root/loki_after_labels",
			build: func(w io.Writer) Logger {
				return NewLogger(context.Background(),
					WithLoggerWriter(w), WithLoggerLabels(want), WithLokiFromEnv())
			},
		},
		{
			name: "root/labels_before_writer_and_loki",
			build: func(w io.Writer) Logger {
				return NewLogger(context.Background(),
					WithLoggerLabels(want), WithLokiFromEnv(), WithLoggerWriter(w))
			},
		},
		{
			name: "isolated/loki_before_labels",
			build: func(w io.Writer) Logger {
				return logging.NewLogger(context.Background(),
					logging.WithLoggerWriter(w), WithLokiFromEnv(), logging.WithLoggerLabels(want))
			},
		},
		{
			name: "isolated/loki_after_labels",
			build: func(w io.Writer) Logger {
				return logging.NewLogger(context.Background(),
					logging.WithLoggerWriter(w), logging.WithLoggerLabels(want), WithLokiFromEnv())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := newLokiCapture(t)

			logger := tc.build(io.Discard)
			logger.Info(context.Background()).Msg("seam order")

			streams := flushAndCollect(t, capture, logger)
			if !hasStreamLabels(streams, want) {
				t.Fatalf("stream labels did not include %+v; got %#v", want, streams)
			}
		})
	}
}

// TestLokiCardinalitySplitSurvivesTheSanctionerIndirection covers FR-009
// acceptance 2 and Constitution Principle VIII after the concrete *lokiWriter
// assertion became a LabelSanctioner interface assertion.
//
// The specific hazard: promotion is by exact (key, value) PAIR, not by key. A
// payload field that reuses a label key name with a different value — the classic
// .Str("host", req.Host) on a per-request basis — must stay payload-only.
// Promoting it would turn an unbounded value into a Loki stream label and blow up
// index cardinality, which is a production incident, not a test failure.
func TestLokiCardinalitySplitSurvivesTheSanctionerIndirection(t *testing.T) {
	capture := newLokiCapture(t)

	declared := Labels{Environment: "prod", Application: "cardinality", Host: "declared-host"}
	logger := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
		WithLoggerLabels(declared),
	)

	// A payload field reusing a sanctioned KEY with an unsanctioned VALUE.
	logger.Info(context.Background()).
		Str("host", "per-request-host-that-must-not-be-promoted").
		Str("trace_id", "ffffffffffffffffffffffffffffffff").
		Str("user_id", "user-42").
		Msg("cardinality split")

	streams := flushAndCollect(t, capture, logger)
	if len(streams) == 0 {
		t.Fatal("no push streams captured")
	}

	for _, stream := range streams {
		// Only the five sanctioned keys may ever appear.
		for key := range stream {
			switch key {
			case lokiLabelEnvironment, lokiLabelApplication, lokiLabelHost, lokiLabelVersion, lokiLabelLevel:
			default:
				t.Errorf("stream carries non-sanctioned label key %q: %#v", key, stream)
			}
		}
		// High-cardinality payload must never be promoted, by key or by value.
		for _, forbidden := range []string{"trace_id", "span_id", "user_id"} {
			if _, ok := stream[forbidden]; ok {
				t.Errorf("high-cardinality field %q was promoted to a stream label: %#v", forbidden, stream)
			}
		}
		// The reused key must carry the DECLARED value or be absent — never the
		// per-request one.
		if got, ok := stream[lokiLabelHost]; ok && got != declared.Host {
			t.Errorf("host promoted with unsanctioned value %q, want %q or absent", got, declared.Host)
		}
	}
}

// TestLokiDrainSemanticsUnchanged covers FR-009 acceptance 4: drain delivers
// within the documented deadline, Flush on a nil logger is safe, and a drain
// failure never becomes an error the caller must handle or an exit-code change.
func TestLokiDrainSemanticsUnchanged(t *testing.T) {
	t.Run("buffered_entries_delivered_within_deadline", func(t *testing.T) {
		capture := newLokiCapture(t)

		logger := NewLogger(context.Background(), WithLoggerWriter(io.Discard), WithLokiFromEnv())
		logger.Info(context.Background()).Msg("drain me")

		start := time.Now()
		streams := flushAndCollect(t, capture, logger)
		elapsed := time.Since(start)

		if len(streams) == 0 {
			t.Fatal("Flush did not deliver the buffered entry")
		}
		// lokiFlushTimeout caps the wait; allow generous slack for a loaded CI box
		// while still failing if the deadline stopped being enforced at all.
		if elapsed > 4*lokiFlushTimeout {
			t.Errorf("Flush took %v, well beyond the %v drain deadline", elapsed, lokiFlushTimeout)
		}
	})

	t.Run("nil_logger_returns_nil", func(t *testing.T) {
		if err := Flush(context.Background(), nil); err != nil {
			t.Fatalf("Flush(ctx, nil) = %v, want nil", err)
		}
	})

	t.Run("unreachable_endpoint_fails_open", func(t *testing.T) {
		// A closed port: the push cannot succeed. Drain must still return nil, so a
		// log-shipping outage can never change a CLI's exit code.
		t.Setenv("AX_LOKI_URL", "http://127.0.0.1:1")

		var diagnostics writerRecorder
		logger := NewLogger(context.Background(), WithLoggerWriter(&diagnostics), WithLokiFromEnv())
		logger.Info(context.Background()).Msg("unreachable")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := Flush(ctx, logger); err != nil {
			t.Fatalf("Flush = %v, want nil (fail-open)", err)
		}
	})
}

// TestDerivedLoggerCarriesLokiSinkForward covers C-08 end to end (US3's edge
// case): a logger derived through WithLabels keeps its Loki sink, so a later
// Flush still drains it, AND the derived labels are re-sanctioned so they are
// promoted.
//
// Both halves matter. Dropping the sink makes Flush a silent no-op on the derived
// logger — buffered lines vanish with no error. Failing to re-sanction leaves the
// lines shipped but unlabelled, so they land in the wrong stream.
func TestDerivedLoggerCarriesLokiSinkForward(t *testing.T) {
	capture := newLokiCapture(t)

	base := NewLogger(context.Background(),
		WithLoggerWriter(io.Discard),
		WithLokiFromEnv(),
		WithLoggerLabels(Labels{Environment: "prod", Application: "base"}),
	)

	derivedLabels := Labels{Environment: "prod", Application: "derived", Version: "v2"}
	derived := base.WithLabels(derivedLabels)

	derived.Info(context.Background()).Msg("derived through the seam")

	// Flush the DERIVED logger: this is what proves the sink was carried forward.
	streams := flushAndCollect(t, capture, derived)
	if len(streams) == 0 {
		t.Fatal("derived logger delivered nothing; its Loki sink was not carried forward")
	}
	if !hasStreamLabels(streams, derivedLabels) {
		t.Fatalf("derived labels were not re-sanctioned for promotion; got %#v", streams)
	}
}

// writerRecorder is a minimal io.Writer for diagnostics that a test only needs to
// not panic on.
type writerRecorder struct {
	mu   sync.Mutex
	data []byte
}

func (w *writerRecorder) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	return len(p), nil
}
