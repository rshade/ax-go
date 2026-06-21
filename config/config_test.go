package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rshade/ax-go/contract"
)

func TestParseAcceptsHujson(t *testing.T) {
	input := `{
		// comments are allowed
		"name": "ax",
		"ports": [8080, 9090,],
	}`
	var got struct {
		Name  string `json:"name"`
		Ports []int  `json:"ports"`
	}

	if err := Parse(context.Background(), strings.NewReader(input), &got); err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Name != "ax" {
		t.Fatalf("Name = %q, want ax", got.Name)
	}
	if len(got.Ports) != 2 || got.Ports[0] != 8080 || got.Ports[1] != 9090 {
		t.Fatalf("Ports = %#v, want [8080 9090]", got.Ports)
	}
}

func TestParseClassifiesValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int64
		wantCode string
	}{
		{
			name:     "too large",
			input:    "{}",
			maxBytes: 1,
			wantCode: "config_too_large",
		},
		{
			name:     "invalid max bytes",
			input:    "{}",
			maxBytes: -1,
			wantCode: "config_max_bytes_invalid",
		},
		{
			name:     "invalid hujson",
			input:    "{",
			maxBytes: DefaultMaxBytes,
			wantCode: "config_invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got struct{}
			err := Parse(context.Background(), strings.NewReader(tt.input), &got, WithMaxBytes(tt.maxBytes))
			assertContractError(t, err, tt.wantCode)
		})
	}
}

func TestParseRejectsNilOption(t *testing.T) {
	var got struct{}
	err := Parse(context.Background(), strings.NewReader("{}"), &got, nil)
	assertContractError(t, err, "config_option_invalid")
}

func TestParseFileHonorsOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.hujson")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var got struct{}
	err := ParseFile(context.Background(), path, &got, WithMaxBytes(1))
	assertContractError(t, err, "config_too_large")
}

func TestPatchPreservesComments(t *testing.T) {
	const existing = `{
	// service endpoint
	"host": "localhost",
	"port": 8080,
}`
	patch := []byte(`[{"op":"replace","path":"/port","value":9090}]`)

	patched, err := Patch(context.Background(), strings.NewReader(existing), patch)
	if err != nil {
		t.Fatalf("Patch returned error: %v", err)
	}
	if !strings.Contains(string(patched), "// service endpoint") {
		t.Fatalf("patched config lost comment:\n%s", patched)
	}
	if !strings.Contains(string(patched), "9090") {
		t.Fatalf("patched config missing replacement:\n%s", patched)
	}
}

func TestPatchClassifiesErrors(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		patch    []byte
		wantCode string
	}{
		{
			name:     "invalid hujson",
			existing: "{",
			patch:    []byte(`[]`),
			wantCode: "config_invalid",
		},
		{
			name:     "invalid patch",
			existing: `{"a":1}`,
			patch:    []byte(`not json`),
			wantCode: "config_patch_invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Patch(context.Background(), strings.NewReader(tt.existing), tt.patch)
			assertContractError(t, err, tt.wantCode)
		})
	}
}

func TestPatchFileWritesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.hujson")
	initial := []byte(`{
	// production endpoint
	"host": "prod.example.com",
	"port": 443,
}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	patch := []byte(`[{"op":"replace","path":"/port","value":8443}]`)
	if err := PatchFile(context.Background(), path, patch); err != nil {
		t.Fatalf("PatchFile returned error: %v", err)
	}

	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched config: %v", err)
	}
	if !strings.Contains(string(result), "// production endpoint") {
		t.Fatalf("patched file lost comment:\n%s", result)
	}
	if !strings.Contains(string(result), "8443") {
		t.Fatalf("patched file missing replacement:\n%s", result)
	}
}

func assertContractError(t *testing.T, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatal("got nil error, want contract error")
	}
	var contractErr *contract.Error
	if !errors.As(err, &contractErr) {
		t.Fatalf("error type = %T, want *contract.Error", err)
	}
	if contractErr.ErrorCode != wantCode {
		t.Fatalf("ErrorCode = %q, want %q", contractErr.ErrorCode, wantCode)
	}
	if got := contract.ErrorExitCode(err); got != contract.ExitValidation {
		t.Fatalf("ErrorExitCode = %d, want %d", got, contract.ExitValidation)
	}
}
