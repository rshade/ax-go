package ax

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// executeTelemetryCommand runs a minimal Cobra command through Execute with the
// supplied environment and shutdown budget, returning stdout, stderr, and the
// exit code.
//
// This helper deliberately lives in an untagged file. Its call sites span
// telemetry_export_test.go (built only without ax_no_otlp), telemetry_debug_test.go,
// telemetry_security_test.go, and execute_test.go; keeping it here is what lets the
// OTLP-dependent test files carry //go:build !ax_no_otlp without breaking test
// compilation of the declined build.
func executeTelemetryCommand(t *testing.T, env map[string]string, shutdownBudget time.Duration) ([]byte, string, int) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := &cobra.Command{
		Use: "app",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
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
		WithTelemetryShutdownTimeout(shutdownBudget),
		WithEnv(func(key string) string {
			return env[key]
		}),
	)
	return stdout.Bytes(), stderr.String(), code
}
