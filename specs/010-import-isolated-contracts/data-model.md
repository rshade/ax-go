# Data Model: Import-Isolated Contracts

**Feature branch**: `010-import-isolated-contracts`
**Date**: 2026-06-21

## Entity: Contract Surface

Represents one public import surface that thin consumers can use without the root runtime facade.

**Fields**

- `import_path`: Public module-relative import path.
- `package_name`: Go package identifier.
- `purpose`: The contract area the package owns.
- `allowed_dependencies`: Dependency categories the package may import.
- `forbidden_dependencies`: Runtime adapter dependencies that must never appear.
- `root_facade_symbols`: Existing root `ax` symbols that remain compatibility wrappers or aliases.
- `verification`: Tests and examples that prove the surface works and stays isolated.

**Validation Rules**

- `import_path` must be outside `internal/`, `cmd/`, `pkg/`, and `src/`.
- A contract surface must not import root `github.com/rshade/ax-go`.
- A contract surface must not import any dependency listed in the import-isolation contract's forbidden set.
- Every exported symbol must have a contract doc comment and relevant examples per doc-coverage policy.

## Entity: Root Facade Mapping

Represents the compatibility relationship between existing root symbols and new package surfaces.

**Fields**

- `root_symbol`: Existing public root-package identifier.
- `target_surface`: Contract surface that owns the reusable behavior.
- `compatibility_requirement`: Behavior and output shape that must remain unchanged.
- `migration_note`: Documentation text explaining optional direct subpackage import.
- `deprecation_state`: Always `live` for this feature.

**Validation Rules**

- No mapping may set `deprecation_state` to `deprecated` or `removed` in this feature.
- Any behavior difference between root and subpackage must be documented and tested.
- Root wrappers must preserve existing golden file behavior for `ax.Error` and `__schema`.

## Entity: Boundary Rule

Represents an import-isolation constraint enforced by tests.

**Fields**

- `surface`: Contract surface being checked.
- `forbidden_pattern`: Exact import path or path prefix that is disallowed.
- `reason`: Why the dependency would violate thin-consumer isolation.
- `test_location`: Test file responsible for enforcing the rule.

**Validation Rules**

- Every contract surface must have at least one boundary check.
- Boundary checks must inspect transitive package dependencies, not only direct imports.
- A failed boundary check must name the forbidden dependency and the surface that imported it.

## Entity: Machine Contract Shape

Represents JSON payloads and metadata shapes that agents parse.

**Fields**

- `shape_name`: Contracted JSON shape, such as error envelope, success envelope, ax schema, or MCP schema.
- `required_fields`: Fields consumers can rely on.
- `optional_fields`: Additive fields consumers must tolerate.
- `schema_version`: Version string governing compatible evolution where applicable.
- `golden_fixture`: Fixture or test that pins the shape.

**Validation Rules**

- Required field removal, rename, re-type, or semantic change is breaking.
- Additive fields are allowed when documented and tolerated by existing consumers.
- Shapes must use structs for deterministic marshaling order.

## Entity: Decision Record Absorption

Represents the legacy ADR content absorbed into this feature's research.

**Fields**

- `adr_path`: Legacy ADR file path.
- `decision`: Decision carried forward.
- `alternatives`: Alternatives considered by the ADR.
- `consequences`: Consequences retained by this feature.
- `retirement_required`: Whether the feature's final tasks must delete the ADR after absorption.

**Validation Rules**

- Every governing ADR listed in `plan.md` must have a corresponding absorbed record in `research.md`.
- ADR files must not be deleted before research absorption is complete.
- `tasks.md` must include final retirement tasks for absorbed governing ADRs.
