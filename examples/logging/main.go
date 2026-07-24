// Command logging is the minimal consumer of ax-go's import-isolated logging
// surface, and the artifact internal/cmd/sizecheck measures against the absolute
// binary-size ceiling.
//
// It imports github.com/rshade/ax-go/logging and nothing else outside the
// standard library, which is the point: the resulting binary contains no OTLP
// exporter, no gRPC, no protobuf, no Cobra, and no net/http, yet still emits
// fully trace-correlated structured logs.
//
// Its counterpart examples/rootlogging is byte-for-byte this program with
// logging.NewLogger swapped for ax.NewLogger. The two differ by one import and
// one call, so the ratio between their binary sizes isolates exactly one
// variable: which surface was imported. Keep them diff-clean against each other.
package main

import (
	"context"

	"github.com/rshade/ax-go/logging"
)

// Label values the program declares. They are constants so the command and its
// tests agree by construction rather than by two copies of a string literal.
const (
	appName = "example-logging"
	envName = "dev"
)

func main() {
	ctx := context.Background()

	log := logging.NewLogger(ctx,
		logging.WithLoggerLabels(logging.Labels{
			Application: appName,
			Environment: envName,
		}),
	)

	emit(ctx, log)
}

// emit writes one correlated line on the diagnostic stream. It is a separate
// function so the test can exercise it against an injected writer without
// running main, keeping the stream assertions honest about which stream each
// byte landed on.
func emit(ctx context.Context, log logging.Logger) {
	log.Info(ctx).Str("stage", "startup").Msg("ready")
}
