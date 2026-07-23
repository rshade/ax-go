package ax

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// DefaultHTTPTimeout is the default total timeout applied to clients returned
// by HTTPClient and NewHTTPClient. It bounds the full request lifecycle
// (connect, redirects, reading the response body) so an outbound call can never
// block forever.
const DefaultHTTPTimeout = 30 * time.Second

// HTTPClientOption configures NewHTTPClient.
type HTTPClientOption func(*httpClientConfig)

type httpClientConfig struct {
	timeout time.Duration
}

// WithHTTPTimeout sets the total request timeout on the returned client.
//
// Non-positive durations are ignored and fall back to DefaultHTTPTimeout: a
// zero http.Client.Timeout disables the timeout entirely, which is exactly the
// unbounded-request hazard the client constructors exist to prevent.
func WithHTTPTimeout(d time.Duration) HTTPClientOption {
	return func(cfg *httpClientConfig) {
		cfg.timeout = d
	}
}

// HTTPClient returns an HTTP client with OTel propagation instrumentation and
// the default bounded request timeout (DefaultHTTPTimeout). To override the
// timeout or set other options, use NewHTTPClient with WithHTTPTimeout. TLS
// verification uses the http.DefaultTransport secure defaults; never relax them.
func HTTPClient() *http.Client {
	return NewHTTPClient()
}

// NewHTTPClient returns an HTTP client with OTel propagation instrumentation and
// a bounded request timeout (DefaultHTTPTimeout unless overridden with
// WithHTTPTimeout). TLS verification uses the http.DefaultTransport secure
// defaults; never relax them.
func NewHTTPClient(opts ...HTTPClientOption) *http.Client {
	cfg := httpClientConfig{timeout: DefaultHTTPTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.timeout <= 0 {
		cfg.timeout = DefaultHTTPTimeout
	}
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   cfg.timeout,
	}
}
