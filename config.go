package ax

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	internalconfig "github.com/rshade/ax-go/internal/config"
)

const (
	// DefaultMaxConfigBytes is the default maximum config size: 1 MiB.
	DefaultMaxConfigBytes int64 = 1 << 20
)

// ParseConfigOption configures ParseConfig and ParseConfigFile.
type ParseConfigOption func(*parseConfigOptions)

type parseConfigOptions struct {
	maxBytes int64
}

// WithMaxConfigBytes sets the maximum number of bytes read from config input.
func WithMaxConfigBytes(maxBytes int64) ParseConfigOption {
	return func(cfg *parseConfigOptions) {
		cfg.maxBytes = maxBytes
	}
}

// ParseConfig parses Hujson from r, standardizes it to strict JSON, and
// unmarshals it into dst.
func ParseConfig(r io.Reader, dst any, opts ...ParseConfigOption) error {
	cfg := parseConfigOptions{maxBytes: DefaultMaxConfigBytes}
	for _, opt := range opts {
		opt(&cfg)
	}

	data, err := internalconfig.ReadBounded(r, cfg.maxBytes)
	if err != nil {
		return normalizeConfigReadError(err)
	}

	return internalconfig.Unmarshal(data, dst)
}

// ParseConfigFile parses a Hujson configuration file into dst.
func ParseConfigFile(path string, dst any, opts ...ParseConfigOption) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return ParseConfig(file, dst, opts...)
}

func normalizeConfigReadError(err error) error {
	var invalidMax internalconfig.InvalidMaxBytesError
	if errors.As(err, &invalidMax) {
		return NewError(
			context.Background(),
			"config_max_bytes_invalid",
			"config max bytes must be greater than or equal to zero",
			WithActionableFix("set a non-negative config byte limit"),
			WithErrorContext(map[string]any{"max_bytes": invalidMax.MaxBytes}),
			WithErrorExitCode(ExitValidation),
		)
	}

	var tooLarge internalconfig.TooLargeError
	if errors.As(err, &tooLarge) {
		return NewError(
			context.Background(),
			"config_too_large",
			fmt.Sprintf("config exceeds maximum size of %d bytes", tooLarge.MaxBytes),
			WithActionableFix("reduce the config size or raise the limit with WithMaxConfigBytes"),
			WithErrorContext(map[string]any{"max_bytes": tooLarge.MaxBytes}),
			WithErrorExitCode(ExitValidation),
		)
	}
	return err
}
