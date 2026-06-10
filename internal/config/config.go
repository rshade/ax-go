package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/tailscale/hujson"
)

const (
	// MaxConfigBytesCeiling is the largest valid config read limit in bytes.
	MaxConfigBytesCeiling int64 = 1 << 30

	readChunkSize = 32 * 1024

	// maxPreallocBytes caps the read buffer's up-front allocation at the
	// default cap plus the one-over tripwire byte (1 MiB + 1). Caps at or
	// below it read with zero reallocation; larger caps start here and grow
	// only as input actually arrives, so a huge cap never allocates huge
	// memory for a small config.
	maxPreallocBytes int64 = 1<<20 + 1
)

// InvalidMaxBytesError reports an invalid config read limit.
type InvalidMaxBytesError struct {
	MaxBytes int64
}

// Error returns the validation message for an out-of-range config read limit.
func (e InvalidMaxBytesError) Error() string {
	return fmt.Sprintf("config max bytes must be between 0 and %d", MaxConfigBytesCeiling)
}

// TooLargeError reports config input that exceeded the configured read limit.
type TooLargeError struct {
	MaxBytes int64
}

// Error returns the validation message for oversized config input.
func (e TooLargeError) Error() string {
	return fmt.Sprintf("config exceeds maximum size of %d bytes", e.MaxBytes)
}

// ReadBounded reads config bytes from r under maxBytes, measured in bytes.
//
// It consumes at most maxBytes+1 bytes so one byte over the cap is detectable
// without reading the rest of the source. maxBytes values below zero or above
// MaxConfigBytesCeiling return InvalidMaxBytesError before any source read; the
// finite ceiling keeps maxBytes+1 overflow impossible. If ctx is canceled or
// expires between chunk reads, ReadBounded returns ctx.Err() wrapped with %w.
// If the source exceeds maxBytes, ReadBounded returns TooLargeError. A non-EOF
// source error observed before maxBytes+1 bytes is returned with its chain
// preserved; if one read both crosses maxBytes and returns a non-EOF source
// error, the oversize classification wins. A Read that returns (0, nil) — no
// data and no error — is rejected immediately as io.ErrNoProgress rather than
// retried. The read buffer pre-allocates at most maxPreallocBytes (1 MiB + 1):
// caps at or below the default read with zero reallocation, while larger caps
// grow with the input, so peak memory tracks bytes actually read (with
// append's transient growth overhead), never the configured ceiling.
func ReadBounded(ctx context.Context, r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 || maxBytes > MaxConfigBytesCeiling {
		return nil, InvalidMaxBytesError{MaxBytes: maxBytes}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	limit := maxBytes + 1
	data := make([]byte, 0, min(limit, maxPreallocBytes))
	buf := make([]byte, readChunkSize)
	for int64(len(data)) < limit {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("read config: %w", err)
		}

		remaining := limit - int64(len(data))
		readBuf := buf
		if remaining < int64(len(readBuf)) {
			readBuf = readBuf[:remaining]
		}

		n, err := r.Read(readBuf)
		if n > 0 {
			data = append(data, readBuf[:n]...)
		}
		if int64(len(data)) > maxBytes {
			return nil, TooLargeError{MaxBytes: maxBytes}
		}
		if err == io.EOF {
			return data, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read config source: %w", err)
		}
		if n == 0 {
			return nil, fmt.Errorf("read config source: %w", io.ErrNoProgress)
		}
	}
	return data, nil
}

// Unmarshal parses Hujson, standardizes it to strict JSON, and decodes it.
//
// dst is intentionally any because encoding/json.Unmarshal accepts arbitrary
// destination pointers.
func Unmarshal(data []byte, dst any) error {
	value, err := hujson.Parse(data)
	if err != nil {
		return fmt.Errorf("parse hujson: %w", err)
	}
	value.Standardize()

	if unmarshalErr := json.Unmarshal(value.Pack(), dst); unmarshalErr != nil {
		return fmt.Errorf("unmarshal config: %w", unmarshalErr)
	}
	return nil
}

// HujsonParseError wraps a Hujson parse failure so the caller can distinguish
// a bad-input error from a patch-application error.
type HujsonParseError struct {
	Err error
}

// Error returns the Hujson parse failure message.
func (e *HujsonParseError) Error() string { return "parse hujson: " + e.Err.Error() }

// Unwrap preserves the error chain so errors.Is and errors.As reach the root cause.
func (e *HujsonParseError) Unwrap() error { return e.Err }

// PatchApplyError wraps an RFC 6902 patch failure so the caller can distinguish
// it from a bad-input (parse) error.
type PatchApplyError struct {
	Err error
}

// Error returns the patch application failure message.
func (e *PatchApplyError) Error() string { return "apply patch: " + e.Err.Error() }

// Unwrap preserves the error chain so errors.Is and errors.As reach the root cause.
func (e *PatchApplyError) Unwrap() error { return e.Err }

// Patch parses data as Hujson, applies RFC 6902 patch operations to the AST,
// formats the result canonically, and returns the patched Hujson bytes.
//
// Comments in data are preserved through the AST. Whitespace is NOT preserved:
// Format normalizes indentation, value alignment, and blank lines to canonical
// Hujson style. Format is required because patch operations splice raw JSON
// into the AST — without it, added values render minified mid-line. The patch
// document must be strict JSON per RFC 6902 (an array of operation objects). A
// Hujson parse failure returns *HujsonParseError; a patch-application failure
// returns *PatchApplyError; no other error types are returned — the public
// error normalization in package ax relies on that invariant.
func Patch(data []byte, patch []byte) ([]byte, error) {
	ast, parseErr := hujson.Parse(data)
	if parseErr != nil {
		return nil, &HujsonParseError{Err: parseErr}
	}
	if patchErr := ast.Patch(patch); patchErr != nil {
		return nil, &PatchApplyError{Err: patchErr}
	}
	ast.Format()
	return ast.Pack(), nil
}
