# Research: Register every error code and guard the registry

No Technical Context item was marked NEEDS CLARIFICATION. This file records
the decisions the plan rests on and the corrections to issue #285's emit-site
list that the survey turned up.

## R1. The emit-site survey

**Decision**: Convert exactly these sites. Lines are from the branch tip
`5d3125c` (spec 032 merged into the stack).

| File | Line(s) | Today | After |
| --- | --- | --- | --- |
| `execute.go` | 231, 232 | `contractErrorWarnings` (local duplicate) | `contract.ErrorCodeWarningsAsErrors` |
| `execute.go` | 355 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `execute.go` | 469 | `"internal_error"` | `contract.ErrorCodeInternalError` |
| `confirm.go` | 37 | `"confirmation_required"` | `contract.ErrorCodeConfirmationRequired` |
| `contract/error.go` | 172 | `"internal_error"` | `ErrorCodeInternalError` |
| `internal/mcpserver/dispatch.go` | 431, 510 | `"internal_error"` | `contract.ErrorCodeInternalError` |
| `internal/mcpserver/dispatch.go` | 487 | hand-built JSON with `"internal_error"` | the JSON concatenated around `contract.ErrorCodeInternalError` |
| `internal/mcpserver/dispatch.go` | 519 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `internal/mcpserver/server.go` | 85, 113, 124, 142, 154 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `internal/mcpserver/transport.go` | 52, 57 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `internal/schema/declare.go` | 107 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `internal/schema/conflict_error.go` | 28, 55 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `mcp/command.go` | 104 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `schema/schema.go` | 174 | `"validation_error"` | `contract.ErrorCodeValidationError` |
| `schema/declarations.go` | 13, 137 | `invalidDeclarationCode` (local constant) | `contract.ErrorCodeInvalidSchemaDeclaration` |

That is 19 string-literal call arguments and 2 call sites that name an
unexported local constant (spec SC-001), plus the one hand-built JSON
fallback. `config/config.go` already uses constants (spec 032).

**Corrections to the issue**: it misses `internal/schema/conflict_error.go`
(2 sites), lists 3 of the 5 `server.go` sites (at stale lines), misses the
`dispatch.go` JSON fallback, and cites `schema/schema.go:166` (actual 174) and
`schema/declarations.go:130,199,226` (one emit site, at 137, plus the
constant at 13; lines 199 and 226 do not emit a code). `execute.go:248` is the
`contractErrorWarnings` declaration, at 248 as cited.

**No missing runtime code**: the survey found no code outside spec 032's nine
runtime codes and the authoring-time `invalid_schema_declaration`. The drift
guard (R4) re-proves this mechanically on every test run.

## R2. Constant names

**Decision**: Transliterate each spelling: `ErrorCodeConfirmationRequired`,
`ErrorCodeInternalError`, `ErrorCodeValidationError`,
`ErrorCodeInvalidSchemaDeclaration`, all untyped `string` constants in
`contract/codes.go`.

**Rationale**: The six existing constants transliterate exactly
(`warnings_as_errors` → `ErrorCodeWarningsAsErrors`). Keeping that rule makes
the name derivable from the spelling and back, which is what an agent writing a
match site needs. Untyped constants assign to `Error.ErrorCode` (a `string`)
without conversion.

**Alternatives considered**: `ErrorCodeInternal` / `ErrorCodeValidation`.
Shorter, but they break the derivation rule and collide in meaning with the
`ExitInternal` / `ExitValidation` exit-code constants. Recorded as spec agent
decision 1.

## R3. Registry shape

**Decision**: In `contract/codes.go`:

- `KnownErrorCodes()` keeps its signature and returns a new slice literal of
  the nine runtime constants, in byte-wise order. Every entry is a constant;
  the three string literals left by spec 032 become
  `ErrorCodeConfirmationRequired`, `ErrorCodeInternalError`, and
  `ErrorCodeValidationError`.
- A new unexported `authoringTimeErrorCodes()` returns a new slice holding
  `ErrorCodeInvalidSchemaDeclaration`, with a comment stating spec 032
  FR-001's reason: the code is returned while the command tree is built,
  before `ax.Execute` dispatches a command, so an agent never receives it
  from a run.

**Rationale**: The runtime registry already exists as `KnownErrorCodes()`;
making it the one list avoids a second copy that could drift. A function
rather than a package-level slice keeps the package free of mutable state
(Principle X) and returns a fresh slice each call (spec 032 FR-010). The
authoring-time list stays unexported because its only reader is the
registry tests; nothing outside the package needs the set, and an export
would be new surface with no consumer.

**Alternatives considered**:

- An exported `AuthoringTimeErrorCodes()`. Rejected as unneeded surface; it
  can be added later without a break (spec agent decision 2).
- A `map[string]codeKind` registry. Rejected: it is mutable package state and
  orders nondeterministically.
- Sorting inside `KnownErrorCodes()`. Rejected: the literal is already sorted
  and the property test pins it, so a sort per call is wasted work.

`deadcode -test` counts tests as roots, so an unexported function read only by
tests is reachable and `make dead-check` stays clean.

## R4. The drift guard

**Decision**: A type-free syntactic checker in `internal/testutil`
(`errorcodes.go`), driven by two tests:

- `internal/testutil/errorcodes_test.go` runs it against a committed fixture
  tree (`internal/testutil/testdata/errorcodes/`) and asserts the exact list of
  violations, by file, line, kind, and expression.
- `contract/codes_guard_test.go` (package `contract`, so it can read the
  unexported authoring-time list) runs it against this module with the
  registry and the module's exclusion policy, and asserts zero violations.

How it works:

1. Walk the module directory. Skip `_test.go` files, directories named
   `testdata`, hidden directories, `node_modules`, any directory below the
   root that holds its own `go.mod`, and the caller's excluded directories.
   Parse every remaining `.go` file with `go/parser`, ignoring build
   constraints, so files behind `ax_no_grpc`, `ax_no_otlp`, or a platform
   suffix are checked whatever configuration the test runs under.
2. Read the registered names: parse the non-test files of
   `<module>/contract`, collect every exported constant whose value is a
   string literal, and keep those whose value is in the caller's registered
   set (runtime registry plus authoring-time list).
3. In each file, resolve the local names of the `contract` import and the
   root-package import (explicit alias, or the default `contract` / the
   configured root package name). A dot import of either is a violation.
4. Check each emit site's code expression:
   - a call `<contract>.NewError(ctx, code, ...)` or `<root>.NewError(...)`,
     or an unqualified `NewError(...)` in a file of the root or `contract`
     directory;
   - a composite-literal element `ErrorCode: code`;
   - an assignment `x.ErrorCode = code`.
   The code is accepted only when it is `<contract>.<Name>` with `<Name>`
   registered, or, inside `contract`, a bare registered `<Name>`. Anything
   else (a literal, a local constant, a variable, a concatenation, a call) is
   a violation.
5. A `NewError` reference that is not the function of a call expression
   (a function value, an argument) is a violation, because its code argument
   cannot be seen.
6. A pass-through exemption accepts a bare identifier that names the
   enclosing function's parameter, only at a listed (file, function, parameter)
   with a non-empty reason. The module lists exactly two: `error.go`
   `NewError` `code` (the root constructor forwards its caller's code) and
   `contract/error.go` `NewError` `code` (the constructor stores its caller's
   code in the `ErrorCode` field). An exemption entry that matches nothing is
   reported too, so a stale entry cannot linger.

**Rationale**: The issue's technical note is the design: a syntactic check is
complete when the rule requires the argument to be a registered constant at
the call site, because then the check never has to evaluate an expression.
`go/parser` and `go/ast` are stdlib, so the guard adds no dependency. Parsing
every file regardless of constraints closes the "code behind a build
constraint is invisible to the default toolchain" gap that `AGENTS.md` warns
about, without a 4 × 6 matrix of `go list` loads. Splitting the checker
(`internal/testutil`) from the policy (`contract`) mirrors the import-isolation
helpers: `testutil` owns mechanism, and each package test supplies the rule.
Matching "registered" by value, not by name, catches a constant that exists
but was never added to a list.

**Alternatives considered**:

- Type-aware analysis with `go/types` over `go list -export` (the `slopcheck`
  approach). Rejected: it needs a load per build configuration and platform to
  see constrained files, and resolving constants buys nothing once the rule
  forbids computed codes.
- A `go/analysis` analyzer run through `golangci-lint`. Rejected: it means a
  custom linter plugin or a new gate binary. A test in the normal suite fails
  in the same CI jobs with far less machinery.
- Checking only `NewError` call sites, as the issue's third acceptance
  criterion literally says. Rejected because composite literals and field
  assignments set the same envelope field and would be an unguarded door
  (spec agent decision 4). They cost one more case each and find nothing
  today.
- Recognizing pass-through helpers automatically (any function with a `code
  string` parameter). Rejected: it would silently accept a new helper that
  forwards an unregistered code. An explicit, reasoned list fails closed.

**Limits, stated in the checker's doc comment**: the guard is type-free. A
local variable that shadows an import name, or a type other than
`contract.Error` with an `ErrorCode` field, is matched by name. Both fail
closed (they report, they do not hide), except a shadowed import name, which
no code in this module uses.

## R5. What the guard excludes, and why

| Path | Why |
| --- | --- |
| `*_test.go` | Test expectations keep string literals so they pin the frozen spellings. |
| `testdata/` | Fixtures, including the guard's own deliberately bad fixture. |
| nested `go.mod` | A separate module (the `unreadfield` and `slopcheck` fixtures). |
| hidden dirs, `node_modules` | Not Go library code (`.git`, `.slop`, `.specify`, npm tooling). |
| `internal/cmd/` | Gate tools with their own codes, excluded by spec 032 FR-001. Their `fail(ctx, code, ...)` helpers forward a `code` parameter by design. |
| `examples/` | Adopter code; its codes are adopter-defined (spec 032 FR-001). |

`internal/cmd/` and `examples/` are the caller's policy and live in
`contract/codes_guard_test.go`, each with its reason. The other rows are the
checker's fixed behavior.

## R6. The registry consistency test

**Decision**: `contract/codes_test.go` gains a test that parses the
`contract` package's own constants (the same helper the guard uses), takes
every exported constant named `ErrorCode*`, and asserts:

- each constant's value appears in exactly one of `KnownErrorCodes()` and
  `authoringTimeErrorCodes()`;
- each entry of both lists is the value of exactly one such constant;
- the two lists are disjoint, and the authoring-time list is sorted and has no
  duplicates.

Together with the guard, adding a runtime code takes exactly one registry
edit, and forgetting either half fails (spec SC-005): a constant with no list
entry fails this test, and a code with no constant cannot pass the guard.

## R7. Test-first sequencing (Principle VII)

1. Fixture and checker test first. It fails to compile until the checker
   exists, then passes once the checker reports exactly the fixture's planted
   violations.
2. Module guard test next, against the registry as it stands. It must fail
   and list the 21 sites from R1 (plus nothing else). This is the proof that
   the guard sees the real repository.
3. Registry consistency test. It fails to compile until the four constants
   and `authoringTimeErrorCodes()` exist.
4. Add the constants and the authoring-time list, then convert the emit sites.
   The guard and consistency tests pass; every golden and envelope test passes
   unchanged.

Run with `-count=1` after a literal-to-constant swap, because `go test`
caches results for byte-identical behavior.

## R8. Gates this touches

- `make surface-check`: four `added` `contract` constant features, reviewed
  and regenerated with `make surface-update`. No root symbol is added, so the
  root audit does not change (spec 032 R4).
- `make size-check`: no new dependency in a probe's graph. `contract` gains
  only constants, and the guard is test code.
- `make dead-check`: `authoringTimeErrorCodes()` and the checker are reached
  from tests, which count as roots.
- `make slop-check`: every field of the checker's config and violation types
  is read.
- `make cover-check`: `internal/testutil` has a 25% floor and gains a tested
  helper; `contract` and the converted packages keep their coverage.
- `make bench-check`: no tracked hot path changes. The `__schema` reflection
  path copies the same nine constants. No run is required.
- `make doc-coverage`: no new primary-API symbol. The four constants carry doc
  comments (`godoclint`).
- Goldens: none change (spec FR-011).
