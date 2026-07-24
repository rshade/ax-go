package logcore

import (
	"context"
	"errors"
)

// Sink is a write-through log destination that can drain buffered entries at
// shutdown. Every sink is an io.Writer — New fans output out to all of them via
// io.MultiWriter alongside the primary writer — and exposes a context-aware
// Drain so shutdown cannot hang.
//
// Drain is exported, and the interface with it, for a language reason rather than
// a design preference: Go qualifies an unexported method name by its defining
// package, so an interface requiring a lowercase method can only ever be
// satisfied from inside logcore. The Loki direct-push writer stays in package ax
// (Constitution Principle VIII forbids coupling log shipping into the core
// logger), so it must satisfy this contract across a package boundary, and an
// unexported method makes that uncompilable.
//
// Exporting the name does not open the seam to external registration: logcore
// lives under internal/, which the toolchain forbids any other module from
// importing, and neither public logging surface re-exports Sink.
type Sink interface {
	Write(p []byte) (int, error)
	Drain(ctx context.Context) error
}

// LabelSanctioner is the optional capability by which a sink is told which label
// pairs may be promoted from a log line into stream labels.
//
// It is deliberately separate from Sink rather than folded into it. The sink seam
// must stay fully generic: a file rotator or ring-buffer sink has no label
// concept and must not be forced to implement one. New asserts the capability and
// leaves sinks without it alone, which is what keeps the core logger free of any
// knowledge of who implements it.
//
// It is exported for the same cross-package satisfaction reason as Sink.Drain.
type LabelSanctioner interface {
	SanctionLabels(labels Labels)
}

// flusher is satisfied by Logger implementations that support draining buffered
// sinks. It stays UNEXPORTED because zerologLogger lives in this package, so
// same-package satisfaction applies — this is the one place the unexported
// interface idiom still works, and it is used. Flush type-asserts against it.
type flusher interface {
	flush(ctx context.Context) error
}

// flush drains each additional sink in order, passing the caller's context so
// sinks can respect cancellation and deadlines. Drain errors are collected and
// joined, so errors.Is finds each one and a failing sink never short-circuits the
// sinks registered after it.
//
// Exit code mapping: sink drain errors are surfaced to Flush callers but do not
// map to a CLI exit code.
func (l zerologLogger) flush(ctx context.Context) error {
	var errs []error
	for _, s := range l.sinks {
		if err := s.Drain(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Flush performs a best-effort, non-destructive drain of any buffered entries
// held by l's sinks. It returns nil — performing no work — when l is nil or when
// l holds no drainable sinks, so a caller never has to test before calling.
//
// Errors from individual sinks are joined; errors.Is against a specific sink's
// error works on the result. A drain failure is reported to the caller but must
// never change the process exit code.
func Flush(ctx context.Context, l Logger) error {
	if l == nil {
		return nil
	}
	f, ok := l.(flusher)
	if !ok {
		return nil
	}
	return f.flush(ctx)
}
