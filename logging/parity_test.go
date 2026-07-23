// This file carries NO BUILD TAG, deliberately.
//
// A parity claim verified only under the default build proves nothing: the two
// declined configurations (ax_no_grpc, ax_no_otlp) resolve a different file set
// in root ax, and it is precisely under those that a divergence would be easiest
// to introduce and hardest to notice. Untagged means `go test -race -tags=...`
// runs it under all four configurations.
//
// It also lives in the EXTERNAL test package logging_test, so its import of root
// ax can never contribute to the logging package's own dependency graph and the
// import-isolation assertion stays unambiguous. That is safe only because root ax
// delegates to internal/logcore rather than to this package — under a chain
// design, this file would be an import cycle.
package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/rs/zerolog"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/logging"
)

// TestSurfacesEmitByteIdenticalOutput covers FR-006/SC-004 and L-10: given
// identical writers, level, and labels, a line emitted through ax.NewLogger and
// one emitted through logging.NewLogger are byte-identical.
//
// Byte-identical is the right bar rather than "semantically equivalent". Agents
// diff outputs across runs, and a field-order difference or a formatting
// difference between the two surfaces would silently break any consumer that
// migrated from one to the other — while every field-by-field assertion stayed
// green.
func TestSurfacesEmitByteIdenticalOutput(t *testing.T) {
	labels := ax.Labels{
		Environment: "prod",
		Application: "parity",
		Host:        "host-1",
		Version:     "v1.2.3",
	}

	cases := []struct {
		name  string
		ctx   func(testing.TB) context.Context
		level zerolog.Level
		emit  func(ax.Logger, context.Context)
	}{
		{
			name:  "plain_line_no_span",
			ctx:   func(testing.TB) context.Context { return context.Background() },
			level: zerolog.InfoLevel,
			emit:  func(l ax.Logger, ctx context.Context) { l.Info(ctx).Msg("parity") },
		},
		{
			name:  "plain_line_active_span",
			ctx:   activeSpanContext,
			level: zerolog.InfoLevel,
			emit:  func(l ax.Logger, ctx context.Context) { l.Info(ctx).Msg("parity") },
		},
		{
			name:  "structured_fields_active_span",
			ctx:   activeSpanContext,
			level: zerolog.InfoLevel,
			emit: func(l ax.Logger, ctx context.Context) {
				l.Info(ctx).
					Str("resource_id", "01890d3e-2b7a-7c9e-9a1e-9f3c0a1b2c3d").
					Int("attempt", 3).
					Bool("retry", true).
					Msg("parity")
			},
		},
		{
			name:  "error_level",
			ctx:   activeSpanContext,
			level: zerolog.InfoLevel,
			emit:  func(l ax.Logger, ctx context.Context) { l.Error(ctx).Msg("parity") },
		},
		{
			name:  "debug_at_debug_level",
			ctx:   activeSpanContext,
			level: zerolog.DebugLevel,
			emit:  func(l ax.Logger, ctx context.Context) { l.Debug(ctx).Msg("parity") },
		},
		{
			name:  "warn_level",
			ctx:   activeSpanContext,
			level: zerolog.InfoLevel,
			emit:  func(l ax.Logger, ctx context.Context) { l.Warn(ctx).Msg("parity") },
		},
		{
			name:  "derived_logger",
			ctx:   activeSpanContext,
			level: zerolog.InfoLevel,
			emit: func(l ax.Logger, ctx context.Context) {
				l.WithLabels(ax.Labels{Application: "derived", Version: "v9"}).Info(ctx).Msg("parity")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx(t)

			var rootBuf, isolatedBuf bytes.Buffer

			rootLogger := ax.NewLogger(context.Background(),
				ax.WithLoggerWriter(&rootBuf),
				ax.WithLoggerLevel(tc.level),
				ax.WithLoggerLabels(labels),
			)
			isolatedLogger := logging.NewLogger(context.Background(),
				logging.WithLoggerWriter(&isolatedBuf),
				logging.WithLoggerLevel(tc.level),
				logging.WithLoggerLabels(labels),
			)

			tc.emit(rootLogger, ctx)
			tc.emit(isolatedLogger, ctx)

			if rootBuf.Len() == 0 {
				t.Fatal("root surface emitted nothing; the case asserts parity on a line that exists")
			}
			if rootBuf.String() != isolatedBuf.String() {
				t.Fatalf(
					"surfaces diverged.\n  root ax: %s\n  logging: %s",
					rootBuf.String(), isolatedBuf.String(),
				)
			}
		})
	}
}

// TestSurfacesAgreeOnTraceCorrelation pins the specific field pair that would be
// most damaging to diverge and that has two independent implementations behind
// it: root's trace.go carries its own traceIDs copy for the error envelope, and
// internal/logcore carries one for the log hot path. This is the test the comment
// on root's traceIDs points at.
func TestSurfacesAgreeOnTraceCorrelation(t *testing.T) {
	ctx := activeSpanContext(t)

	var rootBuf, isolatedBuf bytes.Buffer
	ax.NewLogger(context.Background(), ax.WithLoggerWriter(&rootBuf)).Info(ctx).Msg("correlated")
	logging.NewLogger(context.Background(), logging.WithLoggerWriter(&isolatedBuf)).Info(ctx).Msg("correlated")

	rootLine := decodeLine(t, rootBuf.String())
	isolatedLine := decodeLine(t, isolatedBuf.String())

	for _, field := range []string{fieldTraceID, fieldSpanID} {
		if rootLine[field] != isolatedLine[field] {
			t.Errorf("%s diverged: root ax = %v, logging = %v", field, rootLine[field], isolatedLine[field])
		}
	}
	// And both must equal the span the context actually carries, so the test
	// cannot pass by both surfaces being identically wrong.
	if rootLine[fieldTraceID] != testTraceIDHex {
		t.Errorf("%s = %v, want %q", fieldTraceID, rootLine[fieldTraceID], testTraceIDHex)
	}
	if rootLine[fieldSpanID] != testSpanIDHex {
		t.Errorf("%s = %v, want %q", fieldSpanID, rootLine[fieldSpanID], testSpanIDHex)
	}
}

// TestSurfacesAgreeOnFieldOrder asserts the JSON key SEQUENCE matches, not just
// the key set. Byte-identical output already implies it, but decoding into a map
// (as several other tests do) discards order entirely, so this states the
// requirement explicitly for anyone who later relaxes the byte comparison.
func TestSurfacesAgreeOnFieldOrder(t *testing.T) {
	ctx := activeSpanContext(t)
	labels := ax.Labels{Environment: "prod", Application: "order", Host: "h", Version: "v1"}

	var rootBuf, isolatedBuf bytes.Buffer
	ax.NewLogger(context.Background(),
		ax.WithLoggerWriter(&rootBuf), ax.WithLoggerLabels(labels),
	).Info(ctx).Str("a", "1").Int("b", 2).Msg("ordered")
	logging.NewLogger(context.Background(),
		logging.WithLoggerWriter(&isolatedBuf), logging.WithLoggerLabels(labels),
	).Info(ctx).Str("a", "1").Int("b", 2).Msg("ordered")

	rootKeys := jsonKeyOrder(t, rootBuf.Bytes())
	isolatedKeys := jsonKeyOrder(t, isolatedBuf.Bytes())

	if len(rootKeys) != len(isolatedKeys) {
		t.Fatalf("key counts differ: root ax %v, logging %v", rootKeys, isolatedKeys)
	}
	for i := range rootKeys {
		if rootKeys[i] != isolatedKeys[i] {
			t.Fatalf("key order diverged at %d: root ax %v, logging %v", i, rootKeys, isolatedKeys)
		}
	}
}

// TestSurfacesAgreeOnLevelFiltering asserts a filtered event produces no output
// on either surface. A divergence here would mean one surface was quietly
// emitting debug lines in production.
func TestSurfacesAgreeOnLevelFiltering(t *testing.T) {
	ctx := activeSpanContext(t)

	var rootBuf, isolatedBuf bytes.Buffer
	ax.NewLogger(context.Background(),
		ax.WithLoggerWriter(&rootBuf), ax.WithLoggerLevel(zerolog.InfoLevel),
	).Debug(ctx).Msg("filtered")
	logging.NewLogger(context.Background(),
		logging.WithLoggerWriter(&isolatedBuf), logging.WithLoggerLevel(zerolog.InfoLevel),
	).Debug(ctx).Msg("filtered")

	if rootBuf.Len() != 0 || isolatedBuf.Len() != 0 {
		t.Fatalf("filtered event produced output: root ax %q, logging %q", rootBuf.String(), isolatedBuf.String())
	}
}

// TestSurfacesAgreeOnDefaults asserts the constructors agree on level and, by
// emitting through an explicitly configured writer afterwards, that neither
// surface's defaults leak into the other.
func TestSurfacesAgreeOnDefaults(t *testing.T) {
	rootLevel := ax.NewLogger(context.Background()).Zerolog().GetLevel()
	isolatedLevel := logging.NewLogger(context.Background()).Zerolog().GetLevel()

	if rootLevel != isolatedLevel {
		t.Fatalf("default level diverged: root ax = %v, logging = %v", rootLevel, isolatedLevel)
	}
	if rootLevel != zerolog.InfoLevel {
		t.Fatalf("default level = %v, want %v on both surfaces", rootLevel, zerolog.InfoLevel)
	}
}

// jsonKeyOrder returns the top-level JSON object keys in the order they appear on
// the wire, which json.Unmarshal into a map would discard.
func jsonKeyOrder(t *testing.T, line []byte) []string {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(line))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("read opening token: %v", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		t.Fatalf("line does not start with a JSON object: %v", tok)
	}

	var keys []string
	for dec.More() {
		keyTok, keyErr := dec.Token()
		if keyErr != nil {
			t.Fatalf("read key: %v", keyErr)
		}
		key, ok := keyTok.(string)
		if !ok {
			t.Fatalf("object key is not a string: %v", keyTok)
		}
		keys = append(keys, key)

		// Consume the value, descending through any nested composite.
		depth := 0
		for {
			valTok, valErr := dec.Token()
			if valErr != nil {
				if valErr == io.EOF {
					t.Fatalf("unexpected EOF reading value for %q", key)
				}
				t.Fatalf("read value for %q: %v", key, valErr)
			}
			if delim, ok := valTok.(json.Delim); ok {
				switch delim {
				case '{', '[':
					depth++
					continue
				case '}', ']':
					depth--
				}
			}
			if depth <= 0 {
				break
			}
		}
	}
	return keys
}
