package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// This file is untagged so every assertion in it runs under all four supported
// build configurations (FR-012, SC-005). That is the whole point: a payload
// parity claim verified only in the default build says nothing about the builds
// the claim is actually about.
//
// The committed testdata/ goldens are the shared reference. Because the same
// golden is compared in every configuration, "byte-identical across
// configurations" is enforced transitively without needing to run two
// configurations in one process — which build tags make impossible anyway.

// TestBuildTagParitySchemaPayloadMatchesGolden asserts the __schema payload is
// byte-identical to the committed golden in whichever configuration this binary
// was built for.
func TestBuildTagParitySchemaPayloadMatchesGolden(t *testing.T) {
	root := newSchemaTestCommand()

	var stdout bytes.Buffer
	if err := WriteJSON(&stdout, BuildSchema(root, WithSchemaVersion("v0.1.0"))); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_ax.golden.json", stdout.Bytes())
}

// TestBuildTagParityMCPSchemaIsStable asserts the __schema --as=mcp adapter —
// the surface an agent actually consumes — renders identically in every
// configuration.
//
// It asserts against the committed golden rather than a literal copy of the
// payload. An inline copy has to be updated by hand whenever the MCP shape
// grows a field (it went stale once already, when nonDeterministicFields was
// added), and a parity test that needs hand-editing to track the real shape is
// a parity test that can silently stop matching it.
func TestBuildTagParityMCPSchemaIsStable(t *testing.T) {
	root := newSchemaTestCommand()

	var stdout bytes.Buffer
	if err := WriteJSON(&stdout, BuildMCPSchema(root)); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_mcp.golden.json", stdout.Bytes())
}

// TestBuildTagParityErrorEnvelopeMatchesGolden asserts the ax.Error envelope is
// byte-identical to the committed golden in every configuration.
func TestBuildTagParityErrorEnvelopeMatchesGolden(t *testing.T) {
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
}

// TestBuildTagParityStreamSeparation asserts stdout carries the payload and
// nothing else in every configuration — including the ax_no_otlp build, which
// writes an extra "exporter disabled" diagnostic that must land on stderr.
func TestBuildTagParityStreamSeparation(t *testing.T) {
	stdout, stderr, code := executeTelemetryCommand(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:1",
	}, defaultTelemetryShutdownTimeout)

	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr)
	}
	if !bytes.Equal(stdout, []byte("{\"ok\":true}\n")) {
		t.Fatalf("stdout = %q, want the payload alone", stdout)
	}
	if strings.Contains(string(stdout), "ax:") {
		t.Fatalf("stdout contains a diagnostic: %q", stdout)
	}
}

// TestBuildTagParityExitCodeMapping asserts the deterministic exit-code contract
// is unaffected by either build constraint.
func TestBuildTagParityExitCodeMapping(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{name: "success", err: nil, wantCode: ExitSuccess},
		{
			name: "validation",
			err: NewError(context.Background(), "validation_error", "bad input",
				WithErrorExitCode(ExitValidation)),
			wantCode: ExitValidation,
		},
		{
			name: "network",
			err: NewError(context.Background(), "network_error", "timeout",
				WithErrorExitCode(ExitNetwork)),
			wantCode: ExitNetwork,
		},
		{
			name: "auth",
			err: NewError(context.Background(), "auth_error", "denied",
				WithErrorExitCode(ExitAuth)),
			wantCode: ExitAuth,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			root := &cobra.Command{
				Use: "app",
				RunE: func(cmd *cobra.Command, _ []string) error {
					if tc.err != nil {
						return tc.err
					}
					return WriteJSON(cmd.OutOrStdout(), struct {
						OK bool `json:"ok"`
					}{OK: true})
				},
			}

			code := Execute(
				context.Background(),
				root,
				WithStdout(&stdout),
				WithStderr(&stderr),
				WithStdoutIsTTY(false),
				WithEnv(func(string) string { return "" }),
			)
			if code != tc.wantCode {
				t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, tc.wantCode, stderr.String())
			}
		})
	}
}

// TestBuildTagParityDryRunSuppressesSideEffects asserts --dry-run still produces
// the same envelope with dry_run true and performs no side effect, in every
// configuration.
func TestBuildTagParityDryRunSuppressesSideEffects(t *testing.T) {
	var sideEffects int

	var stdout, stderr bytes.Buffer
	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !DryRunFromContext(cmd.Context()) {
				sideEffects++
			}
			return WriteJSON(cmd.OutOrStdout(), struct {
				DryRun bool `json:"dry_run"`
			}{DryRun: DryRunFromContext(cmd.Context())})
		},
	}
	root.SetArgs([]string{"--dry-run"})

	code := Execute(
		context.Background(),
		root,
		WithStdout(&stdout),
		WithStderr(&stderr),
		WithStdoutIsTTY(false),
		WithEnv(func(string) string { return "" }),
	)
	if code != ExitSuccess {
		t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
	}
	if sideEffects != 0 {
		t.Fatalf("side effects performed under --dry-run: %d", sideEffects)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v (%q)", err, stdout.String())
	}
	if payload["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true; stdout=%s", payload["dry_run"], stdout.String())
	}
}

// TestBuildTagParityIdempotencyKeyIsSurfaced asserts an explicitly supplied
// idempotency key reaches the command context, and that an absent one is
// auto-generated, in every configuration.
func TestBuildTagParityIdempotencyKeyIsSurfaced(t *testing.T) {
	const suppliedKey = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

	run := func(args []string) string {
		t.Helper()

		var seen string
		var seenOK bool
		var stdout, stderr bytes.Buffer
		root := &cobra.Command{
			Use: "app",
			RunE: func(cmd *cobra.Command, _ []string) error {
				seen, seenOK = IdempotencyKeyFromContext(cmd.Context())
				return WriteJSON(cmd.OutOrStdout(), struct {
					OK bool `json:"ok"`
				}{OK: true})
			},
		}
		root.SetArgs(args)

		code := Execute(
			context.Background(),
			root,
			WithStdout(&stdout),
			WithStderr(&stderr),
			WithStdoutIsTTY(false),
			WithEnv(func(string) string { return "" }),
		)
		if code != ExitSuccess {
			t.Fatalf("Execute exit code = %d, want %d; stderr=%s", code, ExitSuccess, stderr.String())
		}
		if !seenOK {
			t.Fatal("idempotency key missing from the command context")
		}
		return seen
	}

	if got := run([]string{"--idempotency-key", suppliedKey}); got != suppliedKey {
		t.Fatalf("IdempotencyKeyFromContext = %q, want the supplied key %q", got, suppliedKey)
	}
	if got := run(nil); got == "" {
		t.Fatal("IdempotencyKeyFromContext is empty; an absent key must be auto-generated")
	}
}
