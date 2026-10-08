//go:build ax_no_grpc

package taggedread

func useGRPC() bool { return load().grpc }
