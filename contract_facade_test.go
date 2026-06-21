package ax

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/rshade/ax-go/contract"
)

func TestRootContractFacadeUsesIsolatedTypes(t *testing.T) {
	var mode contract.Mode = ModeJSON
	if mode != contract.ModeJSON {
		t.Fatalf("root ModeJSON = %q, want contract json mode", mode)
	}

	var option contract.ErrorOption = WithErrorTool("app")
	err := NewError(context.Background(), "validation_error", "bad input", option)
	var contractErr *contract.Error = err
	if contractErr.Tool != "app" {
		t.Fatalf("Tool = %q, want app", contractErr.Tool)
	}

	var envelope contract.Envelope[string] = NewEnvelope(context.Background(), "ok")
	if envelope.Meta.TraceID != contract.ZeroTraceID {
		t.Fatalf("TraceID = %q, want %q", envelope.Meta.TraceID, contract.ZeroTraceID)
	}
}

func TestRootContractFacadeMatchesIsolatedBehavior(t *testing.T) {
	ctx := WithIdempotencyKey(WithDryRun(context.Background(), true), "key-1")
	rootEnvelope := NewEnvelope(ctx, "ok")
	contractEnvelope := contract.NewEnvelope(ctx, "ok")
	if rootEnvelope.Meta.IdempotencyKey != contractEnvelope.Meta.IdempotencyKey {
		t.Fatalf(
			"idempotency key = %q, want %q",
			rootEnvelope.Meta.IdempotencyKey,
			contractEnvelope.Meta.IdempotencyKey,
		)
	}
	if rootEnvelope.Meta.DryRun != contractEnvelope.Meta.DryRun {
		t.Fatalf("dry run = %v, want %v", rootEnvelope.Meta.DryRun, contractEnvelope.Meta.DryRun)
	}

	rootErr := NewError(context.Background(), "validation_error", "bad input", WithErrorExitCode(ExitValidation))
	if !errors.As(rootErr, new(*contract.Error)) {
		t.Fatalf("errors.As(rootErr, *contract.Error) = false")
	}
	if ErrorExitCode(rootErr) != contract.ErrorExitCode(rootErr) {
		t.Fatalf("root and contract ErrorExitCode disagree")
	}

	var buf bytes.Buffer
	if err := WriteJSONLine(&buf, rootEnvelope); err != nil {
		t.Fatalf("WriteJSONLine returned error: %v", err)
	}
	if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
		t.Fatalf("WriteJSONLine output %q, want trailing newline", buf.String())
	}
}
