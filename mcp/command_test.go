package mcp

import (
	"context"
	"errors"
	"testing"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/mcpserver"
)

// TestParseTransport pins the --transport value mapping: stdio is the default,
// parsing is case/space tolerant, and an unknown value fails closed with a
// validation error (exit 2) and the zero Transport — never a valid-looking
// transport alongside the error.
func TestParseTransport(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		want      mcpserver.Transport
		wantError bool
	}{
		{name: "empty defaults to stdio", value: "", want: mcpserver.TransportStdio},
		{name: "stdio", value: "stdio", want: mcpserver.TransportStdio},
		{name: "http", value: "http", want: mcpserver.TransportHTTP},
		{name: "case and space tolerant", value: " HTTP ", want: mcpserver.TransportHTTP},
		{name: "unknown rejected", value: "grpc", wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseTransport(context.Background(), tc.value)
			if tc.wantError {
				if err == nil {
					t.Fatalf("parseTransport(%q) returned nil error", tc.value)
				}
				if got != 0 {
					t.Errorf("parseTransport(%q) returned transport %d alongside the error, want the zero value",
						tc.value, got)
				}
				var contractErr *contract.Error
				if !errors.As(err, &contractErr) || contractErr.ErrorCode != "validation_error" {
					t.Errorf("error = %v, want a validation_error contract.Error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTransport(%q) returned error: %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("parseTransport(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}
