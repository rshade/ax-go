package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestParseConfigAcceptsHujson(t *testing.T) {
	input := `{
		// comments are allowed
		"name": "ax",
		"ports": [8080, 9090,],
	}`
	var got struct {
		Name  string `json:"name"`
		Ports []int  `json:"ports"`
	}

	if err := ParseConfig(context.Background(), strings.NewReader(input), &got); err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if got.Name != "ax" {
		t.Fatalf("Name = %q, want ax", got.Name)
	}
	if len(got.Ports) != 2 || got.Ports[0] != 8080 || got.Ports[1] != 9090 {
		t.Fatalf("Ports = %#v, want [8080 9090]", got.Ports)
	}
}

func TestParseConfigRejectsOversizedInput(t *testing.T) {
	input := strings.NewReader(strings.Repeat(" ", 1<<20) + "{}")
	var got struct{}

	err := ParseConfig(context.Background(), input, &got)
	if err == nil {
		t.Fatal("ParseConfig returned nil error for oversized input")
	}

	var axErr *Error
	if !errors.As(err, &axErr) {
		t.Fatalf("ParseConfig error type = %T, want *Error", err)
	}
	if code := ErrorExitCode(err); code != ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", code, ExitValidation)
	}
}

func TestParseConfigAcceptsInputAtDefaultLimit(t *testing.T) {
	input := strings.NewReader(strings.Repeat(" ", 1<<20-2) + "{}")
	var got struct{}

	if err := ParseConfig(context.Background(), input, &got); err != nil {
		t.Fatalf("ParseConfig returned error for input at default limit: %v", err)
	}
}

func TestParseConfigHonorsMaxConfigBytesOption(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		max     int64
		wantErr bool
	}{
		{
			name:    "custom cap rejects input over limit",
			input:   "{}",
			max:     1,
			wantErr: true,
		},
		{
			name:    "custom cap above default accepts larger input",
			input:   strings.Repeat(" ", 1<<20) + "{}",
			max:     1<<20 + 2,
			wantErr: false,
		},
		{
			name:    "negative cap is validation error",
			input:   "{}",
			max:     -1,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got struct{}

			err := ParseConfig(
				context.Background(),
				strings.NewReader(tc.input),
				&got,
				WithMaxConfigBytes(tc.max),
			)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("ParseConfig returned error: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("ParseConfig returned nil error")
			}
			var axErr *Error
			if !errors.As(err, &axErr) {
				t.Fatalf("ParseConfig error type = %T, want *Error", err)
			}
			if code := ErrorExitCode(err); code != ExitValidation {
				t.Fatalf("ErrorExitCode = %d, want %d", code, ExitValidation)
			}
		})
	}
}

func TestParseConfigRejectsNilOption(t *testing.T) {
	var got struct{}

	err := ParseConfig(context.Background(), strings.NewReader("{}"), &got, nil)
	assertConfigError(t, err, "config_option_invalid")
}

func TestParseConfigClassifiesInvalidConfigAsValidation(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		dst        any
		checkChain func(t *testing.T, err error)
	}{
		{
			name:  "invalid Hujson",
			input: "{",
			dst:   &struct{}{},
			checkChain: func(t *testing.T, err error) {
				t.Helper()
				var axErr *Error
				if !errors.As(err, &axErr) {
					t.Fatalf("error type = %T, want *Error", err)
				}
				if errors.Unwrap(axErr) == nil {
					t.Fatal("config_invalid error has no cause; want the parser error preserved in the chain")
				}
			},
		},
		{
			name:  "schema mismatch",
			input: `{"count":"not an integer"}`,
			dst: &struct {
				Count int `json:"count"`
			}{},
			checkChain: func(t *testing.T, err error) {
				t.Helper()
				var typeErr *json.UnmarshalTypeError
				if !errors.As(err, &typeErr) {
					t.Fatal("errors.As(*json.UnmarshalTypeError) = false, want the decode error preserved in the chain")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ParseConfig(context.Background(), strings.NewReader(tc.input), tc.dst)
			axErr := assertConfigError(t, err, "config_invalid")
			if axErr.ActionableFix == "" {
				t.Fatal("ActionableFix is empty")
			}
			tc.checkChain(t, err)
		})
	}
}

func TestParseConfigLeavesInvalidDestinationAsProgrammerError(t *testing.T) {
	cases := []struct {
		name string
		dst  any
	}{
		{
			name: "nil destination",
			dst:  nil,
		},
		{
			name: "non-pointer destination",
			dst: struct {
				Name string `json:"name"`
			}{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ParseConfig(context.Background(), strings.NewReader(`{"name":"ax"}`), tc.dst)
			if err == nil {
				t.Fatal("ParseConfig returned nil error")
			}
			assertNotAxError(t, err)

			var invalidUnmarshal *json.InvalidUnmarshalError
			if !errors.As(err, &invalidUnmarshal) {
				t.Fatalf("errors.As(*json.InvalidUnmarshalError) = false for %T", err)
			}
			if got := ErrorExitCode(err); got != ExitInternal {
				t.Fatalf("ErrorExitCode = %d, want %d", got, ExitInternal)
			}
		})
	}
}

func TestParseConfigFileHonorsMaxConfigBytesOption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.hujson")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var got struct{}
	err := ParseConfigFile(context.Background(), path, &got, WithMaxConfigBytes(1))
	if err == nil {
		t.Fatal("ParseConfigFile returned nil error")
	}

	var axErr *Error
	if !errors.As(err, &axErr) {
		t.Fatalf("ParseConfigFile error type = %T, want *Error", err)
	}
	if code := ErrorExitCode(err); code != ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", code, ExitValidation)
	}
}

func TestParseConfigEnforcesLimitAtReadBoundary(t *testing.T) {
	const capBytes int64 = 64

	t.Run("tripwire rejects 100x input at cap plus one", func(t *testing.T) {
		reader := newCountingTripwireReader(t, strings.NewReader(strings.Repeat(" ", int(capBytes*100))), capBytes+1)
		var got struct {
			Name string `json:"name"`
		}

		err := ParseConfig(context.Background(), reader, &got, WithMaxConfigBytes(capBytes))
		assertConfigError(t, err, "config_too_large")
		if got.Name != "" {
			t.Fatalf("ParseConfig mutated dst after oversize rejection: %#v", got)
		}
		if reader.read > capBytes+1 {
			t.Fatalf("ParseConfig read %d bytes, want <= %d", reader.read, capBytes+1)
		}
	})

	cases := []struct {
		name      string
		input     string
		maxBytes  int64
		wantCode  string
		wantError bool
	}{
		{
			name:     "exactly at cap accepted",
			input:    strings.Repeat(" ", int(capBytes)-2) + "{}",
			maxBytes: capBytes,
		},
		{
			name:      "one byte over cap rejected",
			input:     strings.Repeat(" ", int(capBytes)+1),
			maxBytes:  capBytes,
			wantCode:  "config_too_large",
			wantError: true,
		},
		{
			name:      "above ceiling rejected",
			input:     "{}",
			maxBytes:  MaxConfigBytesCeiling + 1,
			wantCode:  "config_max_bytes_invalid",
			wantError: true,
		},
		{
			name:      "math max rejected",
			input:     "{}",
			maxBytes:  math.MaxInt64,
			wantCode:  "config_max_bytes_invalid",
			wantError: true,
		},
		{
			name:     "ceiling accepted",
			input:    "{}",
			maxBytes: MaxConfigBytesCeiling,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got struct {
				Name string `json:"name"`
			}

			err := ParseConfig(
				context.Background(),
				strings.NewReader(tc.input),
				&got,
				WithMaxConfigBytes(tc.maxBytes),
			)
			if !tc.wantError {
				if err != nil {
					t.Fatalf("ParseConfig returned error: %v", err)
				}
				return
			}

			assertConfigError(t, err, tc.wantCode)
			if got.Name != "" {
				t.Fatalf("ParseConfig mutated dst after rejection: %#v", got)
			}
		})
	}

	t.Run("default cap rejects stream larger than one mebibyte", func(t *testing.T) {
		var got struct {
			Name string `json:"name"`
		}

		err := ParseConfig(
			context.Background(),
			strings.NewReader(strings.Repeat(" ", int(DefaultMaxConfigBytes)+1)),
			&got,
		)
		assertConfigError(t, err, "config_too_large")
		if got.Name != "" {
			t.Fatalf("ParseConfig mutated dst after default-cap rejection: %#v", got)
		}
	})
}

func TestParseConfigMaxBytesOptionContract(t *testing.T) {
	t.Run("raising cap accepts input the default rejects", func(t *testing.T) {
		input := strings.Repeat(" ", int(DefaultMaxConfigBytes)) + "{}"

		var rejected struct{}
		err := ParseConfig(context.Background(), strings.NewReader(input), &rejected)
		assertConfigError(t, err, "config_too_large")

		var accepted struct{}
		if err := ParseConfig(
			context.Background(),
			strings.NewReader(input),
			&accepted,
			WithMaxConfigBytes(DefaultMaxConfigBytes+2),
		); err != nil {
			t.Fatalf("ParseConfig returned error after raising cap: %v", err)
		}
	})

	t.Run("lowering cap rejects input", func(t *testing.T) {
		var got struct{}

		err := ParseConfig(context.Background(), strings.NewReader("{}"), &got, WithMaxConfigBytes(1))
		assertConfigError(t, err, "config_too_large")
	})

	t.Run("sequential calls have no residual cap state", func(t *testing.T) {
		var first struct{}
		err := ParseConfig(context.Background(), strings.NewReader("{}"), &first, WithMaxConfigBytes(1))
		assertConfigError(t, err, "config_too_large")

		var second struct{}
		if err := ParseConfig(
			context.Background(),
			strings.NewReader("{}"),
			&second,
			WithMaxConfigBytes(2),
		); err != nil {
			t.Fatalf("ParseConfig returned error with raised cap: %v", err)
		}

		var third struct{}
		err = ParseConfig(context.Background(), strings.NewReader("{}"), &third, WithMaxConfigBytes(1))
		assertConfigError(t, err, "config_too_large")
	})

	t.Run("zero cap passes empty input through size check only", func(t *testing.T) {
		var got struct{}

		err := ParseConfig(context.Background(), strings.NewReader(""), &got, WithMaxConfigBytes(0))
		if err == nil {
			t.Fatal("ParseConfig returned nil error for empty Hujson")
		}
		assertNotConfigTooLarge(t, err)
	})

	t.Run("zero cap rejects non-empty input as oversize", func(t *testing.T) {
		var got struct{}

		err := ParseConfig(context.Background(), strings.NewReader("{}"), &got, WithMaxConfigBytes(0))
		assertConfigError(t, err, "config_too_large")
	})

	t.Run("out-of-range caps are validation errors", func(t *testing.T) {
		for _, maxBytes := range []int64{-1, MaxConfigBytesCeiling + 1, math.MaxInt64} {
			var got struct{}

			err := ParseConfig(
				context.Background(),
				strings.NewReader("{}"),
				&got,
				WithMaxConfigBytes(maxBytes),
			)
			assertConfigError(t, err, "config_max_bytes_invalid")
		}
	})

	t.Run("ceiling cap accepts normal input", func(t *testing.T) {
		var got struct{}

		if err := ParseConfig(
			context.Background(),
			strings.NewReader("{}"),
			&got,
			WithMaxConfigBytes(MaxConfigBytesCeiling),
		); err != nil {
			t.Fatalf("ParseConfig returned error at ceiling cap: %v", err)
		}
	})

	t.Run("file path default and override use the same cap contract", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.hujson")
		input := strings.Repeat(" ", int(DefaultMaxConfigBytes)) + "{}"
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		var rejected struct{}
		err := ParseConfigFile(context.Background(), path, &rejected)
		assertConfigError(t, err, "config_too_large")

		var accepted struct{}
		if err := ParseConfigFile(
			context.Background(),
			path,
			&accepted,
			WithMaxConfigBytes(DefaultMaxConfigBytes+2),
		); err != nil {
			t.Fatalf("ParseConfigFile returned error after raising cap: %v", err)
		}
	})
}

func TestParseConfigErrorGoldens(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		goldenPath  string
		wantCode    string
		hasMaxBytes bool
		wantMax     int64
	}{
		{
			name: "oversize",
			err: func() error {
				var got struct{}
				return ParseConfig(context.Background(), strings.NewReader("{}"), &got, WithMaxConfigBytes(1))
			}(),
			goldenPath:  "testdata/config_too_large.golden.json",
			wantCode:    "config_too_large",
			hasMaxBytes: true,
			wantMax:     1,
		},
		{
			name: "invalid max bytes",
			err: func() error {
				var got struct{}
				return ParseConfig(context.Background(), strings.NewReader("{}"), &got, WithMaxConfigBytes(-1))
			}(),
			goldenPath:  "testdata/config_max_bytes_invalid.golden.json",
			wantCode:    "config_max_bytes_invalid",
			hasMaxBytes: true,
			wantMax:     -1,
		},
		{
			name: "invalid config",
			err: func() error {
				var got struct{}
				return ParseConfig(context.Background(), strings.NewReader("{"), &got)
			}(),
			goldenPath: "testdata/config_invalid.golden.json",
			wantCode:   "config_invalid",
		},
		{
			name: "nil option",
			err: func() error {
				var got struct{}
				return ParseConfig(context.Background(), strings.NewReader("{}"), &got, nil)
			}(),
			goldenPath: "testdata/config_option_invalid.golden.json",
			wantCode:   "config_option_invalid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var axErr *Error
			if !errors.As(tc.err, &axErr) {
				t.Fatalf("ParseConfig error type = %T, want *Error", tc.err)
			}
			if axErr.ErrorCode != tc.wantCode {
				t.Fatalf("ErrorCode = %q, want %q", axErr.ErrorCode, tc.wantCode)
			}
			if axErr.ExitCode() != ExitValidation {
				t.Fatalf("ExitCode = %d, want %d", axErr.ExitCode(), ExitValidation)
			}
			if axErr.SchemaVersion != ErrorSchemaVersion {
				t.Fatalf("SchemaVersion = %q, want %q", axErr.SchemaVersion, ErrorSchemaVersion)
			}
			if axErr.ActionableFix == "" {
				t.Fatal("ActionableFix is empty")
			}
			if tc.hasMaxBytes {
				if axErr.Context == nil {
					t.Fatal("Context is nil, want max_bytes field")
				}
				if got := axErr.Context["max_bytes"]; got != tc.wantMax {
					t.Fatalf("Context[max_bytes] = %v, want %d", got, tc.wantMax)
				}
			} else if axErr.Context != nil {
				t.Fatalf("Context = %v, want none", axErr.Context)
			}

			var stderr bytes.Buffer
			if err := WriteError(&stderr, axErr); err != nil {
				t.Fatalf("WriteError returned error: %v", err)
			}
			assertGolden(t, tc.goldenPath, stderr.Bytes())
		})
	}
}

func TestParseConfigErrorUsesCallerTraceContext(t *testing.T) {
	const traceIDHex = "4bf92f3577b34da6a3ce929d0e0e4736"
	const spanIDHex = "00f067aa0ba902b7"

	traceID, err := oteltrace.TraceIDFromHex(traceIDHex)
	if err != nil {
		t.Fatalf("TraceIDFromHex returned error: %v", err)
	}
	spanID, err := oteltrace.SpanIDFromHex(spanIDHex)
	if err != nil {
		t.Fatalf("SpanIDFromHex returned error: %v", err)
	}
	ctx := oteltrace.ContextWithSpanContext(context.Background(), oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	}))

	var got struct{}
	err = ParseConfig(ctx, strings.NewReader("{}"), &got, WithMaxConfigBytes(1))
	axErr := assertConfigError(t, err, "config_too_large")
	if axErr.TraceID != traceIDHex {
		t.Fatalf("TraceID = %q, want %q", axErr.TraceID, traceIDHex)
	}
}

func TestParseConfigDeterminismSourceErrorsAndCancelation(t *testing.T) {
	t.Run("repeated oversize parse has deterministic classification", func(t *testing.T) {
		err1 := parseConfigError(strings.Repeat(" ", 3), WithMaxConfigBytes(1))
		err2 := parseConfigError(strings.Repeat(" ", 3), WithMaxConfigBytes(1))

		axErr1 := assertConfigError(t, err1, "config_too_large")
		axErr2 := assertConfigError(t, err2, "config_too_large")
		if axErr1.ErrorCode != axErr2.ErrorCode {
			t.Fatalf("ErrorCode changed between runs: %q vs %q", axErr1.ErrorCode, axErr2.ErrorCode)
		}
	})

	t.Run("source error before cap plus one preserves chain", func(t *testing.T) {
		sourceErr := errors.New("broken config stream")
		reader := &erroringReader{
			chunk: []byte(`{`),
			err:   sourceErr,
		}

		err := ParseConfig(context.Background(), reader, &struct{}{}, WithMaxConfigBytes(8))
		if !errors.Is(err, sourceErr) {
			t.Fatalf("ParseConfig error = %v, want source error in chain", err)
		}
		assertNotAxError(t, err)
		assertNotConfigTooLarge(t, err)
	})

	t.Run("oversize wins when read crosses cap with source error", func(t *testing.T) {
		sourceErr := errors.New("late source error")
		reader := &erroringReader{
			chunk: []byte(`abc`),
			err:   sourceErr,
		}

		err := ParseConfig(context.Background(), reader, &struct{}{}, WithMaxConfigBytes(2))
		assertConfigError(t, err, "config_too_large")
		if errors.Is(err, sourceErr) {
			t.Fatalf("ParseConfig preserved source error after oversize classification: %v", err)
		}
	})

	t.Run("source error before oversize is not classified as oversize", func(t *testing.T) {
		sourceErr := errors.New("early source error")
		reader := &erroringReader{
			chunk: []byte(`ab`),
			err:   sourceErr,
		}

		err := ParseConfig(context.Background(), reader, &struct{}{}, WithMaxConfigBytes(2))
		if !errors.Is(err, sourceErr) {
			t.Fatalf("ParseConfig error = %v, want source error in chain", err)
		}
		assertNotAxError(t, err)
		assertNotConfigTooLarge(t, err)
	})

	t.Run("already canceled context reaches both public entry points", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := ParseConfig(ctx, strings.NewReader("{}"), &struct{}{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ParseConfig error = %v, want context.Canceled", err)
		}
		assertNotAxError(t, err)
		assertNotConfigTooLarge(t, err)

		path := filepath.Join(t.TempDir(), "config.hujson")
		if writeErr := os.WriteFile(path, []byte("{}"), 0o600); writeErr != nil {
			t.Fatalf("write config file: %v", writeErr)
		}

		err = ParseConfigFile(ctx, path, &struct{}{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ParseConfigFile error = %v, want context.Canceled", err)
		}
		assertNotAxError(t, err)
		assertNotConfigTooLarge(t, err)
		if got := ErrorExitCode(err); got != ExitInternal {
			t.Fatalf("ErrorExitCode(context.Canceled) = %d, want %d", got, ExitInternal)
		}
	})

	t.Run("deadline between chunks reaches public stream entry point", func(t *testing.T) {
		expired := make(chan struct{})
		ctx := controlledDeadlineContext{expired: expired}

		reader := &publicControlledChunkReader{
			chunks: [][]byte{
				[]byte(`{`),
				[]byte(`}`),
			},
			expireAfterFirstChunk: expired,
		}

		err := ParseConfig(ctx, reader, &struct{}{})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("ParseConfig error = %v, want context.DeadlineExceeded", err)
		}
		assertNotAxError(t, err)
		assertNotConfigTooLarge(t, err)
		if got := ErrorExitCode(err); got != ExitNetwork {
			t.Fatalf("ErrorExitCode(context.DeadlineExceeded) = %d, want %d", got, ExitNetwork)
		}
	})
}

type countingTripwireReader struct {
	t      *testing.T
	source io.Reader
	limit  int64
	read   int64
}

func newCountingTripwireReader(t *testing.T, source io.Reader, limit int64) *countingTripwireReader {
	t.Helper()
	return &countingTripwireReader{t: t, source: source, limit: limit}
}

func (r *countingTripwireReader) Read(p []byte) (int, error) {
	r.t.Helper()

	n, err := r.source.Read(p)
	r.read += int64(n)
	if r.read > r.limit {
		r.t.Fatalf("reader returned %d bytes, want <= %d", r.read, r.limit)
	}
	return n, err
}

func assertConfigError(t *testing.T, err error, wantCode string) *Error {
	t.Helper()

	if err == nil {
		t.Fatal("ParseConfig returned nil error")
	}
	var axErr *Error
	if !errors.As(err, &axErr) {
		t.Fatalf("ParseConfig error type = %T, want *Error", err)
	}
	if axErr.ErrorCode != wantCode {
		t.Fatalf("ErrorCode = %q, want %q", axErr.ErrorCode, wantCode)
	}
	if got := ErrorExitCode(err); got != ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", got, ExitValidation)
	}
	return axErr
}

func assertNotConfigTooLarge(t *testing.T, err error) {
	t.Helper()

	var axErr *Error
	if errors.As(err, &axErr) && axErr.ErrorCode == "config_too_large" {
		t.Fatalf("error code = %q, want non-config_too_large error", axErr.ErrorCode)
	}
}

func assertNotAxError(t *testing.T, err error) {
	t.Helper()

	var axErr *Error
	if errors.As(err, &axErr) {
		t.Fatalf("error type = %T, want non-*Error", err)
	}
}

func parseConfigError(input string, opts ...ParseConfigOption) error {
	var got struct{}
	return ParseConfig(context.Background(), strings.NewReader(input), &got, opts...)
}

type erroringReader struct {
	chunk []byte
	err   error
	done  bool
}

func (r *erroringReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.chunk), r.err
}

func (r *publicControlledChunkReader) Read(p []byte) (int, error) {
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

type publicControlledChunkReader struct {
	chunks                [][]byte
	expireAfterFirstChunk chan struct{}
	index                 int
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
