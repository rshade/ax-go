package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rshade/ax-go/contract"
)

const (
	// httpShutdownBudget bounds graceful HTTP shutdown after ctx cancellation.
	httpShutdownBudget = 5 * time.Second
	// httpReadHeaderTimeout is a secure default guarding against slow-header
	// clients (Principle IX).
	httpReadHeaderTimeout = 10 * time.Second
)

// serveStdio runs the server over the stdio transport until ctx is canceled or
// the client disconnects, returning nil on clean shutdown (FR-015/FR-020,
// C-14/C-19).
func serveStdio(ctx context.Context, server *sdk.Server) error {
	// A canceled context is a clean shutdown, not a transport failure: only a
	// run error that is not attributable to cancellation propagates.
	if err := server.Run(ctx, &sdk.StdioTransport{}); err != nil && ctx.Err() == nil {
		return fmt.Errorf("mcp: stdio transport: %w", err)
	}
	return nil
}

// serveHTTP binds the loopback-guarded listener and serves the streamable HTTP
// transport until ctx is canceled (FR-016/FR-018, C-15/C-17/C-19).
func serveHTTP(ctx context.Context, server *sdk.Server, cfg Config) error {
	listener, err := loopbackListener(ctx, cfg)
	if err != nil {
		return err
	}
	return serveOnListener(ctx, server, listener)
}

// loopbackListener validates cfg.HTTPAddr against the loopback policy and binds
// it. A non-loopback host without AllowNonLoopback fails closed with a
// validation error (exit 2) before any socket is opened (FR-016/FR-018, C-15).
func loopbackListener(ctx context.Context, cfg Config) (net.Listener, error) {
	host, _, splitErr := net.SplitHostPort(cfg.HTTPAddr)
	if splitErr != nil {
		return nil, contract.NewError(ctx, "validation_error",
			fmt.Sprintf("mcp: invalid http address %q: %v", cfg.HTTPAddr, splitErr),
			contract.WithErrorExitCode(contract.ExitValidation))
	}
	if !cfg.AllowNonLoopback && !isLoopbackHost(host) {
		return nil, contract.NewError(ctx, "validation_error",
			fmt.Sprintf(
				"mcp: refusing to bind non-loopback address %q without WithAllowNonLoopback "+
					"(fail-closed against accidental public exposure)", cfg.HTTPAddr),
			contract.WithErrorExitCode(contract.ExitValidation))
	}
	var listenConfig net.ListenConfig
	listener, listenErr := listenConfig.Listen(ctx, "tcp", cfg.HTTPAddr)
	if listenErr != nil {
		return nil, fmt.Errorf("mcp: listen on %s: %w", cfg.HTTPAddr, listenErr)
	}
	return listener, nil
}

// serveOnListener serves the streamable HTTP transport on listener until ctx is
// canceled, then drains in-flight requests within httpShutdownBudget and
// returns nil (C-19/C-20). It is the white-box seam exercised by the transport
// and concurrency tests.
func serveOnListener(ctx context.Context, server *sdk.Server, listener net.Listener) error {
	// JSONResponse returns each tool result as a single application/json response
	// rather than a lingering text/event-stream. This matches the bridge's
	// buffer-the-whole-result model (D9, C-21) and lets graceful shutdown close
	// idle connections promptly instead of waiting on an open SSE stream.
	handler := sdk.NewStreamableHTTPHandler(
		func(*http.Request) *sdk.Server { return server },
		&sdk.StreamableHTTPOptions{JSONResponse: true},
	)
	httpServer := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServer.Serve(listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownBudget)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			// Graceful budget exceeded; force-close lingering connections so
			// Serve returns and the engine stops.
			_ = httpServer.Close()
		}
		<-serveErr
		return nil
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("mcp: http transport: %w", err)
	}
}

// isLoopbackHost reports whether host is a loopback bind target. An empty host
// (all interfaces) or unknown hostname is treated as non-loopback so the
// loopback guard fails closed.
func isLoopbackHost(host string) bool {
	switch host {
	case "":
		return false
	case "localhost":
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
