package config

import (
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/tailscale/hujson"
)

// InvalidMaxBytesError reports an invalid config read limit.
type InvalidMaxBytesError struct {
	MaxBytes int64
}

// Error returns the validation message for a negative config read limit.
func (e InvalidMaxBytesError) Error() string {
	return "config max bytes must be greater than or equal to zero"
}

// TooLargeError reports config input that exceeded the configured read limit.
type TooLargeError struct {
	MaxBytes int64
}

// Error returns the validation message for oversized config input.
func (e TooLargeError) Error() string {
	return fmt.Sprintf("config exceeds maximum size of %d bytes", e.MaxBytes)
}

// ReadBounded reads at most maxBytes from r and reports oversized input.
func ReadBounded(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, InvalidMaxBytesError{MaxBytes: maxBytes}
	}

	limit := maxBytes + 1
	if maxBytes == math.MaxInt64 {
		limit = maxBytes
	}

	data, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, TooLargeError{MaxBytes: maxBytes}
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
