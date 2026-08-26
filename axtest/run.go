package axtest

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"

	ax "github.com/rshade/ax-go"
)

// Result is the full outcome of one Run call: the command's captured
// machine-payload stream, its captured diagnostic stream, and the
// deterministic exit code ax.Execute returned. All three are independently
// inspectable because a blocked confirmation or validation failure writes its
// ax.Error envelope to Stderr, never Stdout (Constitution Principle I), so a
// test asserting on that path has nothing to decode from Stdout at all.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Run executes root through the same ax.Execute lifecycle a production binary
// uses — agent-safety flags mounted, mode resolved, context populated — and
// returns the captured stdout, captured stderr, and resulting exit code.
//
// Run captures stdout and stderr itself regardless of whether opts also
// supplies ax.WithStdout or ax.WithStderr: Run appends its own capture options
// last, so they take precedence over anything opts contains for the Result
// this call returns. ax.WithStdout/ax.WithStderr exist for production use, not
// for overriding what Run reports.
//
// Run never calls t.Fatal, t.Fatalf, or t.Error itself: a non-zero ExitCode is
// returned, not treated as a Run-level failure, because a command under test
// may deliberately exercise a validation or permission failure. It calls
// t.Helper() so a caller's subsequent failure attributes to the caller's own
// line, not into this package.
//
// ctx is forwarded to ax.Execute unmodified. Run performs I/O — it executes an
// arbitrary command tree, which may itself do network or filesystem I/O — so
// Constitution Principle X requires context.Context as the first parameter
// here with no test-code exception. A caller with no specific need passes
// context.Background() explicitly.
//
// Run is safe to call more than once against the same root within one test:
// it does not reset flag values a previous call set, which is ordinary
// command-framework behavior, not Run's concern. It is also safe to call
// concurrently from parallel subtests, each against its own root command tree:
// Run holds no package-level or shared state between calls.
func Run(
	ctx context.Context,
	t testing.TB,
	root *cobra.Command,
	args []string,
	opts ...ax.ExecuteOption,
) Result {
	t.Helper()

	var stdout, stderr bytes.Buffer
	root.SetArgs(args)
	exitCode := ax.Execute(ctx, root, append(opts, ax.WithStdout(&stdout), ax.WithStderr(&stderr))...)

	return Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
	}
}
