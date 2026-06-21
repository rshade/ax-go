package ax

import (
	"context"
	"testing"
)

func TestModeContextRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		mode   Mode
		wantOK bool
	}{
		{name: "json mode round-trips", mode: ModeJSON, wantOK: true},
		{name: "human mode round-trips", mode: ModeHuman, wantOK: true},
		// An explicitly-stored empty Mode is retrievable (ok=true); a missing key
		// is the ok=false path (tested separately below).
		{name: "empty mode value is retrievable", mode: Mode(""), wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithMode(context.Background(), tt.mode)
			got, ok := ModeFromContext(ctx)
			if ok != tt.wantOK {
				t.Fatalf("ModeFromContext ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.mode {
				t.Fatalf("ModeFromContext = %q, want %q", got, tt.mode)
			}
		})
	}
}

func TestModeFromContextMissingKeyReturnsFalse(t *testing.T) {
	_, ok := ModeFromContext(context.Background())
	if ok {
		t.Fatal("ModeFromContext on context with no mode key should return ok=false")
	}
}

func TestDryRunContextRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		dryRun bool
	}{
		{name: "dry-run enabled", dryRun: true},
		{name: "dry-run disabled", dryRun: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithDryRun(context.Background(), tt.dryRun)
			got := DryRunFromContext(ctx)
			if got != tt.dryRun {
				t.Fatalf("DryRunFromContext = %v, want %v", got, tt.dryRun)
			}
		})
	}
}

func TestDryRunFromContextMissingKeyReturnsFalse(t *testing.T) {
	if DryRunFromContext(context.Background()) {
		t.Fatal("DryRunFromContext on context with no dry-run key should return false")
	}
}

func TestIdempotencyKeyContextRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		wantOK bool
	}{
		{name: "non-empty key round-trips", key: "request-uuid-4321", wantOK: true},
		// An empty string stored via WithIdempotencyKey is treated the same as a
		// missing key: ok=false. This matches the IdempotencyKeyFromContext contract
		// (`ok && key != ""`), preventing callers from acting on a blank key.
		{name: "empty string treated as missing", key: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := WithIdempotencyKey(context.Background(), tt.key)
			got, ok := IdempotencyKeyFromContext(ctx)
			if ok != tt.wantOK {
				t.Fatalf("IdempotencyKeyFromContext ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && got != tt.key {
				t.Fatalf("IdempotencyKeyFromContext = %q, want %q", got, tt.key)
			}
		})
	}
}

func TestIdempotencyKeyFromContextMissingKeyReturnsNotOK(t *testing.T) {
	key, ok := IdempotencyKeyFromContext(context.Background())
	if ok {
		t.Fatalf("IdempotencyKeyFromContext on context with no key should return ok=false, got key=%q", key)
	}
}
