package ax

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if err := ParseConfig(strings.NewReader(input), &got); err != nil {
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

	err := ParseConfig(input, &got)
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

	if err := ParseConfig(input, &got); err != nil {
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

			err := ParseConfig(strings.NewReader(tc.input), &got, WithMaxConfigBytes(tc.max))
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

func TestParseConfigFileHonorsMaxConfigBytesOption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.hujson")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	var got struct{}
	err := ParseConfigFile(path, &got, WithMaxConfigBytes(1))
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
