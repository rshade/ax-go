# Research: success warnings

## Severity floor

Decision: `--strict` stays a boolean flag. `Warning` has `code` and
`message` only. The floor is presence. Any warning that survives the
blank-code/blank-message drop escalates when `--strict` is set.

Rejected: `--strict=<level>` and a severity enum. Issue #123 puts a
severity taxonomy out of scope, and the acceptance text gates on "a
present warning", not a rank.

## Where escalation runs

The command writes its own stdout. The runner therefore keeps a
per-execution warning list on the context and, only when `--strict` is
set, holds stdout until the command returns. Warnings present: discard
the hold and write `warnings_as_errors` to stderr. No warnings: release
the hold. A returned error still wins and the hold is discarded.

The last `WithWarnings` call in the run is the list `--strict` sees.
Streaming repeated payloads is out of scope.
