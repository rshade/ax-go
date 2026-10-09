# Data Model: Unread struct-field gate (slopcheck)

All types live in `internal/unreadfield` unless noted. None is exported from
the module. "Exported" below means exported from the internal package so the
two front ends can use it.

## FieldKey

Identity of one candidate field, stable across build configurations.

| Field | Type | Rule |
| --- | --- | --- |
| `Package` | `string` | Unit key: the import path with any `[p.test]` variant suffix removed; an external test package keeps its `_test` suffix |
| `Type` | `string` | Declared type name for a package-level type; `name@<base filename>:line:col` for a type declared inside a function (two functions may each declare `row`); `struct{...}@<base filename>:line:col` for an anonymous struct type. Base filename only: `Package` already names the directory, so the key carries no machine path (FR-010) |
| `Field` | `string` | Field name |

Ordering: lexicographic by `Package`, `Type`, `Field`. The type is comparable,
so it serves directly as a map key.

## Position

| Field | Type | Rule |
| --- | --- | --- |
| `File` | `string` | Module-relative, slash-separated; physical, so `//line` directives are ignored |
| `Line` | `int` | ≥ 1 |
| `Col` | `int` | ≥ 1 |

## Finding

| Field | Type | Rule |
| --- | --- | --- |
| `Key` | `FieldKey` | — |
| `Declared` | `Position` | The field's declaration |
| `Assigned` | `[]Position` | Every assigning literal, sorted, deduplicated, non-empty |

Rendered for the envelope cause as:

```text
<Declared>: <Package>.<Type>.<Field> (assigned at <pos>, <pos>)
```

## Result (one unit, one configuration)

| Field | Type | Rule |
| --- | --- | --- |
| `Assigned` | `[]FieldKey` | Every candidate field with at least one assignment site, sorted |
| `Findings` | `[]Finding` | The subset of `Assigned` with no read site, sorted by `Key` |

Invariant: every `Findings[i].Key` is in `Assigned`.

## Report (whole run)

Produced by `Run`. It is the intersection across configurations (research R3).

| Field | Type | Rule |
| --- | --- | --- |
| `Configurations` | `int` | Always 4 for the gate |
| `Packages` | `int` | Distinct unit keys analyzed in any configuration |
| `Fields` | `int` | Distinct `FieldKey`s assigned in any configuration |
| `Findings` | `[]Finding` | Keys that are findings in every configuration whose `Assigned` contains them, sorted. `Assigned` positions are the union across configurations. |

## Configuration

| Field | Type | Rule |
| --- | --- | --- |
| `Name` | `string` | `default`, `ax_no_grpc`, `ax_no_otlp`, `ax_no_grpc+ax_no_otlp` |
| `Tags` | `[]string` | Passed to `go list -tags` |

Supplied by `unreadfield.BuildConfigurations()` (the core owns the policy, so
the gate and `Run`'s tests share it) and kept in sync with the Makefile
`BUILD_TAG_MATRIX` by one core test.

## Classification state (internal to `Analyze`)

- `candidates map[*types.Var]candidate`: origin field → key, declaration
  position, scope verdict.
- `assigned map[*types.Var][]token.Pos`: every literal position.
- `read map[*types.Var]bool`: set by the R4 load classification and the R5
  whole-value rule.

The algorithm keeps no state between `Analyze` calls and no package-level
state.

## Gate pass document (`internal/cmd/slopcheck`)

```go
type result struct {
    Status         string `json:"status"`          // always "pass"
    Configurations int    `json:"configurations"`
    Packages       int    `json:"packages"`
    Fields         int    `json:"fields"`
    Unread         int    `json:"unread"`          // always 0 on a pass
}
```
