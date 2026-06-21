package contract

import "testing"

func TestResolveModePrecedence(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		agentMode   string
		stdoutIsTTY bool
		want        Mode
		wantErr     bool
	}{
		{
			name:        "explicit format wins over env and tty",
			format:      "human",
			agentMode:   "1",
			stdoutIsTTY: false,
			want:        ModeHuman,
		},
		{
			name:        "agent mode wins over tty",
			agentMode:   "TRUE",
			stdoutIsTTY: true,
			want:        ModeJSON,
		},
		{
			name:        "tty defaults human",
			stdoutIsTTY: true,
			want:        ModeHuman,
		},
		{
			name:        "non tty defaults json",
			stdoutIsTTY: false,
			want:        ModeJSON,
		},
		{
			name:    "invalid explicit mode errors",
			format:  "xml",
			wantErr: true,
		},
		{
			name:      "invalid agent mode errors",
			agentMode: "maybe",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveMode(tt.format, tt.agentMode, tt.stdoutIsTTY)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ResolveMode returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveMode returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveMode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseModeAliases(t *testing.T) {
	tests := []struct {
		input string
		want  Mode
	}{
		{input: "json", want: ModeJSON},
		{input: "agent", want: ModeJSON},
		{input: "machine", want: ModeJSON},
		{input: "human", want: ModeHuman},
		{input: "text", want: ModeHuman},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseMode(tt.input)
			if err != nil {
				t.Fatalf("ParseMode returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseMode = %q, want %q", got, tt.want)
			}
		})
	}
}
