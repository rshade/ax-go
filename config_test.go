package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestPatchConfigPreservesCommentsAfterPatch(t *testing.T) {
	existing := `{
	// database settings
	"host": "localhost",
	"port": 5432,
}`
	patch := []byte(`[{"op":"replace","path":"/port","value":5433}]`)

	got, err := PatchConfig(context.Background(), strings.NewReader(existing), patch)
	if err != nil {
		t.Fatalf("PatchConfig returned error: %v", err)
	}

	if !strings.Contains(string(got), "// database settings") {
		t.Fatalf("PatchConfig stripped comments; got:\n%s", got)
	}
	if !strings.Contains(string(got), `"host": "localhost"`) {
		t.Fatalf("PatchConfig dropped unchanged field; got:\n%s", got)
	}
	if !strings.Contains(string(got), "5433") {
		t.Fatalf("PatchConfig did not apply replace; got:\n%s", got)
	}
}

func TestPatchConfigReturnsConfigInvalidForBadHujson(t *testing.T) {
	patch := []byte(`[{"op":"add","path":"/x","value":1}]`)
	_, err := PatchConfig(context.Background(), strings.NewReader("{"), patch)
	assertConfigError(t, err, "config_invalid")
}

func TestPatchConfigReturnsConfigPatchInvalidForBadPatch(t *testing.T) {
	cases := []struct {
		name  string
		patch string
	}{
		{
			name:  "patch is not an array",
			patch: `{"op":"add","path":"/x","value":1}`,
		},
		{
			name:  "target path not found",
			patch: `[{"op":"remove","path":"/nonexistent"}]`,
		},
		{
			name:  "missing required op field",
			patch: `[{"path":"/x","value":1}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PatchConfig(
				context.Background(),
				strings.NewReader(`{"a":1}`),
				[]byte(tc.patch),
			)
			assertPatchError(t, err, "config_patch_invalid")
		})
	}
}

func TestPatchConfigHonorsReadCapOption(t *testing.T) {
	existing := strings.Repeat(" ", 10) + `{"a":1}`
	patch := []byte(`[]`)

	_, err := PatchConfig(
		context.Background(),
		strings.NewReader(existing),
		patch,
		WithMaxConfigBytes(5),
	)
	assertConfigError(t, err, "config_too_large")
}

func TestPatchConfigEmptyPatchIsNoOp(t *testing.T) {
	existing := `{
	// preserved comment
	"name": "ax",
}`
	got, err := PatchConfig(context.Background(), strings.NewReader(existing), []byte(`[]`))
	if err != nil {
		t.Fatalf("PatchConfig returned error on no-op patch: %v", err)
	}

	if !strings.Contains(string(got), "// preserved comment") {
		t.Fatalf("PatchConfig stripped comments on no-op; got:\n%s", got)
	}
}

func TestPatchConfigFilePreservesCommentsOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")

	original := []byte(`{
	// production host
	"host": "prod.example.com",
	"port": 443,
}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	patch := []byte(`[{"op":"replace","path":"/port","value":8443}]`)
	if err := PatchConfigFile(context.Background(), path, patch); err != nil {
		t.Fatalf("PatchConfigFile returned error: %v", err)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	if !strings.Contains(string(result), "// production host") {
		t.Fatalf("PatchConfigFile stripped comments; got:\n%s", result)
	}
	if !strings.Contains(string(result), "8443") {
		t.Fatalf("PatchConfigFile did not apply patch; got:\n%s", result)
	}
}

func TestPatchConfigFilePreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")

	const wantMode = 0o640
	if err := os.WriteFile(path, []byte(`{"a":1}`), wantMode); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	patch := []byte(`[{"op":"replace","path":"/a","value":2}]`)
	if err := PatchConfigFile(context.Background(), path, patch); err != nil {
		t.Fatalf("PatchConfigFile returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat patched file: %v", err)
	}
	if got := info.Mode().Perm(); got != wantMode {
		t.Fatalf("file mode = %04o, want %04o", got, wantMode)
	}
}

func TestPatchConfigFileReturnsErrorWhenFileDoesNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.hujson")
	err := PatchConfigFile(context.Background(), path, []byte(`[]`))
	if err == nil {
		t.Fatal("PatchConfigFile returned nil error for nonexistent file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PatchConfigFile error = %v, want os.ErrNotExist in chain", err)
	}
}

func TestPatchConfigFileLeavesOriginalIntactWhenTempCreateFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions are not enforced")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")
	original := []byte(`{"a":1}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	// A read-only directory forces os.CreateTemp to fail inside
	// atomicWriteFile, exercising the no-corruption guarantee.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	patch := []byte(`[{"op":"replace","path":"/a","value":2}]`)
	if err := PatchConfigFile(context.Background(), path, patch); err == nil {
		t.Fatal("PatchConfigFile returned nil error with read-only directory")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original file: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("original file modified after failed write; got:\n%s", got)
	}
}

func TestPatchConfigFileConcurrentPatchesKeepFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")
	if err := os.WriteFile(path, []byte(`{"port":1}`), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	const writers = 8
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			patch := fmt.Appendf(nil, `[{"op":"replace","path":"/port","value":%d}]`, 1000+i)
			errs[i] = PatchConfigFile(context.Background(), path, patch)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: PatchConfigFile returned error: %v", i, err)
		}
	}

	var got struct {
		Port int `json:"port"`
	}
	if err := ParseConfigFile(context.Background(), path, &got); err != nil {
		t.Fatalf("patched file is not valid Hujson: %v", err)
	}
	if got.Port < 1000 || got.Port >= 1000+writers {
		t.Fatalf("port = %d, want last-writer value in [1000, %d)", got.Port, 1000+writers)
	}
}

func TestPatchConfigErrorGolden(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		patch    string
		wantCode string
		golden   string
	}{
		{
			name:     "patch apply failure",
			existing: `{"a":1}`,
			patch:    `[{"op":"remove","path":"/nonexistent"}]`,
			wantCode: "config_patch_invalid",
			golden:   "testdata/config_patch_invalid.golden.json",
		},
		{
			name:     "invalid hujson",
			existing: `{`,
			patch:    `[]`,
			wantCode: "config_invalid",
			golden:   "testdata/config_patch_hujson_invalid.golden.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, gotErr := PatchConfig(
				context.Background(),
				strings.NewReader(tc.existing),
				[]byte(tc.patch),
			)
			assertPatchError(t, gotErr, tc.wantCode)

			var axErr *Error
			if !errors.As(gotErr, &axErr) {
				t.Fatalf("error type = %T, want *Error", gotErr)
			}

			var stderr bytes.Buffer
			if writeErr := WriteError(&stderr, axErr); writeErr != nil {
				t.Fatalf("WriteError returned error: %v", writeErr)
			}
			assertGolden(t, tc.golden, stderr.Bytes())
		})
	}
}

func assertPatchError(t *testing.T, err error, wantCode string) {
	t.Helper()

	if err == nil {
		t.Fatal("PatchConfig returned nil error")
	}
	var axErr *Error
	if !errors.As(err, &axErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if axErr.ErrorCode != wantCode {
		t.Fatalf("ErrorCode = %q, want %q", axErr.ErrorCode, wantCode)
	}
	if got := ErrorExitCode(err); got != ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", got, ExitValidation)
	}
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
