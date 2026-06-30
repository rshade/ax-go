package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rshade/ax-go/contract"
)

func TestErrorRetryable(t *testing.T) {
	cases := []struct {
		name   string
		opts   []ErrorOption
		want   string
		absent bool
	}{
		{name: "explicit true", opts: []ErrorOption{WithRetryable(true)}, want: `"retryable":true`},
		{name: "explicit false", opts: []ErrorOption{WithRetryable(false)}, want: `"retryable":false`},
		{name: "unspecified", opts: nil, absent: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewError(
				context.Background(),
				"network_timeout",
				"upstream timed out",
				tc.opts...)
			var buf bytes.Buffer
			if writeErr := WriteError(&buf, err); writeErr != nil {
				t.Fatalf("WriteError returned error: %v", writeErr)
			}
			got := buf.String()
			if tc.absent {
				if strings.Contains(got, "retryable") {
					t.Fatalf("expected no retryable key, got %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q in %s", tc.want, got)
			}
		})
	}
}

func TestErrorRetryAfterSeconds(t *testing.T) {
	cases := []struct {
		name    string
		seconds int64
		want    string
		absent  bool
	}{
		{name: "positive", seconds: 30, want: `"retry_after_seconds":30`},
		{name: "zero omitted", seconds: 0, absent: true},
		{name: "negative ignored", seconds: -5, absent: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewError(
				context.Background(),
				"network_timeout",
				"upstream timed out",
				WithRetryAfterSeconds(tc.seconds),
			)
			var buf bytes.Buffer
			if writeErr := WriteError(&buf, err); writeErr != nil {
				t.Fatalf("WriteError returned error: %v", writeErr)
			}
			got := buf.String()
			if tc.absent {
				if strings.Contains(got, "retry_after_seconds") {
					t.Fatalf("expected no retry_after_seconds key, got %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("expected %q in %s", tc.want, got)
			}
		})
	}
}

func TestWriteErrorEnvelope(t *testing.T) {
	err := NewError(
		context.Background(),
		"validation_error",
		"bad input",
		WithErrorTool("app"),
		WithErrorVersion("v0.1.0"),
		WithActionableFix("fix the input"),
		WithErrorContext(map[string]any{"field": "name"}),
		WithSuggestions("retry with --help"),
		WithErrorExitCode(ExitValidation),
	)

	var stderr bytes.Buffer
	if writeErr := WriteError(&stderr, err); writeErr != nil {
		t.Fatalf("WriteError returned error: %v", writeErr)
	}
	assertGolden(t, "testdata/error_envelope.golden.json", stderr.Bytes())

	var got map[string]any
	if decodeErr := json.Unmarshal(stderr.Bytes(), &got); decodeErr != nil {
		t.Fatalf("stderr did not contain JSON: %v", decodeErr)
	}

	required := map[string]string{
		"error_code":     "validation_error",
		"message":        "bad input",
		"trace_id":       ZeroTraceID,
		"tool":           "app",
		"version":        "v0.1.0",
		"schema_version": ErrorSchemaVersion,
		"actionable_fix": "fix the input",
	}
	for key, want := range required {
		if got[key] != want {
			t.Fatalf("%s = %v, want %q", key, got[key], want)
		}
	}

	if ErrorExitCode(err) != ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", ErrorExitCode(err), ExitValidation)
	}
}

func TestErrorExitCodeContextErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{
			name: "wrapped deadline exceeded",
			err:  fmt.Errorf("read config: %w", context.DeadlineExceeded),
			want: ExitNetwork,
		},
		{
			name: "wrapped canceled",
			err:  fmt.Errorf("read config: %w", context.Canceled),
			want: ExitInternal,
		},
		{
			name: "plain non context error",
			err:  errors.New("plain failure"),
			want: ExitInternal,
		},
		{
			name: "envelope exit code wins over ctx sentinel in cause chain",
			err: NewError(
				context.Background(),
				"config_invalid",
				"decode failed",
				WithErrorExitCode(ExitValidation),
				WithErrorCause(fmt.Errorf("read config: %w", context.DeadlineExceeded)),
			),
			want: ExitValidation,
		},
		{
			name: "recovery fields do not affect exit code",
			err: NewError(
				context.Background(),
				"network_timeout",
				"upstream timed out",
				WithErrorExitCode(ExitNetwork),
				WithRetryable(true),
				WithRetryAfterSeconds(30),
			),
			want: ExitNetwork,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ErrorExitCode(tc.err); got != tc.want {
				t.Fatalf("ErrorExitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestErrorCauseChain(t *testing.T) {
	sentinel := errors.New("underlying decode failure")

	withCause := NewError(
		context.Background(),
		"config_invalid",
		"decode failed",
		WithErrorCause(sentinel),
	)
	if !errors.Is(withCause, sentinel) {
		t.Fatal("errors.Is(withCause, sentinel) = false, want the cause reachable through Unwrap")
	}

	plain := NewError(context.Background(), "validation_error", "bad input")
	if got := errors.Unwrap(plain); got != nil {
		t.Fatalf("errors.Unwrap(plain) = %v, want nil", got)
	}

	withRecovery := NewError(
		context.Background(),
		"config_invalid",
		"decode failed",
		WithRetryable(false),
		WithRetryAfterSeconds(5),
		WithErrorCause(sentinel),
	)
	if !errors.Is(withRecovery, sentinel) {
		t.Fatal(
			"errors.Is(withRecovery, sentinel) = false, want the cause reachable despite recovery fields",
		)
	}
}

func TestWriteErrorRecoveryEnvelope(t *testing.T) {
	err := NewError(
		context.Background(),
		"network_timeout",
		"upstream timed out",
		WithErrorTool("app"),
		WithErrorVersion("v0.1.0"),
		WithErrorExitCode(ExitNetwork),
		WithActionableFix("retry the request"),
		WithSuggestions("check upstream status"),
		WithRetryable(true),
		WithRetryAfterSeconds(30),
	)

	var stderr bytes.Buffer
	if writeErr := WriteError(&stderr, err); writeErr != nil {
		t.Fatalf("WriteError returned error: %v", writeErr)
	}
	assertGolden(t, "testdata/error_recovery_envelope.golden.json", stderr.Bytes())
}

func TestErrorDefaultShapeUnchanged(t *testing.T) {
	err := NewError(
		context.Background(),
		"validation_error",
		"bad input",
		WithErrorTool("app"),
		WithErrorVersion("v0.1.0"),
	)

	var stderr bytes.Buffer
	if writeErr := WriteError(&stderr, err); writeErr != nil {
		t.Fatalf("WriteError returned error: %v", writeErr)
	}
	got := stderr.String()
	for _, key := range []string{"retryable", "retry_after_seconds"} {
		if strings.Contains(got, key) {
			t.Fatalf("default envelope unexpectedly contains %q: %s", key, got)
		}
	}
}

func TestRootErrorEnvelopeMatchesIsolatedContractShape(t *testing.T) {
	rootErr := NewError(
		context.Background(),
		"validation_error",
		"bad input",
		WithErrorTool("app"),
		WithErrorVersion("v0.1.0"),
		WithActionableFix("fix the input"),
		WithErrorContext(map[string]any{"field": "name"}),
		WithSuggestions("retry with --help"),
		WithRetryable(true),
		WithRetryAfterSeconds(30),
		WithErrorExitCode(ExitValidation),
	)
	contractErr := contract.NewError(
		context.Background(),
		"validation_error",
		"bad input",
		contract.WithErrorTool("app"),
		contract.WithErrorVersion("v0.1.0"),
		contract.WithActionableFix("fix the input"),
		contract.WithErrorContext(map[string]any{"field": "name"}),
		contract.WithSuggestions("retry with --help"),
		contract.WithRetryable(true),
		contract.WithRetryAfterSeconds(30),
		contract.WithErrorExitCode(contract.ExitValidation),
	)

	var rootOut bytes.Buffer
	if err := WriteError(&rootOut, rootErr); err != nil {
		t.Fatalf("root WriteError returned error: %v", err)
	}
	var contractOut bytes.Buffer
	if err := contract.WriteError(&contractOut, contractErr); err != nil {
		t.Fatalf("contract WriteError returned error: %v", err)
	}
	if !bytes.Equal(rootOut.Bytes(), contractOut.Bytes()) {
		t.Fatalf(
			"root error envelope diverged from contract\nroot:     %s\ncontract: %s",
			rootOut.Bytes(),
			contractOut.Bytes(),
		)
	}
}
