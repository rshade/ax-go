package contract

import "testing"

func TestExitCodes(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{name: "success", got: ExitSuccess, want: 0},
		{name: "internal", got: ExitInternal, want: 1},
		{name: "validation", got: ExitValidation, want: 2},
		{name: "network", got: ExitNetwork, want: 3},
		{name: "auth", got: ExitAuth, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("exit code = %d, want %d", tt.got, tt.want)
			}
		})
	}
}
