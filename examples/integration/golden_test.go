package main

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestByteIdenticalModuloNonDeterministicFields(t *testing.T) {
	schemaOutput, _, schemaCode := runGolden(t, []string{schemaCommandName})
	if schemaCode != ax.ExitSuccess {
		t.Fatalf("schema exit code = %d, want %d", schemaCode, ax.ExitSuccess)
	}
	var discovered ax.Schema
	if err := json.Unmarshal([]byte(schemaOutput), &discovered); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	runRoot := func(entityID string) []byte {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runWithEntityID(
			context.Background(),
			[]string{"--format=json", "--name=Ada"},
			strings.NewReader(""),
			&stdout,
			&stderr,
			emptyEnv,
			goldenVersion,
			func() (string, error) { return entityID, nil },
		)
		if code != ax.ExitSuccess {
			t.Fatalf("root exit code = %d, want %d; stderr=%s", code, ax.ExitSuccess, stderr.String())
		}
		return stdout.Bytes()
	}

	first := runRoot("019744d2-1a5f-7000-8000-000000000001")
	second := runRoot("019744d2-1a5f-7000-8000-000000000002")
	if bytes.Equal(first, second) {
		t.Fatal("unmasked runs are byte-identical, want non-deterministic fields to differ")
	}

	firstMasked := deleteJSONLocators(t, first, discovered.Command.NonDeterministicFields)
	secondMasked := deleteJSONLocators(t, second, discovered.Command.NonDeterministicFields)
	if !bytes.Equal(firstMasked, secondMasked) {
		t.Fatalf("masked outputs differ\nfirst:  %s\nsecond: %s", firstMasked, secondMasked)
	}
}

func deleteJSONLocators(t *testing.T, payload []byte, locators []string) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode command output: %v", err)
	}
	for _, locator := range locators {
		deleteJSONPath(t, document, strings.Split(locator, "."))
	}
	masked, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode masked command output: %v", err)
	}
	return masked
}

func deleteJSONPath(t *testing.T, document map[string]json.RawMessage, path []string) {
	t.Helper()
	if len(path) == 0 {
		t.Fatal("empty non-deterministic field locator")
	}

	if child, ok := document[path[0]]; ok {
		if len(path) == 1 {
			delete(document, path[0])
			return
		}
		document[path[0]] = deleteJSONValue(t, child, path[1:])
		return
	}

	// path[0] is not a literal key at this level. Raw JSON bytes cannot
	// distinguish a marshaled Go struct from a marshaled Go map, and
	// research.md D3 omits map keys from locators the same way it omits
	// array indices, so a missing literal key means this object is a map
	// value type: apply the full remaining path uniformly to every value.
	if len(document) == 0 {
		t.Fatalf("locator path %q is missing from command output", strings.Join(path, "."))
	}
	for key, value := range document {
		document[key] = deleteJSONValue(t, value, path)
	}
}

// deleteJSONValue applies the remaining locator path segments to raw. A JSON
// array (produced by a Go slice/array field) is not indexed by a locator
// segment — research.md D3 omits array indices because list positions are
// not stable across runs — so path is applied to every element uniformly. A
// JSON object is treated as a struct with literal field keys first; when a
// path segment isn't a literal key, deleteJSONPath falls back to treating
// the object as a map value type and applies the path to every value, since
// map keys are equally omitted from locators for the same reason.
func deleteJSONValue(t *testing.T, raw json.RawMessage, path []string) json.RawMessage {
	t.Helper()

	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err == nil {
		for i, element := range elements {
			elements[i] = deleteJSONValue(t, element, path)
		}
		encoded, err := json.Marshal(elements)
		if err != nil {
			t.Fatalf("encode locator path %q: %v", strings.Join(path, "."), err)
		}
		return encoded
	}

	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		t.Fatalf("decode locator path %q: %v", strings.Join(path, "."), err)
	}
	deleteJSONPath(t, nested, path)
	encoded, err := json.Marshal(nested)
	if err != nil {
		t.Fatalf("encode locator path %q: %v", strings.Join(path, "."), err)
	}
	return encoded
}

// TestDeleteJSONLocatorsArrayElement guards against a regression where a
// locator descending through a slice/array field (e.g. "data.items.id",
// research.md D3's no-index-segment format) crashed deleteJSONPath instead
// of masking the field in every array element.
func TestDeleteJSONLocatorsArrayElement(t *testing.T) {
	first := []byte(`{"data":{"items":[{"id":"one","name":"a"},{"id":"two","name":"b"}]}}`)
	second := []byte(`{"data":{"items":[{"id":"three","name":"a"},{"id":"four","name":"b"}]}}`)

	firstMasked := deleteJSONLocators(t, first, []string{"data.items.id"})
	secondMasked := deleteJSONLocators(t, second, []string{"data.items.id"})
	if !bytes.Equal(firstMasked, secondMasked) {
		t.Fatalf("masked outputs differ\nfirst:  %s\nsecond: %s", firstMasked, secondMasked)
	}

	var masked map[string]json.RawMessage
	if err := json.Unmarshal(firstMasked, &masked); err != nil {
		t.Fatalf("decode masked output: %v", err)
	}
	if !strings.Contains(string(masked["data"]), `"name":"a"`) {
		t.Errorf("masked output = %s, want deterministic sibling field preserved", masked["data"])
	}
}

// TestDeleteJSONLocatorsMapValue guards against a regression where a locator
// descending through a map value field (e.g. "data.items.id", the same
// no-index-segment format research.md D3 uses for map keys) hard-failed or
// silently no-op'd in deleteJSONPath instead of masking the field in every
// map value.
func TestDeleteJSONLocatorsMapValue(t *testing.T) {
	first := []byte(`{"data":{"items":{"k1":{"id":"one","name":"a"},"k2":{"id":"two","name":"b"}}}}`)
	second := []byte(`{"data":{"items":{"k1":{"id":"three","name":"a"},"k2":{"id":"four","name":"b"}}}}`)

	firstMasked := deleteJSONLocators(t, first, []string{"data.items.id"})
	secondMasked := deleteJSONLocators(t, second, []string{"data.items.id"})
	if !bytes.Equal(firstMasked, secondMasked) {
		t.Fatalf("masked outputs differ\nfirst:  %s\nsecond: %s", firstMasked, secondMasked)
	}

	var masked map[string]json.RawMessage
	if err := json.Unmarshal(firstMasked, &masked); err != nil {
		t.Fatalf("decode masked output: %v", err)
	}
	if !strings.Contains(string(masked["data"]), `"name":"a"`) {
		t.Errorf("masked output = %s, want deterministic sibling field preserved", masked["data"])
	}
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
