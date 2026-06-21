package ax

import (
	"context"
	"io"

	isolatedconfig "github.com/rshade/ax-go/config"
)

const (
	// DefaultMaxConfigBytes is the default maximum config size: 1 MiB.
	DefaultMaxConfigBytes int64 = isolatedconfig.DefaultMaxBytes
	// MaxConfigBytesCeiling is the largest valid config read limit: 1 GiB.
	MaxConfigBytesCeiling int64 = isolatedconfig.MaxBytesCeiling
)

// ParseConfigOption configures ParseConfig and ParseConfigFile.
type ParseConfigOption = isolatedconfig.Option

// WithMaxConfigBytes sets the maximum config bytes for one parse invocation.
func WithMaxConfigBytes(maxBytes int64) ParseConfigOption {
	return isolatedconfig.WithMaxBytes(maxBytes)
}

// ParseConfig parses Hujson from r under a bounded read cap and unmarshals into dst.
func ParseConfig(ctx context.Context, r io.Reader, dst any, opts ...ParseConfigOption) error {
	return isolatedconfig.Parse(withTraceMetadata(ctx), r, dst, opts...)
}

// ParseConfigFile opens path and applies ParseConfig's contract to its contents.
func ParseConfigFile(ctx context.Context, path string, dst any, opts ...ParseConfigOption) error {
	return isolatedconfig.ParseFile(withTraceMetadata(ctx), path, dst, opts...)
}

// PatchConfig reads Hujson from r, applies RFC 6902 JSON patch operations, and
// returns the patched Hujson content with comments preserved.
func PatchConfig(ctx context.Context, r io.Reader, patch []byte, opts ...ParseConfigOption) ([]byte, error) {
	return isolatedconfig.Patch(withTraceMetadata(ctx), r, patch, opts...)
}

// PatchConfigFile reads path as Hujson, applies RFC 6902 patch operations, and
// writes the patched result back to path atomically, preserving comments.
func PatchConfigFile(ctx context.Context, path string, patch []byte, opts ...ParseConfigOption) error {
	return isolatedconfig.PatchFile(withTraceMetadata(ctx), path, patch, opts...)
}
