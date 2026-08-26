# Contract: `axtest` public package

**Feature**: [spec.md](../spec.md) | **Plan**: [plan.md](../plan.md) |
**Research**: [research.md](../research.md) | **Data model**: [data-model.md](../data-model.md)

This is the library-API contract for the new public package
`github.com/rshade/ax-go/axtest`, in the sense the plan template calls for
("public APIs for libraries"). It fixes shapes and behavior; task-level
implementation detail (file layout, internal helpers) belongs to
`tasks.md`.

Import-only-from-tests convention: this package is designed to be imported
from `_test.go` files. Nothing in the Go toolchain prevents importing it from
a `.go` file, which is exactly the gap FR-009 closes with an automated check
(research.md) rather than relying on convention alone.

## Types

### `Result`

```text
type Result struct {
    Stdout   []byte
    Stderr   []byte
    ExitCode int
}
```

The full outcome of one `Run` call. See data-model.md's Execution Result
entity for field-level invariants. Zero value is a valid, empty result and is
never produced by `Run` itself (`Run` always populates all three fields from
a real `ax.Execute` call).

## Functions

### `Run`

```text
func Run(
    ctx context.Context,
    t testing.TB,
    root *cobra.Command,
    args []string,
    opts ...ax.ExecuteOption,
) Result
```

**Behavior contract**:

- Sets `root`'s arguments to `args` and executes it through
  `ax.Execute(ctx, root, ...)`, capturing stdout and stderr internally
  regardless of whether the caller also supplied
  `ax.WithStdout`/`ax.WithStderr` in `opts`.
- If the caller supplies `ax.WithStdout` or `ax.WithStderr` in `opts`,
  `Run`'s own capture takes precedence for the returned `Result` — a caller
  should not need to supply either; they exist on `ax.ExecuteOption` for
  production use, not for overriding what `Run` returns. (Documented
  behavior, not silently ignored: doc comment states this explicitly.)
- Never calls `t.Fatal`/`t.Fatalf`/`t.Error` itself. A non-zero `ExitCode` is
  returned, not treated as a `Run`-level failure — the caller decides whether
  that outcome is expected (spec Edge Cases: a validation failure must not
  abort the test process).
- Calls `t.Helper()` so failure locations (if the caller subsequently fails
  the test) attribute to the caller's line, not into `axtest`.
- Safe to call more than once against the same `root` within one test
  (spec FR-008); does not reset flag values a previous call set — that is
  ordinary command-framework behavior, not `axtest`'s concern (see
  data-model.md's Execution Result lifecycle note).
- `ctx` is forwarded to `ax.Execute` unmodified — `Run` performs I/O (it
  executes an arbitrary command tree, which may itself do network or
  filesystem I/O) and Constitution Principle X requires `context.Context` as
  the first parameter of any such function, with no test-code exception. A
  caller with no specific context need passes `context.Background()`
  explicitly, matching the pattern this module's own
  `internal/testutil.AssertNoForbiddenImports` and its callers
  (`AssertLoggingSurfaceIsolated`, `AssertContractPackageIsolated`) already
  use for test helpers that do I/O.
- Safe to call concurrently from parallel subtests, each against its own
  `root` tree (spec Edge Case "multiple subtests run concurrently"): `Run`
  holds no package-level or shared state between calls.

**Acceptance mapping**: User Story 1, all five acceptance scenarios; Edge
Cases "dry run combined with confirmation-gated command," "concurrent
subtests," "command tree reused across runs."

### `Decode`

```text
func Decode[T any](t testing.TB, stdout []byte) T
```

**Behavior contract**:

- Unmarshals `stdout` as `ax.Envelope[T]` and returns its `Data` field.
- Calls `t.Helper()` then `t.Fatalf` (never returns an `error`) when `stdout`
  is empty, is not valid JSON, or does not conform to the envelope shape —
  per FR-006, this must happen immediately and with a message that names the
  cause (parse error or shape mismatch), not merely "decode failed."
- Independent of `Run`: accepts any `[]byte` in the right shape, not only a
  `Result.Stdout` value. This is what lets a caller decode output captured
  by other means without a second helper.

**Acceptance mapping**: User Story 2, both acceptance scenarios.

### `RunAndDecode`

```text
func RunAndDecode[T any](
    ctx context.Context,
    t testing.TB,
    root *cobra.Command,
    args []string,
    opts ...ax.ExecuteOption,
) (T, int)
```

**Behavior contract**:

- Calls `t.Helper()` first, then is equivalent to `result := Run(ctx, t,
  root, args, opts...); return Decode[T](t, result.Stdout), result.ExitCode`
  — no independent logic, documented as composing the two helpers above so
  its behavior can never drift from theirs.
- The `t.Helper()` call is required, not cosmetic: without it, a `Decode`
  failure raised through this composition would have its `t.Fatalf` location
  misattributed to the line inside `RunAndDecode` that calls `Decode`,
  rather than to the caller's test line. `testing.Helper()` walks the call
  stack upward from the failure site, skipping only *consecutively*-marked
  frames, and stops at the first frame that never called `t.Helper()` — so
  every intermediate wrapper in a call chain must mark itself, not just the
  innermost one, or the walk stops one frame too early.
- `ctx` is forwarded to `Run` unmodified — see `Run`'s contract for why this
  parameter exists.
- Documented (doc comment, not enforced by a runtime check) as intended for
  the success path: a caller expecting a non-zero exit code should use `Run`
  directly, since `Decode`'s `t.Fatalf` on a failure-shaped payload would
  obscure the exit code the caller actually wanted to assert on.

**Acceptance mapping**: User Story 3's acceptance scenario.

## Non-goals for this contract

- No assertion/matcher helpers (`RequireExitCode`, `RequireDataEqual`, etc.)
  are part of this contract — a caller uses their own assertion library on
  the returned `Result`/`T` (spec Out of Scope).
- No helper decodes an `ax.Error` failure envelope into a typed value. `Decode`
  targets the success envelope's `Data` field only; a caller asserting on a
  failure inspects `Result.Stderr` (raw bytes, or their own JSON unmarshal
  into `ax.Error`) directly. `ax.Error` is not wrapped in a `data` envelope in
  the first place, so `Decode`'s wrapper-avoidance problem does not exist for
  it.
