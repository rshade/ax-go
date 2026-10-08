package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/testutil"
)

func TestExecuteWarningsAndStrict(t *testing.T) {
	payload := map[string]string{"status": "ok"}
	warnings := []Warning{
		{Code: "first", Message: "one"},
		{Code: "third", Message: "three"},
	}
	newRoot := func() *cobra.Command {
		return &cobra.Command{
			Use: "app",
			RunE: func(cmd *cobra.Command, _ []string) error {
				env := WithWarnings(cmd.Context(), NewEnvelope(cmd.Context(), payload), warnings...)
				return WriteJSON(cmd.OutOrStdout(), env)
			},
		}
	}

	t.Run("without strict, warnings stay on stdout and exit 0", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Execute(context.Background(), newRoot(),
			WithStdout(&stdout), WithStderr(&stderr), WithStdoutIsTTY(false))
		if code != ExitSuccess {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.Bytes())
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %s, want empty", stderr.Bytes())
		}
		var got struct {
			Data     map[string]string `json:"data"`
			Warnings []Warning         `json:"warnings"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("stdout %s: %v", stdout.Bytes(), err)
		}
		if got.Data["status"] != "ok" || len(got.Warnings) != 2 ||
			got.Warnings[0].Code != "first" || got.Warnings[1].Code != "third" {
			t.Fatalf("stdout = %s", stdout.Bytes())
		}
	})

	t.Run("strict with warnings exits 2 and leaves stdout empty", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		root := newRoot()
		root.SetArgs([]string{"--strict"})
		code := Execute(context.Background(), root,
			WithStdout(&stdout), WithStderr(&stderr), WithStdoutIsTTY(false))
		if code != ExitValidation {
			t.Fatalf("exit = %d, want 2; stdout=%s stderr=%s", code, stdout.Bytes(), stderr.Bytes())
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %s, want empty", stdout.Bytes())
		}
		var got struct {
			ErrorCode string `json:"error_code"`
			Context   struct {
				Warnings []Warning `json:"warnings"`
			} `json:"context"`
		}
		if err := json.Unmarshal(stderr.Bytes(), &got); err != nil {
			t.Fatalf("stderr %s: %v", stderr.Bytes(), err)
		}
		if got.ErrorCode != "warnings_as_errors" {
			t.Fatalf("error_code = %q, stderr=%s", got.ErrorCode, stderr.Bytes())
		}
		if len(got.Context.Warnings) != 2 || got.Context.Warnings[0].Code != "first" {
			t.Fatalf("context warnings = %#v", got.Context.Warnings)
		}
	})

	t.Run("strict without warnings still succeeds", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		root := &cobra.Command{
			Use: "app",
			RunE: func(cmd *cobra.Command, _ []string) error {
				return WriteJSON(cmd.OutOrStdout(), NewEnvelope(cmd.Context(), payload))
			},
		}
		root.SetArgs([]string{"--strict"})
		code := Execute(context.Background(), root,
			WithStdout(&stdout), WithStderr(&stderr), WithStdoutIsTTY(false))
		if code != ExitSuccess {
			t.Fatalf("exit = %d, want 0; stderr=%s", code, stderr.Bytes())
		}
		if stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("stdout=%s stderr=%s", stdout.Bytes(), stderr.Bytes())
		}
		if bytes.Contains(stdout.Bytes(), []byte("warnings")) {
			t.Fatalf("warnings leaked into %s", stdout.Bytes())
		}
	})

	t.Run("a returned error wins over strict", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		root := &cobra.Command{
			Use: "app",
			RunE: func(cmd *cobra.Command, _ []string) error {
				_ = WithWarnings(cmd.Context(), NewEnvelope(cmd.Context(), payload), warnings...)
				return NewError(cmd.Context(), "validation_error", "bad input", WithErrorExitCode(ExitValidation))
			},
		}
		root.SetArgs([]string{"--strict"})
		code := Execute(context.Background(), root,
			WithStdout(&stdout), WithStderr(&stderr), WithStdoutIsTTY(false))
		if code != ExitValidation {
			t.Fatalf("exit = %d, want 2; stderr=%s", code, stderr.Bytes())
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %s, want empty", stdout.Bytes())
		}
		if bytes.Contains(stderr.Bytes(), []byte("warnings_as_errors")) {
			t.Fatalf("strict escalation replaced the command error: %s", stderr.Bytes())
		}
		if !bytes.Contains(stderr.Bytes(), []byte("validation_error")) {
			t.Fatalf("stderr = %s, want validation_error", stderr.Bytes())
		}
	})

	t.Run("two runs are byte-identical after masking", func(t *testing.T) {
		run := func() []byte {
			var stdout, stderr bytes.Buffer
			code := Execute(context.Background(), newRoot(),
				WithStdout(&stdout), WithStderr(&stderr), WithStdoutIsTTY(false))
			if code != ExitSuccess {
				t.Fatalf("exit = %d, stderr=%s", code, stderr.Bytes())
			}
			return testutil.MaskNonDeterministic(stdout.Bytes())
		}
		first, second := run(), run()
		if !bytes.Equal(first, second) {
			t.Fatalf("masked stdout differs\n%s\n%s", first, second)
		}
	})
}

// TestExecuteStrictResetsBetweenCalls runs two Executes on one command tree.
// The second call must see a fresh --strict value and the stdout writer
// Execute was given for that call.
func TestExecuteStrictResetsBetweenCalls(t *testing.T) {
	payload := map[string]string{"status": "ok"}
	warnings := []Warning{
		{Code: "first", Message: "one"},
		{Code: "third", Message: "three"},
	}
	t.Run("same tree can run again after strict", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
		}{
			{name: "second call passes --strict=false", args: []string{"warn", "--strict=false"}},
			{name: "second call omits --strict", args: []string{"warn"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				root := &cobra.Command{Use: "app"}
				root.AddCommand(&cobra.Command{
					Use: "warn",
					RunE: func(cmd *cobra.Command, _ []string) error {
						env := WithWarnings(cmd.Context(), NewEnvelope(cmd.Context(), payload), warnings...)
						return WriteJSON(cmd.OutOrStdout(), env)
					},
				})

				run := func(args []string) (int, *bytes.Buffer, *bytes.Buffer) {
					stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
					root.SetArgs(args)
					code := Execute(context.Background(), root,
						WithStdout(stdout), WithStderr(stderr), WithStdoutIsTTY(false))
					return code, stdout, stderr
				}

				code, stdout, stderr := run([]string{"warn", "--strict"})
				if code != ExitValidation || stdout.Len() != 0 ||
					!bytes.Contains(stderr.Bytes(), []byte("warnings_as_errors")) {
					t.Fatalf("first exit=%d stdout=%s stderr=%s", code, stdout.Bytes(), stderr.Bytes())
				}

				code, stdout, stderr = run(tc.args)
				if code != ExitSuccess {
					t.Fatalf("second exit = %d, want 0; stdout=%s stderr=%s", code, stdout.Bytes(), stderr.Bytes())
				}
				if stderr.Len() != 0 {
					t.Fatalf("second stderr = %s, want empty", stderr.Bytes())
				}
				var got struct {
					Data     map[string]string `json:"data"`
					Warnings []Warning         `json:"warnings"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
					t.Fatalf("second stdout %s: %v", stdout.Bytes(), err)
				}
				if got.Data["status"] != "ok" || len(got.Warnings) != 2 || got.Warnings[0].Code != "first" {
					t.Fatalf("second stdout = %s", stdout.Bytes())
				}
			})
		}
	})
}
