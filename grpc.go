//go:build !ax_no_grpc

// This file holds the instrumented outbound gRPC dial helper. It is the only
// production code in the module that references google.golang.org/grpc, and the
// reason the gRPC tree is reachable from a consumer that never exports traces.
//
// Its sibling grpc_disabled.go documents the ax_no_grpc build constraint that
// removes it. Unlike the OTLP seam, this one has no stub: GRPCDial is exported,
// and a stub retaining its signature would retain grpc.DialOption and
// *grpc.ClientConn — and therefore the entire dependency tree the constraint
// exists to remove.

package ax

import (
	"context"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

// GRPCDial dials target with OTel client instrumentation.
//
// This helper is absent from builds using the ax_no_grpc build constraint,
// where calling it fails at compile time with "undefined: ax.GRPCDial". See
// grpc_disabled.go for the restoration step.
func GRPCDial(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	instrumented := []grpc.DialOption{grpc.WithStatsHandler(otelgrpc.NewClientHandler())}
	instrumented = append(instrumented, opts...)
	return grpc.NewClient(target, instrumented...)
}
