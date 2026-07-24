// Command rootlogging is the root-facade counterpart of examples/logging, and the
// denominator of the binary-size reduction ratio internal/cmd/sizecheck enforces.
//
// It is byte-for-byte the same program as examples/logging with
// logging.NewLogger swapped for ax.NewLogger, so the only variable between the
// two measured binaries is which surface was imported. A reviewer comparing the
// two main.go files should see one changed import and one changed call, and
// nothing else — anything more makes the ratio measure something other than the
// import boundary.
//
// It is committed rather than synthesised into a temporary module at measurement
// time because an in-module program builds against this repository's own go.mod:
// no network, no replace stanza, and the comparison stays reviewable in git diff.
package main

import (
	"context"

	ax "github.com/rshade/ax-go"
)

// Label values the program declares. They are constants so the command and its
// tests agree by construction rather than by two copies of a string literal.
const (
	appName = "example-logging"
	envName = "dev"
)

func main() {
	ctx := context.Background()

	log := ax.NewLogger(ctx,
		ax.WithLoggerLabels(ax.Labels{
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
func emit(ctx context.Context, log ax.Logger) {
	log.Info(ctx).Str("stage", "startup").Msg("ready")
}
