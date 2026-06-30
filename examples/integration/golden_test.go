package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ax "github.com/rshade/ax-go"
	"github.com/rshade/ax-go/internal/testutil"
)

// updateGolden, when set via `go test -update`, rewrites the golden fixtures in
// testdata/ from the current command output instead of comparing against them.
//
//nolint:gochecknoglobals // test-only golden-file update flag must be package-scoped for the flag package
var updateGolden = flag.Bool("update", false, "update golden files in testdata/")

// goldenVersion is the fixed version injected for golden runs so __schema.version
// and ax.Error.version stay byte-stable across commits (ResolveVersion otherwise
// falls through to the git revision, which changes every commit).
const goldenVersion = "v9.9.9-golden"

const goldenIdempotencyKey = "golden-key"

// runGolden drives the integration command through ax.Execute with the
// deterministic inputs pinned: a fixed version and a fixed entity ID under an
// empty environment. ax.Execute still generates fresh random trace_id/span_id
// per run, so callers mask those (and idempotency_key) before comparison.
func runGolden(t *testing.T, args []string) (string, string, int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := runWithEntityID(
		context.Background(),
		args,
		strings.NewReader(""),
		&out,
		&errBuf,
		emptyEnv,
		goldenVersion,
		func() (string, error) { return deterministicEntityID, nil },
	)
	return out.String(), errBuf.String(), code
}

// assertGolden compares got against testdata/<name>, or rewrites it under
// `-update`. Callers MUST mask non-deterministic fields (trace_id, span_id,
// idempotency_key) via testutil.MaskNonDeterministic before calling, because
// ax.Execute's OTel span generates fresh random trace/span IDs every run.
// Regenerate every fixture with:
//
//	go test ./examples/integration -run TestGolden -update
func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("update golden %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./examples/integration -run TestGolden -update`)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\nwant: %s\ngot:  %s", path, want, got)
	}
}

// TestGoldenSchema pins the example's reflected __schema output (AC#2).
func TestGoldenSchema(t *testing.T) {
	stdout, _, code := runGolden(t, []string{schemaCommandName})
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitSuccess)
	}
	assertGolden(t, "schema_ax.golden.json", testutil.MaskNonDeterministic([]byte(stdout)))
}

// TestGoldenSchemaMCP pins the __schema --as=mcp adapter output (Mandate #4).
func TestGoldenSchemaMCP(t *testing.T) {
	stdout, _, code := runGolden(t, []string{schemaCommandName, "--as=mcp"})
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitSuccess)
	}
	assertGolden(t, "schema_mcp.golden.json", testutil.MaskNonDeterministic([]byte(stdout)))
}

// TestGoldenRootSuccess pins the root command's bounded JSON success envelope.
func TestGoldenRootSuccess(t *testing.T) {
	stdout, _, code := runGolden(t, []string{
		"--format=json", "--idempotency-key=" + goldenIdempotencyKey, "--name=Ada",
	})
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitSuccess)
	}
	assertGolden(t, "root_success.golden.json", testutil.MaskNonDeterministic([]byte(stdout)))
}

// TestGoldenStreamSuccess pins the stream command's NDJSON success output.
func TestGoldenStreamSuccess(t *testing.T) {
	stdout, _, code := runGolden(t, []string{
		"stream", "--format=json", "--idempotency-key=" + goldenIdempotencyKey, "--count=2", "--name=Ada",
	})
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitSuccess)
	}
	assertGolden(t, "stream_success.golden.json", testutil.MaskNonDeterministic([]byte(stdout)))
}

// TestGoldenPatchConfigSuccess pins the patch-config success envelope. The temp
// config path is inherently non-deterministic, so it is masked to <config-path>.
func TestGoldenPatchConfigSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hujson")
	initial := []byte("{\n\t// integration config\n\t\"name\": \"Ada\",\n\t\"count\": 2,\n}")
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	stdout, _, code := runGolden(t, []string{
		"patch-config", "--format=json", "--idempotency-key=" + goldenIdempotencyKey,
		"--config=" + path, `--patch=[{"op":"replace","path":"/count","value":5}]`,
	})
	if code != ax.ExitSuccess {
		t.Fatalf("exit code = %d, want %d", code, ax.ExitSuccess)
	}
	masked := testutil.MaskNonDeterministic([]byte(stdout))
	masked = bytes.ReplaceAll(masked, []byte(path), []byte("<config-path>"))
	assertGolden(t, "patch_config_success.golden.json", masked)
}

// TestGoldenErrorEnvelopes pins one ax.Error envelope per exit-code category
// (Mandate #9): validation, network, auth, and internal. The patch-config
// required-flags error is the second validation example, kept distinct because
// it exercises a different command's error wiring.
func TestGoldenErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		golden   string
	}{
		{"fail", []string{failCommandName, "--format=json"}, ax.ExitValidation, "fail_error.golden.json"},
		{"fetch", []string{fetchCommandName, "--format=json"}, ax.ExitNetwork, "fetch_error.golden.json"},
		{"authz", []string{authzCommandName, "--format=json"}, ax.ExitAuth, "authz_error.golden.json"},
		{"crash", []string{crashCommandName, "--format=json"}, ax.ExitInternal, "crash_error.golden.json"},
		{
			"patch-config-required",
			[]string{patchConfigCommandName, "--format=json"},
			ax.ExitValidation,
			"patch_config_error.golden.json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runGolden(t, tc.args)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d; stderr=%s", code, tc.wantCode, stderr)
			}
			if stdout != "" {
				t.Fatalf("error path leaked stdout: %q", stdout)
			}
			assertGolden(t, tc.golden, testutil.MaskNonDeterministic([]byte(stderr)))
		})
	}
}
