# Phase 0 Research: axtest — Full-Lifecycle Command Test Helper

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

No `[NEEDS CLARIFICATION]` markers were carried out of the spec, so this phase
resolves *design* unknowns instead of ambiguity: how the helper fits the
existing `Execute` lifecycle, whether it should be gated the same way as the
project's other public packages, and how to enforce that it is genuinely
test-only. Each decision is grounded by reading the current source, not
assumption.

## Decision: package location and name

**Decision**: A new public package `axtest` at the module root
(`github.com/rshade/ax-go/axtest`), joining `config`, `contract`, `id`,
`logging`, `mcp`, and `schema`.

**Rationale**: The issue names this package explicitly, and it matches
Principle X (public packages live at the module root, no `pkg/`/`src/`).
Placing it at the root — rather than under `internal/` with only a
re-exported shim, or bolted onto an existing package — gives it the same
discoverability as every other officially supported surface.

**Alternatives considered**:

- *Add helpers to the root `ax` package itself.* Rejected: it would pull
  `testing` into every consumer of root `ax`, including production binaries,
  which is precisely the "boundary the toolchain should enforce" this feature
  needs (see the non-test-import decision below). A production import of
  `ax` must never risk vendoring the `testing` package into a shipped binary.
- *Nest it as `internal/axtest` with a public re-export.* Rejected: adds a
  layer with no isolation benefit (there is nothing size-sensitive to hide;
  see the next decision) and breaks the pattern every other public package
  uses of internal-implementation-behind-a-thin-package only when there is a
  real reason to keep something behind `internal/` (a swappable backend, in
  `logging`'s case). Here there is nothing to keep out.

## Decision: this is organizational isolation, not size isolation

**Decision**: `axtest` is not import-isolation in the `contract`/`config`/
`schema`/`id`/`logging` sense. It freely depends on the full root `ax`
package (to call `Execute`), Cobra (`*cobra.Command` is its input type), and
the standard `testing` package.

**Rationale**: Read `execute.go`: `Execute(ctx, root, opts...)` is the only
place `--dry-run`, `--yes`, `--format`, and `--idempotency-key` get mounted
(`prepareCommand`, `execute.go:161`), and `ax.Envelope[T]` is a type alias for
`contract.Envelope[T]` (`json.go:14`). To exercise the real lifecycle, `axtest`
must call the real `ax.Execute` — there is no lighter dependency that
reproduces the same wiring. Because `axtest` is (by convention, and soon by
an enforced check — see below) never linked into a production binary, none of
the size motivation behind the other import-isolated packages applies. What
`axtest` isolates is discoverability and a stable place to find test tooling,
not bytes.

**Alternatives considered**:

- *Reimplement flag-mounting independently of `Execute`* so `axtest` could
  depend on a lighter subset. Rejected: this is exactly the "one-off
  discovery cost" the issue describes, just moved into the library instead of
  eliminated, and it risks drifting from `Execute`'s real behavior over time
  (two independent implementations of the same wiring).

## Decision: enforce test-only reachability with an import-direction check, not a naming convention alone

**Decision**: A new test asserts that no non-test (`.go`, not `_test.go`)
source file anywhere else in this module imports `axtest`, by checking `go
list -json`'s per-package `Imports` field (which reflects only a package's
non-test compilation unit) rather than `TestImports`/`XTestImports` (which
capture `_test.go` files, including external `_test` packages). A production
leak — someone typing `import "github.com/rshade/ax-go/axtest"` in a `.go`
file — surfaces in `Imports`; legitimate use in a `_test.go` file never does,
because Go itself keeps that distinction in the tool's own output. The check
runs over the cross product of the four build-tag configurations returned by
a local `buildConfigurations()` helper and the six supported GOOS/GOARCH
targets returned by `buildProfiles()` — the same 4 × 6 matrix used by
`internal/cmd/surfacecheck`, not one host-only, untagged `go list -json ./...`
invocation.

**Rationale**: The Go standard library's precedent (`net/http/httptest`,
`io/fs/fstest`) documents "test-only" but does not mechanically enforce it —
nothing stops a `.go` file from importing `httptest` and it will compile,
silently vendoring the `testing` package into a shipped binary. This project's
existing convention is stricter than the stdlib's on exactly this class of
risk (`mcp/import_isolation_test.go`'s `TestContractPackagesDoNotImportMCP`
already asserts a directional import restriction between two public
packages), so leaving `axtest` to documentation-only enforcement would be a
regression from that standard, not parity with it. This satisfies spec
FR-009 and Edge Case coverage for accidental production linkage with no new
dependency: one `go list -json ./...` invocation per configuration/profile
combination.

Scanning every configuration and profile, not just the default host build, is
not optional defense-in-depth here — AGENTS.md states the underlying risk
explicitly:
"Code behind a build constraint is invisible to the default toolchain...
Neither `go vet ./...` nor `go test -race ./...` passes tags... A green
default run does NOT cover the declined configurations." A non-test file
gated behind `//go:build ax_no_grpc` (or `ax_no_otlp`) that imported
`axtest` would compile cleanly and be invisible to a single untagged `go
list -json ./...`; a `_windows.go` or `_arm64.go` file would be equally
invisible on a linux/amd64 host. Either defeats FR-009's "no non-test source
file **anywhere in this module**" guarantee. `ResolveModulePackages` and
`AssertNoProductionImport` therefore take a `BuildProfile` plus variadic
`tags ...string`, and `TestAxtestIsOnlyImportedFromTests` loops over all 24
supported combinations.

**Alternatives considered**:

- *Rely on documentation alone* (the `httptest` precedent). Rejected per
  above — this project already holds a higher bar for its public packages.
- *Check `axtest`'s own transitive dependency graph for forbidden imports*,
  the pattern every existing `testutil.AssertNoForbiddenImports` use follows.
  Rejected: that pattern answers "does `axtest` depend on something it
  shouldn't", which is backwards for this feature — `axtest` is *supposed* to
  depend on the full root `ax`. The actual risk runs the other direction (does
  something else depend on `axtest` from production code), which needs the
  reverse check.
- *A golangci-lint `depguard` rule* forbidding non-test files from importing
  `axtest`. Rejected as a second, redundant enforcement mechanism for the same
  guarantee a plain Go test can assert with no new lint configuration; kept in
  mind as a future defense-in-depth addition, not required here.
- *Scan only the default host configuration*, on the theory that `axtest`
  itself has no conditional code. Rejected: the risk this check guards against
  is a leak in *someone else's* file, and that file can carry a build tag,
  GOOS suffix, GOARCH suffix, or `//go:build` expression `axtest` itself never
  uses. The existing 4 × 6 matrix is precisely the tool this project already
  uses for this blind spot; not applying it here would leave FR-009 dependent
  on the CI host.

## Decision: `axtest` joins the gated public-API surface (8th package)

**Decision**: `axtest` is added to `internal/cmd/surfacecheck`'s
`PublicPackages()`, `internal/cmd/apidiff-verdict`'s `allowedPackages()`, and
`internal/cmd/doccover`'s `scannedPackages()`/`requiredSymbols()`, on equal
footing with the other six public packages. It also receives its own explicit
per-package floor entry in `internal/cmd/covercheck`, calibrated after tests
are written (per the existing "Raising a Floor" procedure, applied here as
the initial-floor case — implement, measure, set ~2pp below measured, as was
done for `internal/cli`/`internal/mcp`/`internal/schema` on 2026-07-17).

**Rationale**: `axtest.Run`, `axtest.Decode`, and `axtest.RunAndDecode` are
exported Go identifiers that every adopting project's test suite will call
directly. A silent rename or signature change breaks those test suites exactly
the way a root `ax` break would break production code — Principle XI's
concern (a consumer depending only on the public surface) does not
distinguish "public surface used from test code" from "public surface used
from production code." Every other package at the module root is already
gated this way; carving `axtest` out because of *how* it is used, rather than
*whether* it is public, would be an inconsistent, unprincipled exception. This
directly satisfies spec SC-005 (parity with existing public packages on first
release).

**Alternatives considered**:

- *Leave `axtest` ungated*, on the theory that test-code breakage is lower
  stakes than production breakage. Rejected: a downstream project's CI going
  red because `axtest.Run`'s signature silently changed is exactly the kind
  of surprise the apidiff/surfacecheck/doccover gates exist to catch before
  release, and it directly contradicts this feature's own motivation (reduce
  rediscovery cost for consumers).

**Consequence recorded for tasks.md**: every comment and doc line that
currently says "seven public packages" (`internal/cmd/surfacecheck/main.go`
package doc, `AGENTS.md`'s Public Surface Gate section, `README.md`) becomes
"eight." The 24-load formula itself (4 build-tag configurations × 6
GOOS/GOARCH profiles) is unaffected — AGENTS.md is explicit that the load
count is fixed regardless of package count, since a load evaluates every
requested package together.

## Decision: `Result` struct, not a bare multi-value return

**Decision**: The execution helper returns a single struct:

```text
Result struct {
    Stdout   []byte
    Stderr   []byte
    ExitCode int
}
```

instead of the issue's illustrative `(stdout []byte, exitCode int)` pair.

**Rationale**: Spec FR-002 requires stdout, stderr, and exit code as three
independently inspectable results — stderr capture was added on top of the
issue's literal proposal because `ax.Error` envelopes for blocked
confirmations and validation failures are written to the diagnostic stream
(Principle I), and Acceptance Scenario 2 of User Story 1 (a blocked
confirmation) is untestable without it: the machine-payload stream is empty
on that path by design. A three-value return is exactly the kind of thing
that gets silently transposed at a call site (`stdout, stderr, code :=
axtest.Run(...)` vs. the reverse); a named-field struct removes the ambiguity
and matches the spec's own "Execution result" entity, which frames the three
values as one thing considered together, not three loose returns.

**Alternatives considered**:

- *Match the issue's literal 2-value return, stdout + exit code only.*
  Rejected: leaves User Story 1's confirmation-blocked scenario (and any
  future test asserting on an `ax.Error` envelope's shape) with no supported
  way to reach the diagnostic output at all — a caller would have to fall
  back to `ax.WithStderr` boilerplate, reintroducing exactly the friction this
  feature exists to remove, just for the failure path instead of the success
  path.
- *Three-value return `(stdout, stderr []byte, exitCode int)`.* Rejected in
  favor of the struct for the transposition-safety reason above; Go idiom
  (e.g., `httptest.NewRecorder()`'s `Result()`) already favors a named result
  type for a multi-field HTTP-style outcome.

`RunAndDecode` stays a 2-value return, `(T, int)`, matching the issue exactly:
it is documented as the happy-path convenience helper (see Assumptions in
spec.md), where only the typed value and the exit code matter.

## Decision: `Run` and `RunAndDecode` take `ctx context.Context` as their first parameter

**Decision**: `Run(ctx context.Context, t testing.TB, root *cobra.Command,
args []string, opts ...ax.ExecuteOption) Result` and the equivalent
`RunAndDecode[T any](ctx context.Context, t testing.TB, ...)` both accept
`ctx` first and forward it unmodified to `ax.Execute(ctx, root, opts...)`.
`Decode[T any](t testing.TB, stdout []byte) T` does **not** gain a `ctx`
parameter.

**Rationale**: Constitution Principle X states `context.Context` "MUST be
the first parameter of any function doing I/O, making outbound calls,
running goroutines, or otherwise cancelable," with no carve-out for test
code. `Run` performs I/O — it executes an arbitrary command tree through
`ax.Execute`, whose own first parameter is `ctx context.Context`
(`execute.go:96`), and that command may itself do network or filesystem I/O.
Dropping the parameter and hardcoding `context.Background()` internally
would silently discard the caller's ability to time out or cancel a hung
command under test, and would be inconsistent with this module's own
established pattern: `internal/testutil.AssertNoForbiddenImports` and every
function built on it (`AssertLoggingSurfaceIsolated`,
`AssertContractPackageIsolated`, and this feature's own
`AssertNoProductionImport`) already put `ctx context.Context` before
`t testing.TB`, precisely because they too do I/O (a `go list` subprocess).
`Decode` does no I/O — it unmarshals an already-in-memory `[]byte` — so
Principle X's rule does not apply to it, and adding an unused `ctx`
parameter would be `any`-style boilerplate with no behavior behind it.

**Alternatives considered**:

- *Hardcode `context.Background()` inside `Run`, as the issue's illustrative
  signature implied and this contract's first draft specified.* Rejected:
  this is the exact Principle X conflict this decision exists to close, and
  a hardcoded background context can never be canceled or given a deadline
  by the caller — a real gap for a command under test that hangs.
- *Give `Decode` a `ctx` parameter too, for signature symmetry with `Run`.*
  Rejected: `Decode` is pure computation over bytes already in memory; adding
  a parameter with no effect on behavior would be exactly the kind of
  "convenience overload without context" AGENTS.md's Go Discipline section
  already tells agents not to add, just inverted (an argument nothing uses,
  instead of an overload that skips one).

## Decision: `Decode` fails the test immediately via `testing.TB`

**Decision**: `Decode[T any](t testing.TB, stdout []byte) T` calls
`t.Helper()` then `t.Fatalf` on an unmarshal error, returning the zero value
of `T` only in the sense that the calling goroutine never observes it (`
Fatalf` calls `FailNow`, which exits the goroutine). No `error` is returned
alongside `T`.

**Rationale**: Matches the issue's proposed signature exactly, and is the
standard idiom for Go test helpers that accept `testing.TB` (they fail the
test directly rather than pushing error handling back onto every call site).
This directly satisfies FR-006's "fail immediately and with a clear cause."

**Alternatives considered**: A `(T, error)` return was considered and
rejected — it would force every call site to handle an error that, per FR-006,
should always terminate the test immediately anyway, reintroducing the
boilerplate this helper exists to remove.

## Decision: flag re-mounting safety is inherited for free

**Decision**: No new mounting logic is needed for FR-008 (safe re-mounting on
a reused command tree). `axtest.Run` calls `ax.Execute` unmodified, and
`prepareCommand` already mounts flags through `cli.EnsurePersistentStringFlag`
/ `cli.EnsurePersistentBoolFlag` (`execute.go:158-165`), which check for an
existing flag before registering one. This is verified by a test that calls
`Run` twice against the same root command, not implemented by new code.

**Rationale**: Avoids duplicating logic `Execute` already gets right, and
keeps `axtest.Run` a thin, honest wrapper rather than a second
implementation of startup wiring that could drift from the real one.

## Decision: documentation and examples

**Decision**: Two example surfaces, not one:

1. `axtest/example_test.go` — a self-contained `ExampleRun` /
   `ExampleRunAndDecode` against a tiny toy command tree, satisfying
   `doccover`'s gated-primary-API requirement (Constitution Principle VII)
   the same way `logging/example_test.go`'s `ExampleNewLogger` does for that
   package.
2. A new test file under `examples/integration/` exercising the *real*
   reference command through `axtest`, satisfying spec FR-010's "canonical
   testing pattern" narrative with a non-toy example a reader can compare
   against the reference CLI's own `main.go`.

**Rationale**: A synthetic toy example proves the API compiles and runs; a
second example against the shipped reference CLI proves the pattern actually
replaces what a real project would otherwise hand-roll (the two-friction
problem statement in the spec). Both are cheap and each serves a distinct
audience: `doccover`'s automated gate, and a human comparing against a real
command tree.

## Decision: no benchmark, no build-tag-matrix work

**Decision**: `axtest` adds no entry to `internal/cmd/benchcheck`'s tracked
benchmarks and requires no build-tag-specific code.

**Rationale**: The spec makes no performance claim (Out of Scope), and
`axtest` contains no branch conditional on `ax_no_grpc`/`ax_no_otlp` — it
calls `ax.Execute` however that resolves under whichever tags a consumer's
own test binary is built with. `make test`/`make validate`'s existing
four-configuration matrix already covers it with no feature-specific
addition.

## ADR governance

No ADR in `docs/adr/` governs test tooling, execution-lifecycle testing, or
envelope decoding. None is absorbed or retired by this feature.
