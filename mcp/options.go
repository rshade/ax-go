package mcp

// Transport identifies an MCP transport.
type Transport int

const (
	// TransportStdio serves MCP over stdin/stdout using newline-delimited JSON.
	// It is the default transport.
	TransportStdio Transport = iota
	// TransportHTTP serves MCP over a streamable HTTP transport. It binds
	// loopback by default; a non-loopback bind also requires
	// WithAllowNonLoopback.
	TransportHTTP
)

// Option configures Serve and NewCommand. Options are functional and
// composable; invalid combinations fail closed at startup with a validation
// error (exit 2) rather than a panic.
type Option func(*options)

// options is the resolved, unexported server configuration assembled from
// functional options. All configuration enters through Serve/NewCommand
// arguments; the package holds no mutable package-level state.
type options struct {
	transport        Transport
	httpAddr         string
	allowNonLoopback bool
	version          string
}

// defaultHTTPAddr is the loopback default bind address for the HTTP transport.
// It is fail-closed: a non-loopback address requires WithAllowNonLoopback.
const defaultHTTPAddr = "127.0.0.1:8080"

// resolveOptions applies opts over the defaults and returns the resolved
// configuration. The HTTP address defaults to loopback so an operator must opt
// in (via WithHTTPAddr plus WithAllowNonLoopback) before any public bind.
func resolveOptions(opts []Option) options {
	resolved := options{
		transport: TransportStdio,
		httpAddr:  defaultHTTPAddr,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&resolved)
		}
	}
	return resolved
}

// WithTransport selects the transport (TransportStdio default, or
// TransportHTTP).
func WithTransport(t Transport) Option {
	return func(o *options) {
		o.transport = t
	}
}

// WithHTTPAddr sets the bind address for the HTTP transport. The default is
// loopback; a non-loopback host also requires WithAllowNonLoopback(true).
func WithHTTPAddr(addr string) Option {
	return func(o *options) {
		o.httpAddr = addr
	}
}

// WithAllowNonLoopback permits the HTTP transport to bind a non-loopback or
// public interface. It is required (fail-closed) before a public bind is
// accepted; without it, a non-loopback WithHTTPAddr fails startup with a
// validation error.
func WithAllowNonLoopback(allow bool) Option {
	return func(o *options) {
		o.allowNonLoopback = allow
	}
}

// WithVersion sets the server version reported in the MCP initialize handshake.
// The version must be a real build version: an empty or placeholder ("dev" or
// "unknown") value is rejected fail-closed at startup (validation error, exit
// 2), so adopters inject a real version via -ldflags or WithVersion.
func WithVersion(version string) Option {
	return func(o *options) {
		o.version = version
	}
}
