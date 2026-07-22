#!/usr/bin/env bash
#
# scan.sh — read the machine sources that describe an ax-go upgrade hop.
#
# Usage:
#   scripts/scan.sh <target-version> [current-version]
#
# Examples:
#   scripts/scan.sh v0.3.0            # just the target's CHANGELOG section
#   scripts/scan.sh v0.3.0 v0.2.0     # every CHANGELOG section newer than current
#
# It reads from the module cache populated by `go get ax-go@<target>`, so run
# the `go get` first. It prints three sections: the CHANGELOG delta, the
# module's `// Deprecated:` symbols, and (if staticcheck is installed) the
# SA1019 hits in the current working directory's module.
set -euo pipefail

MODULE="github.com/rshade/ax-go"

if [ "$#" -lt 1 ]; then
  echo "usage: scan.sh <target-version> [current-version]" >&2
  exit 2
fi

norm() { case "$1" in v*) printf '%s' "$1";; *) printf 'v%s' "$1";; esac; }
TARGET="$(norm "$1")"
CURRENT="${2:+$(norm "$2")}"

MODCACHE="$(go env GOMODCACHE)"
DIR="$MODCACHE/$MODULE@$TARGET"

if [ ! -d "$DIR" ]; then
  echo "module source for $MODULE@$TARGET not found in the cache." >&2
  echo "run:  go get $MODULE@$TARGET && go mod download $MODULE" >&2
  exit 1
fi

echo "== CHANGELOG delta (up to and including $TARGET) =="
CHANGELOG="$DIR/CHANGELOG.md"
if [ -f "$CHANGELOG" ]; then
  # release-please headers look like:  ## [0.3.0](...) (date)
  # The file is newest-first, so the target header sits above the current one.
  # Print from the target header down to (but not including) the current header,
  # or to the next version header when no current version is given.
  awk -v target="${TARGET#v}" -v current="${CURRENT#v}" '
    function is_ver_header(line) { return line ~ /^## \[[0-9]+\.[0-9]+\.[0-9]+\]/ }
    function ver_of(line,  m) { if (match(line, /[0-9]+\.[0-9]+\.[0-9]+/)) return substr(line, RSTART, RLENGTH); return "" }
    {
      if (is_ver_header($0)) {
        v = ver_of($0)
        if (!printing && v == target) { printing = 1 }
        else if (printing && current != "" && v == current) { exit }
        else if (printing && current == "" && v != target) { exit }
      }
      if (printing) print
    }
  ' "$CHANGELOG"
else
  echo "(no CHANGELOG.md in module source)"
fi

echo
echo "== Deprecated symbols in $MODULE@$TARGET =="
# Non-test .go files carrying a Deprecated: note. Show file and the note line.
if grep -rn --include='*.go' -e '// Deprecated:' "$DIR" \
    | grep -v '_test.go:' | sed "s#$DIR/##"; then
  :
else
  echo "(none found)"
fi

echo
echo "== SA1019 (deprecated-symbol use) in the current module =="
if command -v golangci-lint >/dev/null 2>&1; then
  # golangci-lint's bundled staticcheck is version-matched to the toolchain.
  # A clean run exits 0; any finding (SA1019 or otherwise) or a run failure
  # exits non-zero. Only treat exit 0 as "no findings" — otherwise surface
  # SA1019 hits, or fail loudly on a genuine linter/build error so a real
  # failure is never collapsed into a false "clean" report.
  if out="$(golangci-lint run --default=none --enable=staticcheck ./... 2>&1)"; then
    echo "(no SA1019 findings)"
  elif printf '%s\n' "$out" | grep 'SA1019'; then
    :
  else
    echo "golangci-lint failed without SA1019 findings; full output:" >&2
    printf '%s\n' "$out" >&2
    exit 1
  fi
elif command -v staticcheck >/dev/null 2>&1; then
  # Preserve staticcheck's exit status: exit 0 means no findings; otherwise
  # distinguish the go-directive refusal, real SA1019 hits, and any other
  # failure, so a build/import error is never hidden behind "(no findings)".
  if out="$(staticcheck ./... 2>&1)"; then
    echo "(no SA1019 findings)"
  elif printf '%s' "$out" | grep -q 'was built with go'; then
    printf '%s\n' "$out" | grep 'was built with go' >&2
    echo "(standalone staticcheck is older than this module's Go directive; use" \
         "golangci-lint run --default=none --enable=staticcheck ./... instead)"
  elif printf '%s\n' "$out" | grep 'SA1019'; then
    :
  else
    echo "staticcheck failed without SA1019 findings; full output:" >&2
    printf '%s\n' "$out" >&2
    exit 1
  fi
else
  echo "(no linter found; install golangci-lint, or grep the deprecated symbols" \
       "listed above for their use in your code)"
fi
