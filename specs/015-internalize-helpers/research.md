# Phase 0 Research: Certify and Internalize the Public Boundary Before v1.0

**Feature**: `015-internalize-helpers` | **Date**: 2026-07-19

This record resolves the public-surface model, audit lifecycle, release
sequencing, gate contract, and governance questions. No
`NEEDS CLARIFICATION` items remain.

## Decision Records Absorbed

None. Issue #18 names ADR-0012, but that ADR was never written. The only
remaining ADRs are ADR-0004 and ADR-0008, neither of which governs this feature.
ADRs are frozen; the boundary decision is recorded here under Constitution
Principles X–XII.

## D1: Inventory compiler-visible API through `go/types`

**Decision**: `surfacecheck` runs
`go list -deps -export -json .` from the module root for each supported target,
decodes package export paths, and loads the root package with stdlib
`go/importer.ForCompiler(..., "gc", lookup)`. It traverses the resulting
`*types.Package` rather than approximating the API from declarations.

The traversal begins at exported package-scope objects and recursively includes:

- functions, variables, constants, defined types, and aliases;
- direct and promoted exported fields;
- complete interface method sets, including embedded methods;
- value method sets and pointer-only method deltas;
- alias-visible members attributed to the root alias selector;
- unexported concrete types only when an exported declaration exposes that
  concrete type.

Returning an exported interface exposes the interface's method set, not extra
methods of a hidden dynamic implementation. `_test.go` declarations are
excluded by the compiler package load.

**Rationale**: A declaration-only AST walk misses current public fields such as
`Labels.Environment`, interface methods such as `Logger.Debug`, and aliased or
promoted selectors. It also over-counts arbitrary exported methods on hidden
receivers. Compiler export data already encodes the Go type checker's actual
selector and reachability rules and needs no new module dependency.

**Alternatives considered**:

- Raw `go/ast`: rejected for complete API discovery; retained only where source
  doc comments must be inspected.
- `golang.org/x/tools/go/packages`: rejected because the stdlib/toolchain path
  provides the required type information without a new dependency.
- Parse `go doc` or go-apidiff text: rejected because presentation/diff output
  is not a complete stable inventory format.

## D2: Canonical features and supported-target invariance

**Decision**: Each compiler-visible feature has a byte-stable canonical ID:

- `func:ParseConfig`
- `type:Labels`
- `field:Labels.Environment`
- `interface-method:Logger.Debug`
- `method:Telemetry.Shutdown`
- `method:*Telemetry.Shutdown` for pointer-only selectors

Each entry also carries a canonical type/signature string. IDs are attributed
to the public root selector even when the member is promoted or reached through
an alias. Non-identity metadata may record `direct`, `promoted`, or `alias`
origin for review. Root types are unqualified; external types use their full
import path so distinct packages with the same declared name cannot produce the
same signature. Entries sort bytewise by ID.

The host-built tool invokes child `go list` processes for
`linux|darwin|windows` × `amd64|arm64`. Every profile must yield the same
canonical ID/signature set. The tool does not set its own process GOOS to a
foreign target, which would create an unexecutable `go run` binary.

**Rationale**: Identity must remain stable while kind/signature changes remain
detectable attributes. The old `(name, kind, receiver)` identity could not also
describe “kind changed in both sets”; it necessarily produced unrelated
added/stale rows. Target invariance prevents a Windows-only or architecture-only
export from bypassing a Linux-host CI scan.

**Alternatives considered**:

- Host profile only: rejected because the repository officially cross-compiles
  six profiles.
- Per-profile baselines: rejected for now because the intended public contract
  is target-invariant; divergence should fail rather than become normalized.
- Full origin paths in identity: rejected because refactoring implementation
  origin must not masquerade as a public selector change.

## D3: Permanent audit and live baseline are separate artifacts

**Decision**:

- `specs/015-internalize-helpers/public-surface-audit.json` is the permanent
  dated decision history. Records are never deleted.
- `internal/cmd/surfacecheck/baseline.json` is the current operational
  ID/signature projection used by CI.

The gate requires every current baseline feature to map to an active audit
record or to a reviewed later addition record. Audit records in `removed`
state must be absent from source and the baseline. Audit records in
`deprecated` or `removable` state must remain present until a separate removal
change.

**Rationale**: The source issue requires a durable classification audit, while
the gate requires an exact current-state mirror. Deleting an audit row when an
export disappears destroys the reason and migration history. Keeping two
artifacts with explicit cross-validation is less ambiguous than overloading one
array with incompatible historical/current comparison rules.

**Alternatives considered**:

- Delete stale audit rows: rejected because it erases P1 evidence.
- One combined document: rejected because historical rows and live rows obey
  different completeness rules and make routine baseline review harder.
- Prose-only audit: rejected because completeness and lifecycle cannot be
  machine-validated.

## D4: Deprecate, publish, then remove in a follow-up feature

**Decision**: Feature 015 removes no exported symbol. It adds a Go-recognized
`// Deprecated:` paragraph with a replacement or removal reason to every
approved retirement candidate and lands as a non-breaking `feat:` change so
release-please produces a pre-v1 minor.

A merge is not publication. After a real `0.MINOR.0` tag carries the notice, a
follow-up Spec Kit feature may:

1. record the published notice version;
2. transition the audit row from `deprecated` to `removable`;
3. remove the root forwarder;
4. transition the row to `removed` without deleting it;
5. use `breaking-change-approved` and a `feat!:` / `BREAKING CHANGE:` commit.

At the current `v0.3.0` published baseline, notices first published in `v0.4.0`
would make `v0.5.0` the earliest possible removal release.

**Rationale**: Constitution Principle XII says an exported symbol must carry a
notice in at least one published minor and explicitly requires review to reject
early removal. Classification as accidental cannot waive that rule.

**Alternatives considered**:

- Immediate removal based on intent: rejected as a direct constitutional
  conflict.
- One PR that deprecates and removes: rejected because it cannot cross a
  published-release boundary.
- Constitution amendment for leaked helpers: not needed; compatibility
  forwarders preserve the issue's intent without weakening the universal rule.

## D5: Internalization uses compatibility-preserving root forwarders

**Decision**: Audit-approved mechanics move into a cohesive existing or narrowly
named role-specific `internal/` package. The root export becomes a temporary
deprecated forwarder with unchanged name, type, and observable semantics.
Repository call sites migrate to the replacement so SA1019 remains clean.

Every audit record must name an `internal_target` and
`compatibility_strategy`. If API identity or behavior cannot be preserved—for
example, moving a defined type would change identity—the declaration is
deprecated in place and its relocation is deferred to the follow-up removal
feature. No speculative `internal/helpers` package is allowed.

**Rationale**: Principle X locates non-public mechanics under `internal/`, while
Principles XI–XII preserve the current root contract during the notice window.
A public forwarder is compatibility facade glue, not a second implementation.

**Alternatives considered**:

- Lower-case in place: rejected because it both removes the public symbol and
  leaves audited mechanics in the root package.
- Move every type and expose an alias: rejected unless API diff proves type
  identity and semantics are preserved.
- One grab-bag internal package: rejected as non-cohesive architecture.

## D6: Deterministic stream and exit contract

**Decision**:

- Successful check: exit `0`; one minified JSON result struct on stdout;
  stderr empty.
- Inventory mode: exit `0`; one minified JSON inventory struct on stdout;
  stderr empty.
- Drift or malformed/missing/oversized repository artifacts: exit `2`;
  stdout empty; exactly one minified `ax.Error` envelope on stderr.
- Unexpected internal failure: exit `1`; stdout empty; one `ax.Error`.
- Permission failure: exit `4`; stdout empty; one `ax.Error`.

Drift details are sorted before inclusion. Flag parsing uses
`flag.ContinueOnError` with its native writer discarded so usage text never
leaks around the envelope. JSON inputs are strict, reject unknown fields and
trailing values, and are size-capped. Make suppresses recipe echo.

**Rationale**: Plain text on stdout violated Principle I, and treating expected
surface drift as exit `1` contradicted the deterministic validation mapping.
Struct-backed minified JSON makes the result pipe-safe and byte-stable.

**Alternatives considered**:

- Plain pass summary: rejected by the non-negotiable stdout contract.
- Silent success: valid but less useful than a bounded machine-readable count.
- Pretty JSON: rejected because bounded writes are strict minified JSON.

## D7: Module-root invocation and CI wiring

**Decision**: Direct usage is explicitly module-root-only:

```bash
go run ./internal/cmd/surfacecheck
make surface-check
```

From a nested directory, use:

```bash
make -C "$(git rev-parse --show-toplevel)" surface-check
```

`Makefile` adds a phony `surface-check` target with an `@`-prefixed recipe, adds
it to `ci`, and documents it in `help`. The validate job adds an explicit
surface-check step next to doc coverage.

**Rationale**: Go resolves `./internal/cmd/surfacecheck` before the program can
walk upward, so the old “run from anywhere” claim was false. A wrapper script
would add unnecessary surface.

**Alternatives considered**:

- Runtime module-root discovery as an invocation fix: rejected because it
  cannot run until Go has already resolved the relative package path.
- New shell wrapper: rejected because `make -C` is sufficient.

## D8: Package allowlist and identifier classification are distinct

**Decision**: `apidiff-verdict` remains authoritative for the public-package
allowlist: root `ax`, `config`, `contract`, `id`, `mcp`, and `schema`.
Feature 015 inventories only root `ax`. Every current root export is governed
public surface under Principle XI, even if the audit classifies it as an
implementation leak slated for deprecation.

Classification uses documented contract, facade role, examples, repository
usage, release history, and downstream evidence. Package membership and
integration-example imports do not decide identifier intent. Ambiguity resolves
to `supported`.

**Rationale**: Because every audited feature is already inside the allowlisted
root package, package membership cannot distinguish supported identifiers from
accidental exports.

**Alternatives considered**:

- Define public identifiers as what `examples/integration` imports: rejected
  because an example intentionally demonstrates only a subset.
- Treat accidental exports as never public: rejected because Go reachability
  and published compatibility policy, not intent, govern current consumers.

## D9: Persistent evidence, governance, and verification

**Decision**: Each leak record captures the search date, in-repository call
sites, indexed downstream query/evidence, earliest known published presence,
replacement, internal target, compatibility strategy, and lifecycle release
fields. Absence of downstream hits is non-authoritative; a known use favors
`supported` and never waives deprecation.

The gate implementation receives:

- table-driven complete-surface fixtures;
- strict decoder and lifecycle validation tests;
- build-profile invariance tests;
- golden success, inventory, and error-envelope tests;
- a stream/exit matrix;
- deterministic repeated-run tests;
- fuzzing for audit/baseline JSON parsing;
- failure tests for go-list/import/type errors and cancellation.

Existing runtime golden files, race tests, coverage floors, documentation
checks, lint/vet, integration compilation, and benchmark budgets remain
unchanged.

**Rationale**: The audit is a governance decision, not a usage heuristic.
Verified artifacts and explicit evidence make later removal reviewable without
pretending that code search proves absence of consumers.

**Alternatives considered**:

- Search-only classification: rejected because private and unindexed consumers
  exist.
- Numeric sub-second gate target: rejected because the tool is not a hot path
  and no benchmark justified the claim.
- New ADR: rejected because ADRs are frozen and the constitution governs the
  decision.
