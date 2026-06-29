package ax

import "context"

// Guard runs effect unless dry-run is active in ctx.
//
// When dry-run is inactive it executes effect and returns (true, effect's
// error), preserving the error's wrap chain so errors.Is/errors.As keep
// working. When dry-run is active it skips effect entirely — guaranteeing no
// side effect — emits a single suppression line to stderr (only when effect is
// non-nil), and returns (false, nil). A nil effect is a no-op returning
// (false, nil); Guard never panics on a missing callback.
//
// Guard maps no exit code itself: it returns effect's error verbatim for the
// caller to map via ErrorExitCode. The suppression line is written to stderr
// (never stdout) via the canonical logger, so stdout payload determinism is
// unaffected. A nil context is treated as dry-run inactive (the real path runs);
// Guard never panics on a missing context.
func Guard(ctx context.Context, effect func(context.Context) error) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if DryRunFromContext(ctx) {
		if effect != nil {
			logDryRunSkip(ctx, "Guard")
		}
		return false, nil
	}
	if effect == nil {
		return false, nil
	}
	return true, effect(ctx)
}

// Perform runs commit when dry-run is inactive, or the read-only rehearse
// preview when dry-run is active, so a dry-run surfaces the same validation
// errors as a real run without performing the mutation.
//
// Real run: commit is executed (rehearse is ignored) and its error is returned
// with its wrap chain intact; a nil commit is a no-op returning nil. Dry-run:
// rehearse is executed (when non-nil) and its error is returned; commit is
// never executed. When the dry-run preview succeeds (or rehearse is nil) and a
// real commit would have run (commit is non-nil), a single suppression line is
// written to stderr. A failed rehearsal returns its error WITHOUT a suppression
// line, since the command already surfaces that error. A nil rehearse means a
// pure skip, equivalent to Guard.
//
// Perform maps no exit code itself: it returns the running branch's error
// verbatim for the caller to map via ErrorExitCode. The suppression line is
// written to stderr (never stdout), so stdout payload determinism is unaffected.
// A nil context is treated as dry-run inactive (the real path runs); Perform
// never panics on a missing context.
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !DryRunFromContext(ctx) {
		if commit == nil {
			return nil
		}
		return commit(ctx)
	}
	if rehearse != nil {
		if err := rehearse(ctx); err != nil {
			return err
		}
	}
	if commit != nil {
		logDryRunSkip(ctx, "Perform")
	}
	return nil
}

// logDryRunSkip emits the single structured suppression line to stderr via the
// canonical logger when a helper skips a side effect under dry-run. The message
// is a constant and every variable goes through a ZeroLog field method, so no
// user-controlled string is formatted into the line (no log forging, no PII).
// trace_id/span_id are added by the logger's tracing hook.
func logDryRunSkip(ctx context.Context, helper string) {
	NewLogger(ctx).
		Info(ctx).
		Bool("dry_run", true).
		Str("ax_helper", helper).
		Msg("dry-run: side effect suppressed")
}
