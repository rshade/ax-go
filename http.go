package ax

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
)

// HTTPClient returns an HTTP client with OTel propagation instrumentation.
func HTTPClient() *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

// GRPCDial dials target with OTel client instrumentation.
func GRPCDial(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	instrumented := []grpc.DialOption{grpc.WithStatsHandler(otelgrpc.NewClientHandler())}
	instrumented = append(instrumented, opts...)
	return grpc.NewClient(target, instrumented...)
}
