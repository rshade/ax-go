// Command apidiff-verdict scopes a go-apidiff report to ax-go's public API
// surface and decides whether a pull request introduces a breaking change that
// must be acknowledged.
//
// go-apidiff diffs the exported API of every package in the module, including
// internal/ packages. The constitution (Principle XI) exempts internal/ from
// the stability contract: the toolchain blocks external import, so there is no
// external consumer to break. This tool therefore filters the report down to
// the public allowlist — the root package ax plus the public packages
// config, contract, id, mcp, and schema — using exact package-path equality,
// never a prefix match (the root import path is a literal prefix of every
// internal/ path).
//
// It has two subcommands, both reading from stdin:
//
//	go-apidiff <base> --print-compatible | apidiff-verdict verdict
//	go list -f '{{.ImportPath}} {{.Name}}' ./... | apidiff-verdict check-packages
//
// verdict renders the public-only diff (to $GITHUB_STEP_SUMMARY and
// apidiff-comment.md), reports public_breaking / has_public_changes via
// $GITHUB_OUTPUT, and always exits 0 — the workflow's gate step decides
// pass/fail from those outputs. check-packages fails when the set of public
// packages no longer matches the hard-coded allowlist, catching a new public
// package added without guarding it.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// commentMarker is the hidden HTML comment embedded at the top of the rendered
// report. The PR-comment upsert script locates its sticky comment by this
// marker so repeated runs edit one comment instead of stacking new ones.
const commentMarker = "<!-- apidiff-report -->"

// commentFile is the path verdict writes the rendered markdown to, consumed by
// .github/apidiff-comment.sh.
const commentFile = "apidiff-comment.md"

const (
	// minArgs is the minimum os.Args length for a valid invocation: the program
	// name plus a subcommand name.
	minArgs = 2
	// scanBufInitial and scanBufMax size the report scanner's per-line buffer.
	// go-apidiff change lines carry full type signatures and can exceed
	// bufio.Scanner's 64 KiB default, so the buffer is allowed to grow to
	// scanBufMax.
	scanBufInitial = 64 * 1024 // 64 KiB
	scanBufMax     = 1 << 20   // 1 MiB
	// goListFieldCount is the number of whitespace-separated fields in a
	// `go list -f '{{.ImportPath}} {{.Name}}'` line: "<import-path> <name>".
	goListFieldCount = 2
)

// allowedPackages is the single source of truth for ax-go's public API surface:
// the root package ax plus the public packages config, contract, id, mcp, and
// schema (the import-isolated contract packages and the thin mcp runtime
// surface). go-apidiff findings for any other package (notably internal/ and
// examples/) are ignored by the gate. check-packages enforces that this list
// stays in sync with the module's actual public packages. Keep it sorted.
func allowedPackages() []string {
	return []string{
		"github.com/rshade/ax-go",
		"github.com/rshade/ax-go/config",
		"github.com/rshade/ax-go/contract",
		"github.com/rshade/ax-go/id",
		"github.com/rshade/ax-go/mcp",
		"github.com/rshade/ax-go/schema",
	}
}

// changeKind identifies which go-apidiff sub-section the parser is currently
// reading within a package block.
type changeKind int

const (
	kindNone changeKind = iota
	kindIncompatible
	kindCompatible
)

// section holds the parsed go-apidiff findings for a single package.
type section struct {
	pkg          string
	incompatible []string
	compatible   []string
}

func main() {
	if len(os.Args) < minArgs {
		failf("usage: apidiff-verdict <verdict|check-packages>")
	}
	switch os.Args[1] {
	case "verdict":
		runVerdict()
	case "check-packages":
		runCheckPackages()
	default:
		failf("unknown subcommand %q (want verdict or check-packages)", os.Args[1])
	}
}

// runVerdict parses a go-apidiff report from stdin, scopes it to the public
// allowlist, surfaces the result, and records the gate inputs. It always exits
// 0: the gate decision belongs to the workflow, which reads the recorded
// outputs.
func runVerdict() {
	sections, parseErr := parseReport(os.Stdin)
	if parseErr != nil {
		failf("parsing go-apidiff report: %v", parseErr)
	}
	public := filterPublic(sections, allowSet())
	rendered := render(public)

	if err := emitSummary(rendered); err != nil {
		failf("writing summary: %v", err)
	}
	if err := os.WriteFile(commentFile, []byte(rendered), 0o600); err != nil {
		failf("writing %s: %v", commentFile, err)
	}
	if err := emitOutput("public_breaking", hasBreaking(public)); err != nil {
		failf("writing output: %v", err)
	}
	if err := emitOutput("has_public_changes", hasAnyChange(public)); err != nil {
		failf("writing output: %v", err)
	}
}

// runCheckPackages reads `go list -f '{{.ImportPath}} {{.Name}}'` output from
// stdin and exits non-zero if the set of public packages drifts from
// allowedPackages.
func runCheckPackages() {
	if err := checkAllowlist(os.Stdin, allowedPackages()); err != nil {
		fmt.Fprintf(os.Stderr, "apidiff-verdict: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "OK: public package set matches the apidiff allowlist.")
}

// parseReport reads go-apidiff's text report into per-package sections. A
// package header is a line with no leading whitespace; indented lines under it
// are either a sub-section header ("Incompatible changes:" / "Compatible
// changes:") or a "- " change entry. The current sub-section resets on every
// package header so one package's findings cannot leak into the next.
func parseReport(r io.Reader) ([]section, error) {
	var sections []section
	cur := -1
	kind := kindNone

	sc := bufio.NewScanner(r)
	// go-apidiff change lines can be long (full type signatures); raise the
	// scanner's line cap well above the 64 KiB default.
	sc.Buffer(make([]byte, 0, scanBufInitial), scanBufMax)

	for sc.Scan() {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if !hasLeadingSpace(raw) {
			sections = append(sections, section{pkg: trimmed})
			cur = len(sections) - 1
			kind = kindNone
			continue
		}
		if cur < 0 {
			continue
		}
		switch trimmed {
		case "Incompatible changes:":
			kind = kindIncompatible
			continue
		case "Compatible changes:":
			kind = kindCompatible
			continue
		}
		item, ok := strings.CutPrefix(trimmed, "- ")
		if !ok {
			continue
		}
		switch kind {
		case kindIncompatible:
			sections[cur].incompatible = append(sections[cur].incompatible, item)
		case kindCompatible:
			sections[cur].compatible = append(sections[cur].compatible, item)
		case kindNone:
		}
	}
	return sections, sc.Err()
}

// filterPublic keeps only sections whose package path is in the allowlist,
// matched by exact equality (never prefix) so internal/ and examples/ paths
// that share the root import prefix are excluded.
func filterPublic(sections []section, allow map[string]bool) []section {
	var out []section
	for _, s := range sections {
		if allow[s.pkg] {
			out = append(out, s)
		}
	}
	return out
}

// hasBreaking reports whether any section has an incompatible change. It drives
// the merge gate.
func hasBreaking(sections []section) bool {
	for _, s := range sections {
		if len(s.incompatible) > 0 {
			return true
		}
	}
	return false
}

// hasAnyChange reports whether any section has a compatible or incompatible
// change. It drives whether to post a PR comment.
func hasAnyChange(sections []section) bool {
	for _, s := range sections {
		if len(s.incompatible) > 0 || len(s.compatible) > 0 {
			return true
		}
	}
	return false
}

// render produces the markdown report (comment body and step summary) for the
// public-scoped sections, prefixed with the sticky-comment marker.
func render(sections []section) string {
	var b strings.Builder
	b.WriteString(commentMarker)
	b.WriteString("\n## Public API diff\n\n")

	if !hasAnyChange(sections) {
		b.WriteString("No public API changes detected against the base branch.\n")
		return b.String()
	}
	if hasBreaking(sections) {
		b.WriteString("⚠️ **Breaking public API change detected.** ")
		b.WriteString(
			"If intentional, add the `breaking-change-approved` label and use a " +
				"`feat!:` / `BREAKING CHANGE:` commit so release-please bumps the minor " +
				"digit (Constitution Principle XI).\n\n",
		)
	}

	for _, s := range sections {
		if len(s.incompatible) == 0 && len(s.compatible) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### `%s`\n\n", s.pkg)
		writeChanges(&b, "Incompatible changes", s.incompatible)
		writeChanges(&b, "Compatible changes", s.compatible)
	}
	return b.String()
}

// writeChanges appends a labelled bullet list to b when items is non-empty.
func writeChanges(b *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "**%s:**\n\n", label)
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", item)
	}
	b.WriteString("\n")
}

// checkAllowlist reads `go list -f '{{.ImportPath}} {{.Name}}'` output and
// returns an error if the set of public packages (importable, non-internal,
// non-example, non-main) differs from allow.
func checkAllowlist(r io.Reader, allow []string) error {
	got := map[string]bool{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < goListFieldCount {
			return fmt.Errorf("malformed `go list` line: %q (want '<import-path> <package-name>')", line)
		}
		importPath, pkgName := fields[0], fields[1]
		if pkgName == "main" || isInternal(importPath) || isExample(importPath) {
			continue
		}
		got[importPath] = true
	}
	if err := sc.Err(); err != nil {
		return err
	}

	want := map[string]bool{}
	for _, p := range allow {
		want[p] = true
	}

	var missing, extra []string
	for p := range want {
		if !got[p] {
			missing = append(missing, p)
		}
	}
	for p := range got {
		if !want[p] {
			extra = append(extra, p)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}

	sort.Strings(extra)
	sort.Strings(missing)
	var b strings.Builder
	b.WriteString("public package allowlist drift (update allowedPackages in internal/cmd/apidiff-verdict/main.go):")
	for _, p := range extra {
		fmt.Fprintf(&b, "\n  + %s is public but not in the allowlist (guard it, or move it under internal/)", p)
	}
	for _, p := range missing {
		fmt.Fprintf(&b, "\n  - %s is in the allowlist but is no longer a public package", p)
	}
	return errors.New(b.String())
}

// isInternal reports whether importPath denotes an internal/ package.
func isInternal(importPath string) bool {
	return importPath == "internal" ||
		strings.HasSuffix(importPath, "/internal") ||
		strings.Contains(importPath, "/internal/")
}

// isExample reports whether importPath denotes an examples/ package.
func isExample(importPath string) bool {
	return strings.HasSuffix(importPath, "/examples") ||
		strings.Contains(importPath, "/examples/")
}

// hasLeadingSpace reports whether s begins with a space or tab.
func hasLeadingSpace(s string) bool {
	return s != "" && (s[0] == ' ' || s[0] == '\t')
}

// allowSet returns allowedPackages as a lookup set.
func allowSet() map[string]bool {
	set := map[string]bool{}
	for _, p := range allowedPackages() {
		set[p] = true
	}
	return set
}

// emitSummary appends rendered to the GitHub step summary file when running in
// CI, falling back to stdout for local runs.
func emitSummary(rendered string) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		_, err := fmt.Fprint(os.Stdout, rendered)
		return err
	}
	return appendFile(path, rendered)
}

// emitOutput records a boolean step output (key=value) to the GitHub output
// file when running in CI, falling back to stderr for local runs.
func emitOutput(key string, value bool) error {
	line := fmt.Sprintf("%s=%t\n", key, value)
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		_, err := fmt.Fprintf(os.Stderr, "apidiff-verdict: %s", line)
		return err
	}
	return appendFile(path, line)
}

// appendFile appends data to the file at path, creating it if absent.
func appendFile(path, data string) (err error) {
	// #nosec G703 -- path is only the GITHUB_STEP_SUMMARY / GITHUB_OUTPUT file
	// set by the GitHub Actions runner (see emitSummary/emitOutput), never
	// user-controlled input.
	f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if openErr != nil {
		return openErr
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	_, err = f.WriteString(data)
	return err
}

// failf prints a fatal diagnostic to stderr and exits non-zero.
func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "apidiff-verdict: "+format+"\n", args...)
	os.Exit(1)
}
