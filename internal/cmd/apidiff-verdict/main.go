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
// the root package ax plus the public packages config, contract, id, logging,
// mcp, and schema (the import-isolated contract and logging packages and the
// thin mcp runtime surface). go-apidiff findings for any other package (notably
// internal/ and examples/) are ignored by the gate. check-packages enforces that
// this list stays in sync with the module's actual public packages. Keep it
// sorted.
//
// Every entry MUST be a plain string literal. surfacecheck's
// TestPublicPackagesMatchesAPIDiffAllowlist parses THIS FUNCTION'S SOURCE to
// cross-check the duplicated list (the two live in different main packages and
// cannot import each other), and a constant reference is invisible to that
// parser — it silently shrinks the parsed list and the guard starts comparing
// against an incomplete set.
func allowedPackages() []string {
	return []string{
		//nolint:goconst // must stay a literal: the surfacecheck guard parses this source
		"github.com/rshade/ax-go",
		"github.com/rshade/ax-go/config",
		"github.com/rshade/ax-go/contract",
		"github.com/rshade/ax-go/id",
		"github.com/rshade/ax-go/logging",
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

// relocated splits a section's incompatible findings into two slices: the genuine
// breaks first, then the type-relocation artifacts (see isTypeRelocation).
func (s section) relocated() ([]string, []string) {
	var breaking, relocations []string
	for _, item := range s.incompatible {
		if isTypeRelocation(item) {
			relocations = append(relocations, item)
			continue
		}
		breaking = append(breaking, item)
	}
	return breaking, relocations
}

// rootImportPath is the root package's import path, and moduleRoot is that path
// with the trailing slash used for subpackage prefix matching.
const rootImportPath = "github.com/rshade/ax-go"

// moduleRoot is this module's import path. A relocation is only ever excused when
// the type moved WITHIN this module; a type replaced by a third-party one is a
// real break no matter how the finding is worded.
const moduleRoot = rootImportPath + "/"

// isTypeRelocation reports whether an incompatible finding is a known go-apidiff
// false positive: an exported declaration whose source-visible shape did not
// change, but whose referenced type now has a different DECLARING PACKAGE.
//
// Why this exists. go-apidiff keys type identity on the declaring package, so
// moving a type to another package and leaving an identity-preserving alias
// behind (`type Error = contract.Error`) reads as incompatible even though every
// consumer compiles unchanged. ax-go has already shipped this refactor once: the
// v0.1.0 -> v0.2.0 release moved Error, Mode, Envelope, Schema, and the config
// option types into the import-isolated public packages, and it was correctly
// released as a non-breaking `feat:`. Running go-apidiff across that tag boundary
// today reports 37 findings of exactly this shape. The gate landed afterwards (PR
// #82) and has never been reconciled against the project's own precedent; without
// this classifier it would demand a `feat!:` for a release the project already
// established is a no-op for adopters.
//
// Two forms are recognised, both conservative:
//
//   - "X: changed from S to S" where the two renderings are TEXTUALLY IDENTICAL.
//     If the signature a consumer writes is unchanged character for character,
//     there is by construction nothing for a consumer to change.
//   - "X: changed from T to <module>/<pkg>.U" where T is a bare type name, the
//     target lives in this module, and T ends with U — the type kept its name, or
//     kept it minus a root-package prefix. Go guarantees an alias and its target
//     are the same type, so this is exactly what an identity-preserving
//     relocation looks like. T is the rendered TYPE, which for a constant is its
//     type rather than its own name ("ModeHuman: changed from Mode to
//     .../contract.Mode"), and type-parameter lists are stripped so a generic
//     type relocates like any other.
//
// The prefix allowance is not a loophole invented here; it is the project's own
// established convention, visible in the v0.1.0 -> v0.2.0 findings:
//
//	Error:             changed from Error             to .../contract.Error
//	ParseConfigOption: changed from ParseConfigOption to .../config.Option
//	SchemaOption:      changed from SchemaOption      to .../schema.Option
//
// Root ax prefixes what the isolated package names generically, because `Option`
// is unambiguous inside `config` but not inside `ax`. A genuine rename does not
// match: "Foo: changed from Foo to .../pkg.Bar" fails the suffix test and stays
// breaking.
//
// What this does NOT excuse, and why the gate keeps its teeth: a removed symbol,
// a changed method set, a renamed type, a widened or narrowed signature, or a
// relocation to a type outside this module all fail every branch below and stay
// breaking.
//
// The residual risk is a type that relocates AND changes shape in the same
// commit, where go-apidiff might report only the relocation. That risk is carried
// by a compensating control rather than ignored: internal/cmd/surfacecheck
// inventories every public declaration, field, and interface method across 4
// build configurations x 6 platform profiles and diffs it against a reviewed
// baseline, so a changed field type or method set is drift there even when
// apidiff is quiet. The two gates are complementary — apidiff for semantic
// compatibility, surfacecheck for exact structural surface — and relaxing this
// narrow case in one does not open a hole in the other.
//
// Relocations are never silently dropped: render lists them in their own section
// so a reviewer always sees what was excused.
func isTypeRelocation(item string) bool {
	// The declaration's own name is deliberately unused below: what a relocation
	// changes is the RENDERED TYPE, which for a constant is its type rather than
	// its name.
	_, before, after, ok := parseChangedFinding(item)
	if !ok {
		return false
	}
	if before == after {
		return true
	}
	// A relocation is a change of DECLARING PACKAGE only, so the old rendering must
	// be a bare type name. Note this is the rendered TYPE, which is not always the
	// declaration's own name: for a constant such as ModeHuman the rendering is
	// its type, Mode. Requiring before == name would excuse "Mode" while refusing
	// "ModeHuman" in the very same relocation, which is incoherent.
	//
	// Type parameters are stripped first so a generic type's relocation
	// ("Envelope[T any]" -> ".../contract.Envelope[T]") is recognised as the same
	// artifact as a non-generic one.
	before = stripTypeParams(before)
	if !isBareIdentifier(before) {
		return false
	}
	suffix, found := strings.CutPrefix(after, moduleRoot)
	if !found {
		return false
	}
	dot := strings.LastIndex(suffix, ".")
	if dot < 0 {
		return false
	}
	pkgPath, target := suffix[:dot], stripTypeParams(suffix[dot+1:])
	// A package path has no dots (a dotted first segment means another module's
	// domain), and the target must be a bare type name.
	if strings.Contains(pkgPath, ".") || !isBareIdentifier(target) {
		return false
	}
	// Same name, or the established root-package disambiguating-prefix drop
	// (ParseConfigOption → config.Option, LoggerOption → logcore.Option).
	// A bare HasSuffix would also excuse genuine renames such as AppError →
	// contract.Error; those stay breaking.
	return before == target || isOptionPrefixDrop(before, target)
}

// isOptionPrefixDrop reports whether before is target with a non-empty exported
// camelCase prefix stripped — the only established rename-on-relocation pattern
// in this module (FooOption / ParseConfigOption → Option). Any other suffix
// match is a real rename and must stay gated.
func isOptionPrefixDrop(before, target string) bool {
	if target != "Option" || !strings.HasSuffix(before, target) {
		return false
	}
	prefixLen := len(before) - len(target)
	if prefixLen == 0 {
		return false
	}
	// Identifiers accepted by isBareIdentifier are ASCII; the dropped prefix must
	// start with an uppercase letter so it is itself an exported segment.
	first := before[0]
	return first >= 'A' && first <= 'Z'
}

// stripTypeParams removes a trailing type-parameter or type-argument list, so
// "Envelope[T any]" and "Envelope[T]" both reduce to "Envelope". Relocating a
// generic type is the same artifact as relocating a plain one, and the bracketed
// part renders differently on each side purely because one is a declaration and
// the other an instantiation.
func stripTypeParams(s string) string {
	if idx := strings.Index(s, "["); idx >= 0 {
		return s[:idx]
	}
	return s
}

// isBareIdentifier reports whether s is a plain Go identifier — no package
// qualifier, no type parameters, no punctuation. Relocation findings only ever
// concern bare type names; anything richer is a signature change and must stay
// breaking.
func isBareIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// parseChangedFinding splits a go-apidiff finding of the form
// "Name: changed from BEFORE to AFTER" into (name, before, after, ok). Findings
// of any other shape (removed, added, and the various member-level diagnostics)
// return ok false and are therefore always treated as breaking.
func parseChangedFinding(item string) (string, string, string, bool) {
	colon := strings.Index(item, ": changed from ")
	if colon < 0 {
		return "", "", "", false
	}
	name := strings.TrimSpace(item[:colon])
	rest := item[colon+len(": changed from "):]

	// Split on the LAST " to " so a signature containing the substring does not
	// truncate the comparison.
	sep := strings.LastIndex(rest, " to ")
	if sep < 0 {
		return "", "", "", false
	}
	before := strings.TrimSpace(rest[:sep])
	after := strings.TrimSpace(rest[sep+len(" to "):])
	if name == "" || before == "" || after == "" {
		return "", "", "", false
	}
	return name, before, after, true
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

// hasBreaking reports whether any section has a genuinely incompatible change. It
// drives the merge gate. Type-relocation artifacts are excluded (see
// isTypeRelocation) but are still reported.
func hasBreaking(sections []section) bool {
	for _, s := range sections {
		if breaking, _ := s.relocated(); len(breaking) > 0 {
			return true
		}
	}
	return false
}

// hasRelocations reports whether any section carries an excused type-relocation
// finding, so render can explain what it set aside.
func hasRelocations(sections []section) bool {
	for _, s := range sections {
		if _, relocations := s.relocated(); len(relocations) > 0 {
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
	if hasRelocations(sections) {
		b.WriteString(
			"ℹ️ **Type relocations detected and not gated.** go-apidiff reports a type " +
				"whose declaring package changed as incompatible, even when an " +
				"identity-preserving alias keeps every consumer compiling unchanged. " +
				"ax-go shipped exactly this refactor in v0.1.0 → v0.2.0 as a non-breaking " +
				"`feat:`. The findings are listed below so they are reviewed, not hidden; " +
				"structural changes to these types are still gated by " +
				"`make surface-check`.\n\n",
		)
	}

	for _, s := range sections {
		if len(s.incompatible) == 0 && len(s.compatible) == 0 {
			continue
		}
		breaking, relocations := s.relocated()
		fmt.Fprintf(&b, "### `%s`\n\n", s.pkg)
		writeChanges(&b, "Incompatible changes", breaking)
		writeChanges(&b, "Type relocations (not gated)", relocations)
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
