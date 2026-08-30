# Contract: `surfacecheck` CLI, Streams, and Exit Codes

**Tool**: `go run ./internal/cmd/surfacecheck`  
**Local alias**: `make surface-check`  
**Invocation directory**: module root

`surfacecheck` is internal maintainer/CI tooling. It scans the complete
compiler-visible root surface for all supported target profiles, compares the
live baseline, and cross-validates the permanent audit.

## Modes and Flags

| Invocation | Behavior |
|------------|----------|
| `surfacecheck` | Check current source against the default live baseline and permanent audit. |
| `surfacecheck -list` | Print the generated live-baseline candidate as minified JSON; does not read or write the current baseline. |
| `surfacecheck -audit-seed` | Print an audit-shaped seed with identity/signature fields populated and decision fields empty; the seed is intentionally invalid until reviewed and classified. |
| `surfacecheck -baseline <path>` | Check against an alternate baseline; primarily tests/review. |
| `surfacecheck -audit <path>` | Check against an alternate audit; primarily tests/review. |

The default paths are resolved from the module root. Unknown flags and
positional arguments are validation errors. Flag parser output is discarded so
only the standard error envelope reaches stderr.

Direct `go run ./internal/...` and `make surface-check` are module-root
commands. From a nested directory:

```bash
make -C "$(git rev-parse --show-toplevel)" surface-check
```

## Successful Check

Exit `0`; stderr empty; stdout contains exactly one minified JSON object:

```json
{"status":"pass","features_checked":94,"audit_records_checked":94,"profiles_checked":6}
```

The concrete counts come from the checked artifacts; the example numbers are
illustrative. Output ends with one newline.

## Inventory Mode

Exit `0`; stderr empty; stdout contains one candidate live-baseline document:

```json
{"schema_version":1,"features":[{"id":"func:Execute","signature":"func(context.Context, *cobra.Command, ...ExecuteOption) int"}]}
```

Inventory mode never mutates files and never auto-blesses a surface change.

`-audit-seed` follows the same stream/exit rules, but emits the audit document
shape with empty decision fields. It is a mechanical spelling/signature aid,
not a valid committed audit and not an automated classification.

## Failure Contract

Every failure:

- writes nothing to stdout;
- writes exactly one minified `ax.Error` envelope plus newline to stderr;
- contains no separate usage, log, progress, or stack-trace lines;
- sorts all feature-level detail before encoding.

Stable error codes:

| Error code | Exit | Meaning |
|------------|------|---------|
| `surface_drift` | `2` | Source, profile, live baseline, audit state, or deprecation notice disagree. |
| `invalid_surface_artifact` | `2` | Missing, malformed, oversized, unsorted, duplicate, or schema-invalid audit/baseline; invalid flags/arguments. |
| `surface_permission` | `4` | Permission denial reading required files or executing required tooling. |
| `surface_internal` | `1` | Unexpected internal failure not classifiable as repository input or permission. |

Example shape:

```json
{"error_code":"surface_drift","message":"public surface differs from the reviewed baseline","trace_id":"00000000000000000000000000000000","tool":"surfacecheck","version":"v0.4.0","schema_version":"1.0.0","actionable_fix":"review the sorted drift and update source, audit, and baseline intentionally","suggestions":["added field:Labels.NewField","signature-changed func:Execute"],"retryable":false}
```

The implementation uses the repository's actual `ax.Error` schema; omitted
standard zero/optional fields follow that contract and its golden tests.

## Determinism

- Same source, toolchain version, audit, and baseline produce byte-identical
  output.
- Feature and suggestion order is canonical.
- No timestamps, temporary paths, module-cache paths, hostnames, or target
  iteration order appear in output.
- JSON is struct-backed and minified.
- `make surface-check` uses an `@`-prefixed recipe so Make does not echo the
  command onto stdout.

## Exit Summary

| Exit | Meaning |
|------|---------|
| `0` | Success. |
| `1` | Unknown/internal failure. |
| `2` | Validation failure, including surface drift and invalid artifacts. |
| `3` | Reserved network/timeout mapping; the tool has no network operation. |
| `4` | Authentication/permission failure. |

## CI and Make Wiring

- Add `.PHONY: surface-check`.
- `surface-check` runs `@go run ./internal/cmd/surfacecheck`.
- Add `surface-check` to the `ci` aggregate and Make help.
- Add an explicit surface-check step to the existing validate job next to
  documentation coverage.
- `make cover-check` remains a separate authoritative coverage-floor command;
  documentation must not claim `make ci` includes it unless the Makefile is
  changed accordingly.
