package ax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

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
