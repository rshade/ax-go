//go:build ax_no_grpc

// This build declines the instrumented outbound gRPC dial helper.
//
// The ax_no_grpc build constraint removes ax.GRPCDial, which is the only
// production code in this module that imports google.golang.org/grpc. Calling
// it here fails at compile time with the Go toolchain's standard message:
//
//	undefined: ax.GRPCDial
//
// Go gives a library no hook to customise that diagnostic, so this file is the
// explanation it cannot carry inline. To restore the helper, drop ax_no_grpc
// from the build tags — nothing else changes.
//
// Removing this helper alone barely shrinks a binary. The gRPC subtree has two
// independent roots, GRPCDial and the OTLP exporter, so it survives until both
// are gone. Pair this constraint with ax_no_otlp; together they take a
// root-facade binary down roughly 63%, while ax_no_grpc on its own measures
// close to nothing.
//
// Nothing else is affected. ax.HTTPClient and ax.NewHTTPClient remain present
// and instrumented, tracing still records and correlates, and every machine
// payload is byte-identical to the default build. ax.GRPCDial is the single
// public identifier whose presence varies with a build tag.
//
// This file declares no symbol on purpose. A stub retaining GRPCDial's
// signature would retain grpc.DialOption and *grpc.ClientConn in the exported
// surface, pulling the whole dependency tree back in and defeating the
// constraint; an exported marker constant would make the declined build both
// lose and gain an identifier for no practical benefit.

package ax
