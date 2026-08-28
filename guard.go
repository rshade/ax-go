package ax

import (
	"context"
	"errors"
	"net/url"
)

type auditContextKey struct{}

// WithAudit returns a context that enables or disables audit logging for real
// Guard, Perform, GuardWithAudit, and PerformWithAudit invocations made through
// it. Audit logging is enabled when no prior WithAudit call is present.
//
// WithAudit affects only the two default audit lines around a real, non-nil
// effect or commit. It does not alter callback execution, returned errors or
// their wrap chains, exit-code mapping, nil-callback handling, or the existing
// dry-run suppression line.
func WithAudit(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, auditContextKey{}, enabled)
}

// AuditEnabledFromContext reports whether audit logging is enabled for ctx.
// It returns true by default, when no prior WithAudit call is present, and
// mirrors WithAudit's subtree-propagation semantics: a WithAudit call on a
// child context overrides whatever an ancestor context set.
func AuditEnabledFromContext(ctx context.Context) bool {
	enabled, ok := ctx.Value(auditContextKey{}).(bool)
	if !ok {
		return true
	}
	return enabled
}

func guard(
	ctx context.Context,
	description string,
	effect func(context.Context) error,
) (bool, error) {
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

	return true, runAudited(ctx, "Guard", description, effect)
}

func perform(
	ctx context.Context,
	description string,
	rehearse, commit func(context.Context) error,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !DryRunFromContext(ctx) {
		if commit == nil {
			return nil
		}

		return runAudited(ctx, "Perform", description, commit)
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

// runAudited invokes fn, emitting the default audit lines around it unless
// AuditEnabledFromContext(ctx) is false. If fn panics, the deferred check logs
// an abnormal-termination line (rather than a spurious success/failure line)
// before the panic continues to unwind.
func runAudited(
	ctx context.Context,
	helper, description string,
	fn func(context.Context) error,
) error {
	if !AuditEnabledFromContext(ctx) {
		return fn(ctx)
	}
	logger := NewLogger(ctx)
	logAuditStart(ctx, logger, helper, description)
	completed := false
	defer func() {
		if !completed {
			logAuditAbnormal(ctx, logger, helper, description)
		}
	}()
	err := fn(ctx)
	completed = true
	logAuditOutcome(ctx, logger, helper, description, err)
	return err
}

func logAuditStart(ctx context.Context, logger Logger, helper, description string) {
	logger.
		Info(ctx).
		Str("ax_helper", helper).
		Str("description", description).
		Msg("ax: about to run effect")
}

func logAuditOutcome(ctx context.Context, logger Logger, helper, description string, err error) {
	if err != nil {
		// Redact *url.Error to strip embedded URLs with query-string secrets
		// before logging; preserve the full wrapped error for the caller.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		logger.
			Error(ctx).
			Str("ax_helper", helper).
			Str("description", description).
			Err(err).
			Msg("ax: effect failed")
		return
	}
	logger.
		Info(ctx).
		Str("ax_helper", helper).
		Str("description", description).
		Msg("ax: effect succeeded")
}

func logAuditAbnormal(ctx context.Context, logger Logger, helper, description string) {
	logger.
		Error(ctx).
		Str("ax_helper", helper).
		Str("description", description).
		Msg("ax: effect did not return normally")
}

// Guard runs effect unless dry-run is active in ctx. On a real run with a
// non-nil effect, it emits two structured audit lines to stderr by default:
// one before the effect and one reporting success or failure afterward. Pass a
// context derived by WithAudit(ctx, false) to suppress those real-run lines.
//
// When dry-run is active, Guard skips effect entirely, emits the existing
// single suppression line to stderr, and returns (false, nil); audit settings
// do not change that behavior. A nil effect is a silent no-op returning
// (false, nil). On a real run Guard returns (true, effect's error) without
// altering the error or its wrap chain, and maps no exit code itself.
//
// All log output goes to stderr, never stdout. A nil context is treated as
// dry-run inactive, and Guard never panics on a missing context or callback.
// The callback's returned error is logged verbatim (via a best-effort
// redaction that strips embedded request URLs) on the failed audit line;
// callers must ensure returned errors do not otherwise embed secrets, tokens,
// or credentials.
func Guard(ctx context.Context, effect func(context.Context) error) (bool, error) {
	return guard(ctx, "", effect)
}

// GuardWithAudit behaves like Guard and carries description as a structured
// field on the two default audit lines around a real, non-nil effect. Pass a
// context derived by WithAudit(ctx, false) to suppress those real-run lines.
//
// Under dry-run, GuardWithAudit skips effect and emits the same single
// suppression line as Guard; description and the audit setting do not change
// that output. A nil effect is a silent no-op. On a real run it returns
// (true, effect's error) without altering the error or its wrap chain, and maps
// no exit code itself. All log output goes to stderr, never stdout. A nil
// context is treated as dry-run inactive, and missing callbacks never panic.
// description is logged verbatim as a structured field on both audit lines —
// it MUST NOT contain PII, secrets, tokens, or credentials; prefer stable
// resource identifiers. The effect's returned error is logged verbatim (via
// best-effort redaction stripping embedded URLs) on failure; callers must
// ensure returned errors do not otherwise embed secrets or credentials.
func GuardWithAudit(
	ctx context.Context,
	description string,
	effect func(context.Context) error,
) (bool, error) {
	return guard(ctx, description, effect)
}

// Perform runs commit when dry-run is inactive, or the read-only rehearse
// preview when dry-run is active, so a dry-run surfaces the same validation
// errors as a real run without performing the mutation.
//
// On a real run with a non-nil commit, Perform ignores rehearse and emits two
// structured audit lines to stderr by default: one before commit and one
// reporting success or failure afterward. Pass a context derived by
// WithAudit(ctx, false) to suppress those real-run lines. A nil commit is a
// silent no-op.
//
// Under dry-run, Perform runs rehearse when non-nil and never runs commit. A
// successful rehearsal followed by a non-nil suppressed commit emits the
// existing single suppression line; a failed rehearsal returns its error
// without that line. Audit settings do not change dry-run behavior. Perform
// returns the selected callback's error without altering its wrap chain and
// maps no exit code itself. All log output goes to stderr, never stdout. A nil
// context is treated as dry-run inactive, and missing callbacks never panic.
// The callback's returned error is logged verbatim (via a best-effort
// redaction that strips embedded request URLs) on the failed audit line;
// callers must ensure returned errors do not otherwise embed secrets, tokens,
// or credentials.
func Perform(ctx context.Context, rehearse, commit func(context.Context) error) error {
	return perform(ctx, "", rehearse, commit)
}

// PerformWithAudit behaves like Perform and carries description as a structured
// field on the two default audit lines around a real, non-nil commit. Pass a
// context derived by WithAudit(ctx, false) to suppress those real-run lines.
//
// Under dry-run, PerformWithAudit runs rehearse when non-nil, never runs commit,
// and emits the same single suppression line as Perform after a successful
// preview when commit is non-nil; description and the audit setting do not
// change that output. A nil commit is a silent no-op on a real run. The selected
// callback's error is returned without altering its wrap chain, and no exit code
// is mapped. All log output goes to stderr, never stdout. A nil context is
// treated as dry-run inactive, and missing callbacks never panic.
// description is logged verbatim as a structured field on both audit lines —
// it MUST NOT contain PII, secrets, tokens, or credentials; prefer stable
// resource identifiers. The commit's returned error is logged verbatim (via
// best-effort redaction stripping embedded URLs) on failure; callers must
// ensure returned errors do not otherwise embed secrets or credentials.
func PerformWithAudit(
	ctx context.Context,
	description string,
	rehearse, commit func(context.Context) error,
) error {
	return perform(ctx, description, rehearse, commit)
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
