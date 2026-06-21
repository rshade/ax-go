package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rshade/ax-go/contract"
	internalconfig "github.com/rshade/ax-go/internal/config"
)

const (
	// DefaultMaxBytes is the default maximum config size: 1 MiB.
	DefaultMaxBytes int64 = 1 << 20
	// MaxBytesCeiling is the largest valid config read limit: 1 GiB.
	MaxBytesCeiling int64 = internalconfig.MaxConfigBytesCeiling
)

// Option configures Parse, ParseFile, Patch, and PatchFile.
type Option func(*options)

type options struct {
	maxBytes int64
}

// WithMaxBytes sets the maximum config bytes for one parse or patch operation.
func WithMaxBytes(maxBytes int64) Option {
	return func(cfg *options) {
		cfg.maxBytes = maxBytes
	}
}

// Parse parses Hujson from r under a bounded read cap and unmarshals into dst.
func Parse(ctx context.Context, r io.Reader, dst any, opts ...Option) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := applyOptions(ctx, opts)
	if err != nil {
		return err
	}

	data, err := internalconfig.ReadBounded(ctx, r, cfg.maxBytes)
	if err != nil {
		return normalizeReadError(ctx, err)
	}

	if decodeErr := internalconfig.Unmarshal(data, dst); decodeErr != nil {
		return normalizeDecodeError(ctx, decodeErr)
	}
	return nil
}

// ParseFile opens path and applies Parse's contract to its contents.
func ParseFile(ctx context.Context, path string, dst any, opts ...Option) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return Parse(ctx, file, dst, opts...)
}

// Patch reads Hujson from r, applies RFC 6902 JSON patch operations, and
// returns patched Hujson content with comments preserved.
func Patch(ctx context.Context, r io.Reader, patch []byte, opts ...Option) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := applyOptions(ctx, opts)
	if err != nil {
		return nil, err
	}

	data, err := internalconfig.ReadBounded(ctx, r, cfg.maxBytes)
	if err != nil {
		return nil, normalizeReadError(ctx, err)
	}

	patched, err := internalconfig.Patch(data, patch)
	if err != nil {
		return nil, normalizePatchError(ctx, err)
	}
	return patched, nil
}

// PatchFile reads path as Hujson, applies RFC 6902 patch operations, and
// writes the patched result back to path atomically.
func PatchFile(ctx context.Context, path string, patch []byte, opts ...Option) error {
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

	patched, err := Patch(ctx, file, patch, opts...)
	if err != nil {
		return err
	}

	return atomicWriteFile(path, patched, info.Mode())
}

func applyOptions(ctx context.Context, opts []Option) (options, error) {
	cfg := options{maxBytes: DefaultMaxBytes}
	for _, opt := range opts {
		if opt == nil {
			return cfg, contract.NewError(
				ctx,
				"config_option_invalid",
				"config parse option must not be nil",
				contract.WithActionableFix("remove nil ParseConfigOption values before parsing config"),
				contract.WithErrorExitCode(contract.ExitValidation),
			)
		}
		opt(&cfg)
	}
	return cfg, nil
}

func normalizeReadError(ctx context.Context, err error) error {
	var invalidMax internalconfig.InvalidMaxBytesError
	if errors.As(err, &invalidMax) {
		return contract.NewError(
			ctx,
			"config_max_bytes_invalid",
			fmt.Sprintf("config max bytes must be between 0 and %d", MaxBytesCeiling),
			contract.WithActionableFix("set a config byte limit between 0 and MaxConfigBytesCeiling"),
			contract.WithErrorContext(map[string]any{"max_bytes": invalidMax.MaxBytes}),
			contract.WithErrorExitCode(contract.ExitValidation),
		)
	}

	var tooLarge internalconfig.TooLargeError
	if errors.As(err, &tooLarge) {
		return contract.NewError(
			ctx,
			"config_too_large",
			fmt.Sprintf("config exceeds maximum size of %d bytes", tooLarge.MaxBytes),
			contract.WithActionableFix("reduce the config size or raise the limit with WithMaxConfigBytes"),
			contract.WithErrorContext(map[string]any{"max_bytes": tooLarge.MaxBytes}),
			contract.WithErrorExitCode(contract.ExitValidation),
		)
	}
	return err
}

func normalizeDecodeError(ctx context.Context, decodeErr error) error {
	var invalidUnmarshal *json.InvalidUnmarshalError
	if errors.As(decodeErr, &invalidUnmarshal) {
		return decodeErr
	}

	return contract.NewError(
		ctx,
		"config_invalid",
		"config is not valid Hujson or does not match the expected schema",
		contract.WithActionableFix("fix the config syntax or field types and retry"),
		contract.WithErrorCause(decodeErr),
		contract.WithErrorExitCode(contract.ExitValidation),
	)
}

func normalizePatchError(ctx context.Context, err error) error {
	var parseErr *internalconfig.HujsonParseError
	if errors.As(err, &parseErr) {
		return contract.NewError(
			ctx,
			"config_invalid",
			"config is not valid Hujson",
			contract.WithActionableFix("fix the config syntax and retry"),
			contract.WithErrorCause(parseErr.Err),
			contract.WithErrorExitCode(contract.ExitValidation),
		)
	}

	var patchErr *internalconfig.PatchApplyError
	if errors.As(err, &patchErr) {
		return contract.NewError(
			ctx,
			"config_patch_invalid",
			"config patch is not a valid RFC 6902 document or a patch operation failed",
			contract.WithActionableFix(
				"verify the patch document is a valid RFC 6902 JSON array and all target paths exist",
			),
			contract.WithErrorCause(patchErr.Err),
			contract.WithErrorExitCode(contract.ExitValidation),
		)
	}
	return err
}

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

func writeAndClose(file *os.File, name string, data []byte, mode os.FileMode) error {
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", name, err)
	}
	return nil
}
