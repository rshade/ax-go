package ax

import (
	"bytes"
	"context"
	"io"

	"github.com/rshade/ax-go/contract"
)

// warningState is the per-Execute record of warnings and, when --strict is
// set, the held stdout. The pointer lives on the command context.
type warningState struct {
	warnings []Warning
	strict   bool
	buf      *bytes.Buffer
	real     io.Writer
	// release puts back the stdout writer --strict replaced. Execute defers
	// it so the next call on this command tree writes to the writer that call
	// installed, not to the discarded buffer.
	release func()
}

type warningStateKey struct{}

func withWarningState(ctx context.Context) context.Context {
	return context.WithValue(ctx, warningStateKey{}, &warningState{})
}

func warningStateFrom(ctx context.Context) *warningState {
	state, _ := ctx.Value(warningStateKey{}).(*warningState)
	return state
}

// restoreOut returns a command stdout writer that --strict replaced.
func (s *warningState) restoreOut() {
	if s == nil || s.release == nil {
		return
	}
	s.release()
	s.release = nil
}

// WithWarnings returns env with warnings in caller order. A blank code or
// message is dropped. When ctx belongs to ax.Execute, the kept list is what
// --strict escalates. The last call in the run replaces that list.
func WithWarnings[T any](ctx context.Context, env Envelope[T], warnings ...Warning) Envelope[T] {
	env = contract.WithWarnings(env, warnings...)
	if state := warningStateFrom(ctx); state != nil {
		state.warnings = append([]Warning(nil), env.Warnings...)
	}
	return env
}
