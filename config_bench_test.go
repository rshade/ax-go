package ax

import (
	"context"
	"strings"
	"testing"
)

func BenchmarkParseConfigBoundedRead(b *testing.B) {
	const capBytes int64 = 1024

	cases := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:  "1x_cap",
			input: strings.Repeat(" ", int(capBytes)-2) + "{}",
		},
		{
			name:      "10x_cap",
			input:     strings.Repeat(" ", int(capBytes*10)),
			wantError: true,
		},
		{
			name:      "100x_cap",
			input:     strings.Repeat(" ", int(capBytes*100)),
			wantError: true,
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				var got struct{}
				err := ParseConfig(
					context.Background(),
					strings.NewReader(tc.input),
					&got,
					WithMaxConfigBytes(capBytes),
				)
				if tc.wantError && err == nil {
					b.Fatal("ParseConfig returned nil error")
				}
				if !tc.wantError && err != nil {
					b.Fatalf("ParseConfig returned error: %v", err)
				}
			}
		})
	}
}

// BenchmarkParseConfigDefaultCapRead records B/op and allocs/op for a full-size
// read at the default 1 MiB cap, substantiating the pre-allocated read buffer
// (zero reallocation on the default path; SC-001 memory ≈ cap).
func BenchmarkParseConfigDefaultCapRead(b *testing.B) {
	input := strings.Repeat(" ", int(DefaultMaxConfigBytes)-2) + "{}"

	for b.Loop() {
		var got struct{}
		if err := ParseConfig(context.Background(), strings.NewReader(input), &got); err != nil {
			b.Fatalf("ParseConfig returned error: %v", err)
		}
	}
}
