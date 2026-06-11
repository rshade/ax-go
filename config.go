package ax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	internalconfig "github.com/rshade/ax-go/internal/config"
)

const (
	// DefaultMaxConfigBytes is the default maximum config size: 1 MiB.
	DefaultMaxConfigBytes int64 = 1 << 20
	// MaxConfigBytesCeiling is the largest valid config read limit: 1 GiB.
	MaxConfigBytesCeiling int64 = internalconfig.MaxConfigBytesCeiling
)

// ParseConfigOption configures ParseConfig and ParseConfigFile.
type ParseConfigOption func(*parseConfigOptions)

type parseConfigOptions struct {
	maxBytes int64
}

// WithMaxConfigBytes sets the maximum config bytes for one parse invocation.
//
// The value is not global and does not affect later calls. Zero is a valid,
// honored limit: empty input passes the size check and then follows normal parse
// semantics, while any non-empty input is rejected as config_too_large. Values
// below zero or above MaxConfigBytesCeiling return config_max_bytes_invalid,
// mapped to exit code 2; there is no unbounded read path. Passing a nil
// ParseConfigOption is rejected as config_option_invalid, also mapped to exit
// code 2.
func WithMaxConfigBytes(maxBytes int64) ParseConfigOption {
	return func(cfg *parseConfigOptions) {
		cfg.maxBytes = maxBytes
	}
}

// ParseConfig parses Hujson from r under a bounded read cap and unmarshals into dst.
//
// Reads default to DefaultMaxConfigBytes and consume at most cap+1 bytes.
// Oversize input returns an errors.As-discoverable *Error with error_code
// config_too_large and exit code 2. A cap below zero or above
// MaxConfigBytesCeiling returns config_max_bytes_invalid and exit code 2. A nil
// ParseConfigOption returns config_option_invalid and exit code 2. Hujson parse
// and schema/type decode failures return config_invalid and exit code 2, with
// the underlying decode error preserved in the chain (reachable via errors.Is
// and errors.As through Unwrap). Invalid decode destinations, such as nil or
// non-pointer dst values, surface the underlying *json.InvalidUnmarshalError as
// caller misuse and are not classified as config_invalid. Every valid cap is at
// most MaxConfigBytesCeiling, so there is no unbounded read path. ctx
// cancellation is honored between chunk reads, not inside a single blocking
// Read. Wrapped context.DeadlineExceeded maps to exit code 3 via ErrorExitCode,
// and wrapped context.Canceled maps to exit code 1. A non-EOF source error
// before cap+1 bytes is returned with its chain preserved and is not classified
// as oversize; if the same read crosses the cap and returns a source error, the
// oversize validation error wins.
func ParseConfig(ctx context.Context, r io.Reader, dst any, opts ...ParseConfigOption) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := applyParseConfigOptions(ctx, opts)
	if err != nil {
		return err
	}

	data, err := internalconfig.ReadBounded(ctx, r, cfg.maxBytes)
	if err != nil {
		return normalizeConfigReadError(ctx, err)
	}

	if decodeErr := internalconfig.Unmarshal(data, dst); decodeErr != nil {
		return normalizeConfigDecodeError(ctx, decodeErr)
	}
	return nil
}

// ParseConfigFile opens path and applies ParseConfig's contract to its contents.
//
// The file is closed before return. Open failures are returned as-is; read,
// cap, context-cancellation, and Hujson decode behavior match ParseConfig.
func ParseConfigFile(ctx context.Context, path string, dst any, opts ...ParseConfigOption) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return ParseConfig(ctx, file, dst, opts...)
}

func applyParseConfigOptions(ctx context.Context, opts []ParseConfigOption) (parseConfigOptions, error) {
	cfg := parseConfigOptions{maxBytes: DefaultMaxConfigBytes}
	for _, opt := range opts {
		if opt == nil {
			return cfg, NewError(
				ctx,
				"config_option_invalid",
				"config parse option must not be nil",
				WithActionableFix("remove nil ParseConfigOption values before parsing config"),
				WithErrorExitCode(ExitValidation),
			)
		}
		opt(&cfg)
	}
	return cfg, nil
}

func normalizeConfigReadError(ctx context.Context, err error) error {
	var invalidMax internalconfig.InvalidMaxBytesError
	if errors.As(err, &invalidMax) {
		return NewError(
			ctx,
			"config_max_bytes_invalid",
			fmt.Sprintf("config max bytes must be between 0 and %d", MaxConfigBytesCeiling),
			WithActionableFix("set a config byte limit between 0 and MaxConfigBytesCeiling"),
			WithErrorContext(map[string]any{"max_bytes": invalidMax.MaxBytes}),
			WithErrorExitCode(ExitValidation),
		)
	}

	var tooLarge internalconfig.TooLargeError
	if errors.As(err, &tooLarge) {
		return NewError(
			ctx,
			"config_too_large",
			fmt.Sprintf("config exceeds maximum size of %d bytes", tooLarge.MaxBytes),
			WithActionableFix("reduce the config size or raise the limit with WithMaxConfigBytes"),
			WithErrorContext(map[string]any{"max_bytes": tooLarge.MaxBytes}),
			WithErrorExitCode(ExitValidation),
		)
	}
	return err
}

func normalizeConfigDecodeError(ctx context.Context, decodeErr error) error {
	var invalidUnmarshal *json.InvalidUnmarshalError
	if errors.As(decodeErr, &invalidUnmarshal) {
		return decodeErr
	}

	return NewError(
		ctx,
		"config_invalid",
		"config is not valid Hujson or does not match the expected schema",
		WithActionableFix("fix the config syntax or field types and retry"),
		WithErrorCause(decodeErr),
		WithErrorExitCode(ExitValidation),
	)
}

// PatchConfig reads Hujson from r, applies RFC 6902 JSON patch operations, and
// returns the patched Hujson content with comments preserved. Whitespace is
// normalized to canonical Hujson formatting; user indentation, value alignment,
// and blank lines are not preserved byte-for-byte.
//
// This is the comment-preserving write path: unlike strict-JSON writes, the
// returned bytes remain valid Hujson so the caller can write them back to a
// human-maintained config file without stripping user comments. The patch
// document must be a strict JSON array of RFC 6902 operation objects. An invalid
// existing config returns config_invalid; an invalid or failing patch returns
// config_patch_invalid; read cap and context errors follow ParseConfig's contract.
func PatchConfig(ctx context.Context, r io.Reader, patch []byte, opts ...ParseConfigOption) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := applyParseConfigOptions(ctx, opts)
	if err != nil {
		return nil, err
	}

	data, err := internalconfig.ReadBounded(ctx, r, cfg.maxBytes)
	if err != nil {
		return nil, normalizeConfigReadError(ctx, err)
	}

	patched, err := internalconfig.Patch(data, patch)
	if err != nil {
		return nil, normalizePatchError(ctx, err)
	}
	return patched, nil
}

// PatchConfigFile reads path as Hujson, applies RFC 6902 JSON patch operations,
// and writes the patched result back to path atomically, preserving comments.
// Whitespace is normalized to canonical Hujson formatting (see PatchConfig). The
// original file permissions are preserved.
//
// Open or stat failures are returned as-is. Patch behavior matches PatchConfig.
// The write is atomic: a temporary file in the same directory is written and
// then renamed, so a partial write never corrupts the existing file. The rename
// guarantees atomicity, not crash durability: no fsync is issued, so a power
// loss immediately after return may lose the write. If path is a symlink, the
// rename replaces the symlink itself with a regular file; the symlink target is
// not modified. Concurrent external writes to path follow last-writer-wins.
func PatchConfigFile(ctx context.Context, path string, patch []byte, opts ...ParseConfigOption) error {
	if ctx == nil {
		ctx = context.Background()
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	patched, err := PatchConfig(ctx, file, patch, opts...)
	if err != nil {
		return err
	}

	return atomicWriteFile(path, patched, info.Mode())
}

// atomicWriteFile writes data to path atomically: it creates a temporary file in
// the same directory, writes data to it, sets the given mode, and renames the
// temporary file over path. A failure before the rename does not corrupt path.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".ax-patch-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if err = writeAndClose(tmp, tmpPath, data, mode); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if renameErr := os.Rename(tmpPath, path); renameErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", renameErr)
	}
	return nil
}

// writeAndClose writes data to f, sets mode, and closes f. On any error it
// closes f (ignoring the close error) and returns the original error.
func writeAndClose(f *os.File, name string, data []byte, mode os.FileMode) error {
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", name, err)
	}
	return nil
}

// normalizePatchError maps internalconfig.Patch failures onto the frozen config
// error codes: *HujsonParseError becomes config_invalid and *PatchApplyError
// becomes config_patch_invalid, both mapping to ExitValidation. Patch guarantees
// it returns only those two wrapper types, so the trailing passthrough is a
// defensive guard for future error shapes, mirroring normalizeConfigDecodeError.
func normalizePatchError(ctx context.Context, err error) error {
	var parseErr *internalconfig.HujsonParseError
	if errors.As(err, &parseErr) {
		return NewError(
			ctx,
			"config_invalid",
			"config is not valid Hujson",
			WithActionableFix("fix the config syntax and retry"),
			WithErrorCause(parseErr.Err),
			WithErrorExitCode(ExitValidation),
		)
	}

	var patchErr *internalconfig.PatchApplyError
	if errors.As(err, &patchErr) {
		return NewError(
			ctx,
			"config_patch_invalid",
			"config patch is not a valid RFC 6902 document or a patch operation failed",
			WithActionableFix("verify the patch document is a valid RFC 6902 JSON array and all target paths exist"),
			WithErrorCause(patchErr.Err),
			WithErrorExitCode(ExitValidation),
		)
	}
	return err
}
