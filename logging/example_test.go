package logging_test

import (
	"context"
	"os"

	"github.com/rs/zerolog"

	"github.com/rshade/ax-go/logging"
)

// ExampleNewLogger demonstrates the whole primary surface of this package in one
// runnable, output-verified program: the constructor plus each functional option.
//
// The WithX options are shown here rather than as separately gated examples,
// which is the repository convention — an option only makes sense in the context
// of the constructor it configures.
//
// The writer is os.Stdout only so `go test` can compare the output; real programs
// should leave the default, which is stderr, because the payload stream is
// reserved for a command's final machine payload.
//
// The output below is byte-stable across runs, which is what makes this example
// verifiable rather than merely compilable: ax-go's loggers add no timestamp
// unless the caller asks for one, and with no active span the correlation IDs are
// the zero-value constants. Label field order follows the Labels struct, not the
// order they were written in the literal.
func ExampleNewLogger() {
	ctx := context.Background()

	log := logging.NewLogger(ctx,
		logging.WithLoggerWriter(os.Stdout),
		logging.WithLoggerLevel(zerolog.InfoLevel),
		logging.WithLoggerLabels(logging.Labels{
			Application: "my-cli",
			Environment: "prod",
			Version:     "v1.2.3",
		}),
	)

	// Below the configured level: never constructed, never emitted.
	log.Debug(ctx).Msg("filtered out")

	log.Info(ctx).Str("stage", "startup").Msg("ready")

	// Output:
	// {"level":"info","environment":"prod","application":"my-cli","version":"v1.2.3","stage":"startup","trace_id":"00000000000000000000000000000000","span_id":"0000000000000000","message":"ready"}
}
