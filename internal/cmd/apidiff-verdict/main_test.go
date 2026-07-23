package main

import (
	"strings"
	"testing"
)

// publicBreak is a go-apidiff report fixture where a public contract package
// (config) drops an exported symbol.
const publicBreak = `github.com/rshade/ax-go/config
  Incompatible changes:
  - ParseConfig: removed
`

// internalBreak is a report fixture where only an internal package breaks. Its
// import path shares the root prefix, so a naive prefix match would wrongly
// flag it; exact-equality filtering must ignore it.
const internalBreak = `github.com/rshade/ax-go/internal/config
  Incompatible changes:
  - parseInternal: removed
`

// rootCompatibleOnly is a report fixture where the root package gains a symbol
// (a compatible, additive change).
const rootCompatibleOnly = `github.com/rshade/ax-go
  Compatible changes:
  - NewThing: added
`

func TestVerdictFromReport(t *testing.T) {
	cases := []struct {
		name          string
		report        string
		wantBreaking  bool
		wantAnyChange bool
	}{
		{
			name:          "empty report",
			report:        "",
			wantBreaking:  false,
			wantAnyChange: false,
		},
		{
			name:          "public package breaking change",
			report:        publicBreak,
			wantBreaking:  true,
			wantAnyChange: true,
		},
		{
			name:          "internal-only break is exempt",
			report:        internalBreak,
			wantBreaking:  false,
			wantAnyChange: false,
		},
		{
			name:          "internal path sharing root prefix is not matched",
			report:        "github.com/rshade/ax-go/internal/schema\n  Incompatible changes:\n  - X: removed\n",
			wantBreaking:  false,
			wantAnyChange: false,
		},
		{
			name:          "examples package is exempt",
			report:        "github.com/rshade/ax-go/examples/integration\n  Incompatible changes:\n  - main: changed\n",
			wantBreaking:  false,
			wantAnyChange: false,
		},
		{
			name:          "compatible-only public change does not gate",
			report:        rootCompatibleOnly,
			wantBreaking:  false,
			wantAnyChange: true,
		},
		{
			name:          "public compatible plus internal incompatible stays non-breaking",
			report:        rootCompatibleOnly + internalBreak,
			wantBreaking:  false,
			wantAnyChange: true,
		},
		{
			name:          "mixed public packages with one breaking",
			report:        rootCompatibleOnly + publicBreak,
			wantBreaking:  true,
			wantAnyChange: true,
		},
		{
			name: "incompatible section in excluded package does not leak into next",
			report: "github.com/rshade/ax-go/internal/cli\n" +
				"  Incompatible changes:\n" +
				"  - Run: removed\n" +
				"github.com/rshade/ax-go/id\n" +
				"  Compatible changes:\n" +
				"  - NewEntityID: added\n",
			wantBreaking:  false,
			wantAnyChange: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sections, err := parseReport(strings.NewReader(tc.report))
			if err != nil {
				t.Fatalf("parseReport: %v", err)
			}
			public := filterPublic(sections, allowSet())
			if got := hasBreaking(public); got != tc.wantBreaking {
				t.Errorf("hasBreaking = %t, want %t", got, tc.wantBreaking)
			}
			if got := hasAnyChange(public); got != tc.wantAnyChange {
				t.Errorf("hasAnyChange = %t, want %t", got, tc.wantAnyChange)
			}
		})
	}
}

func TestParseReportSplitsPackagesAndKinds(t *testing.T) {
	report := "github.com/rshade/ax-go/contract\n" +
		"  Incompatible changes:\n" +
		"  - Error.Code: changed from string to int\n" +
		"  - NewError: removed\n" +
		"  Compatible changes:\n" +
		"  - NewEnvelope: added\n"

	sections, err := parseReport(strings.NewReader(report))
	if err != nil {
		t.Fatalf("parseReport: %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	s := sections[0]
	if s.pkg != "github.com/rshade/ax-go/contract" {
		t.Errorf("pkg = %q", s.pkg)
	}
	if len(s.incompatible) != 2 {
		t.Errorf("incompatible = %v, want 2 entries", s.incompatible)
	}
	if len(s.compatible) != 1 {
		t.Errorf("compatible = %v, want 1 entry", s.compatible)
	}
	if s.incompatible[0] != "Error.Code: changed from string to int" {
		t.Errorf("first incompatible = %q", s.incompatible[0])
	}
}

func TestParseReportSkipsBlankAndStrayLines(t *testing.T) {
	report := "\n\ngithub.com/rshade/ax-go\n  Compatible changes:\n  - A: added\n\n"
	sections, err := parseReport(strings.NewReader(report))
	if err != nil {
		t.Fatalf("parseReport: %v", err)
	}
	if len(sections) != 1 || len(sections[0].compatible) != 1 {
		t.Fatalf("unexpected sections: %+v", sections)
	}
}

func TestRender(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		out := render(nil)
		if !strings.Contains(out, commentMarker) {
			t.Errorf("missing marker:\n%s", out)
		}
		if !strings.Contains(out, "No public API changes") {
			t.Errorf("missing no-change message:\n%s", out)
		}
	})

	t.Run("breaking change includes label guidance", func(t *testing.T) {
		sections, _ := parseReport(strings.NewReader(publicBreak))
		out := render(filterPublic(sections, allowSet()))
		for _, want := range []string{
			commentMarker,
			"Breaking public API change",
			"breaking-change-approved",
			"github.com/rshade/ax-go/config",
			"ParseConfig: removed",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("render missing %q in:\n%s", want, out)
			}
		}
	})

	t.Run("compatible-only omits breaking banner", func(t *testing.T) {
		sections, _ := parseReport(strings.NewReader(rootCompatibleOnly))
		out := render(filterPublic(sections, allowSet()))
		if strings.Contains(out, "Breaking public API change") {
			t.Errorf("unexpected breaking banner:\n%s", out)
		}
		if !strings.Contains(out, "NewThing: added") {
			t.Errorf("missing compatible change:\n%s", out)
		}
	})
}

func TestCheckAllowlist(t *testing.T) {
	allow := allowedPackages()

	cases := []struct {
		name     string
		goList   string
		wantErr  bool
		errMatch string
	}{
		{
			name: "exact match passes",
			goList: "github.com/rshade/ax-go ax\n" +
				"github.com/rshade/ax-go/config config\n" +
				"github.com/rshade/ax-go/contract contract\n" +
				"github.com/rshade/ax-go/id id\n" +
				"github.com/rshade/ax-go/logging logging\n" +
				"github.com/rshade/ax-go/mcp mcp\n" +
				"github.com/rshade/ax-go/schema schema\n" +
				"github.com/rshade/ax-go/internal/cli cli\n" +
				"github.com/rshade/ax-go/internal/cmd/doccover main\n" +
				"github.com/rshade/ax-go/examples/integration main\n",
			wantErr: false,
		},
		{
			name: "extra public package fails",
			goList: "github.com/rshade/ax-go ax\n" +
				"github.com/rshade/ax-go/config config\n" +
				"github.com/rshade/ax-go/contract contract\n" +
				"github.com/rshade/ax-go/id id\n" +
				"github.com/rshade/ax-go/logging logging\n" +
				"github.com/rshade/ax-go/mcp mcp\n" +
				"github.com/rshade/ax-go/schema schema\n" +
				"github.com/rshade/ax-go/telemetry telemetry\n",
			wantErr:  true,
			errMatch: "github.com/rshade/ax-go/telemetry is public but not in the allowlist",
		},
		{
			name: "missing allowlisted package fails",
			goList: "github.com/rshade/ax-go ax\n" +
				"github.com/rshade/ax-go/config config\n" +
				"github.com/rshade/ax-go/contract contract\n" +
				"github.com/rshade/ax-go/id id\n" +
				"github.com/rshade/ax-go/logging logging\n" +
				"github.com/rshade/ax-go/mcp mcp\n",
			wantErr:  true,
			errMatch: "github.com/rshade/ax-go/schema is in the allowlist but is no longer a public package",
		},
		{
			name: "internal package named like a public leaf is ignored",
			goList: "github.com/rshade/ax-go ax\n" +
				"github.com/rshade/ax-go/config config\n" +
				"github.com/rshade/ax-go/contract contract\n" +
				"github.com/rshade/ax-go/id id\n" +
				"github.com/rshade/ax-go/logging logging\n" +
				"github.com/rshade/ax-go/mcp mcp\n" +
				"github.com/rshade/ax-go/schema schema\n" +
				"github.com/rshade/ax-go/internal/config config\n" +
				"github.com/rshade/ax-go/internal/schema schema\n" +
				"github.com/rshade/ax-go/internal/mcpserver mcpserver\n",
			wantErr: false,
		},
		{
			name:     "malformed line fails",
			goList:   "github.com/rshade/ax-go\n",
			wantErr:  true,
			errMatch: "malformed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkAllowlist(strings.NewReader(tc.goList), allow)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.errMatch != "" && !strings.Contains(err.Error(), tc.errMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- Type-relocation classification -----------------------------------------
//
// These tests defend a deliberate narrowing of the gate. The rule exists because
// go-apidiff reports a type whose declaring package changed as incompatible even
// when an identity-preserving alias keeps every consumer compiling — a false
// positive ax-go has already shipped through once, in v0.1.0 -> v0.2.0. The tests
// below matter less for the cases they excuse than for the ones they refuse: a
// classifier that is too generous turns the merge gate into decoration.

func TestIsTypeRelocationRecognisesAliasArtifacts(t *testing.T) {
	cases := []struct {
		name string
		item string
	}{
		{
			name: "identical renderings",
			item: "Flush: changed from func(context.Context, Logger) error to func(context.Context, Logger) error",
		},
		{
			name: "type kept its name",
			item: "Labels: changed from Labels to github.com/rshade/ax-go/internal/logcore.Labels",
		},
		{
			name: "v0.2.0 precedent: Error to contract.Error",
			item: "Error: changed from Error to github.com/rshade/ax-go/contract.Error",
		},
		{
			name: "v0.2.0 precedent: prefixed option name",
			item: "ParseConfigOption: changed from ParseConfigOption to github.com/rshade/ax-go/config.Option",
		},
		{
			name: "prefixed option name into an internal package",
			item: "LoggerOption: changed from LoggerOption to github.com/rshade/ax-go/internal/logcore.Option",
		},
		{
			// A constant renders as its TYPE, not its own name. Excusing "Mode"
			// while refusing "ModeHuman" in the same relocation would be
			// incoherent, so the rule keys on the rendered type.
			name: "constant whose type relocated",
			item: "ModeHuman: changed from Mode to github.com/rshade/ax-go/contract.Mode",
		},
		{
			// The bracketed part differs purely because one side is a declaration
			// and the other an instantiation.
			name: "generic type relocated",
			item: "Envelope: changed from Envelope[T any] to github.com/rshade/ax-go/contract.Envelope[T]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isTypeRelocation(tc.item) {
				t.Fatalf("isTypeRelocation(%q) = false, want true", tc.item)
			}
		})
	}
}

// TestIsTypeRelocationRefusesRealBreaks is the load-bearing half. Every case here
// is a change a consumer would actually have to react to, and each must remain
// breaking. If any of these starts being excused, the gate has stopped working.
func TestIsTypeRelocationRefusesRealBreaks(t *testing.T) {
	cases := []struct {
		name string
		item string
	}{
		{
			name: "removed symbol",
			item: "ParseConfig: removed",
		},
		{
			name: "genuine rename",
			item: "Foo: changed from Foo to github.com/rshade/ax-go/contract.Bar",
		},
		{
			name: "relocation to a third-party type",
			item: "Logger: changed from Logger to github.com/rs/zerolog.Logger",
		},
		{
			name: "relocation to another module entirely",
			item: "Error: changed from Error to example.com/other/pkg.Error",
		},
		{
			name: "signature widened",
			item: "NewError: changed from func(string) *Error to func(string, int) *Error",
		},
		{
			name: "signature narrowed",
			item: "NewLogger: changed from func(context.Context, ...LoggerOption) Logger to func(context.Context) Logger",
		},
		{
			name: "field type changed",
			item: "Labels.Environment: changed from string to int",
		},
		{
			name: "method removed from an interface",
			item: "Logger.Zerolog: removed",
		},
		{
			name: "kind changed from func to var",
			item: "Flush: changed from func(context.Context, Logger) error to var",
		},
		{
			name: "before is a signature, after is a relocated type",
			item: "X: changed from func() Thing to github.com/rshade/ax-go/pkg.Thing",
		},
		{
			name: "not a changed finding at all",
			item: "NewThing: added",
		},
		{
			// Stripping type parameters must not turn a rename into a match.
			name: "generic type renamed while relocating",
			item: "Envelope: changed from Envelope[T any] to github.com/rshade/ax-go/contract.Wrapper[T]",
		},
		{
			// A dotted first path segment means another module's domain.
			name: "relocation to a dotted third-party path",
			item: "Mode: changed from Mode to gopkg.in/thing.Mode",
		},
		{
			// HasSuffix alone would excuse this; only the *Option prefix drop is
			// the established rename-on-relocation convention.
			name: "suffix rename AppError to Error",
			item: "AppError: changed from AppError to github.com/rshade/ax-go/contract.Error",
		},
		{
			name: "suffix rename HTTPStatus to Status",
			item: "HTTPStatus: changed from HTTPStatus to github.com/rshade/ax-go/contract.Status",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if isTypeRelocation(tc.item) {
				t.Fatalf("isTypeRelocation(%q) = true, want false: this is a real break", tc.item)
			}
		})
	}
}

// TestHasBreakingIgnoresRelocationsButNotRealBreaks pins the gate wiring: a
// section containing only relocations does not gate, and one real break among
// many relocations still does.
func TestHasBreakingIgnoresRelocationsButNotRealBreaks(t *testing.T) {
	relocationsOnly := []section{{
		pkg: "github.com/rshade/ax-go",
		incompatible: []string{
			"Labels: changed from Labels to github.com/rshade/ax-go/internal/logcore.Labels",
			"Flush: changed from func(context.Context, Logger) error to func(context.Context, Logger) error",
		},
	}}
	if hasBreaking(relocationsOnly) {
		t.Error("hasBreaking = true for relocation-only findings, want false")
	}
	if !hasRelocations(relocationsOnly) {
		t.Error("hasRelocations = false, want true so the report can explain what was excused")
	}

	withRealBreak := []section{{
		pkg: "github.com/rshade/ax-go",
		incompatible: []string{
			"Labels: changed from Labels to github.com/rshade/ax-go/internal/logcore.Labels",
			"ParseConfig: removed",
		},
	}}
	if !hasBreaking(withRealBreak) {
		t.Error("hasBreaking = false when a real break accompanies relocations, want true")
	}
}

// TestRenderListsRelocationsSeparately pins that excused findings stay VISIBLE. A
// gate that quietly drops findings is worse than one that fails, because it is
// trusted; the report must always show what it set aside and why.
func TestRenderRelocationsRemainVisible(t *testing.T) {
	out := render([]section{{
		pkg:          "github.com/rshade/ax-go",
		incompatible: []string{"Labels: changed from Labels to github.com/rshade/ax-go/internal/logcore.Labels"},
	}})

	for _, want := range []string{
		"Type relocations (not gated)",
		"Labels: changed from Labels to github.com/rshade/ax-go/internal/logcore.Labels",
		"surface-check",
		"v0.1.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Breaking public API change detected") {
		t.Errorf("render wrongly announced a breaking change for a relocation:\n%s", out)
	}
}

// TestParseChangedFinding pins the parser, including the last-" to " rule that
// keeps a signature containing " to " from truncating the comparison.
func TestParseChangedFinding(t *testing.T) {
	name, before, after, ok := parseChangedFinding(
		"Flush: changed from func(context.Context, Logger) error to func(context.Context, Logger) error")
	if !ok {
		t.Fatal("parseChangedFinding returned ok=false for a well-formed finding")
	}
	if name != "Flush" {
		t.Errorf("name = %q, want Flush", name)
	}
	if before != after {
		t.Errorf("before %q != after %q for an identical-rendering finding", before, after)
	}

	if _, _, _, ok := parseChangedFinding("ParseConfig: removed"); ok {
		t.Error("parseChangedFinding accepted a non-changed finding")
	}
}

// TestClassifierMatchesShippedReleaseHistory is the acceptance test for the
// relocation rule, and the reason it can be trusted.
//
// The v0.1.0 -> v0.2.0 release moved Error, Mode, Envelope, Schema, and the
// config/schema option types into the import-isolated public packages. It shipped
// as a plain `feat:` and was a no-op for adopters. Running go-apidiff across that
// tag boundary today produces 37 findings, every one of this artifact class — so
// a classifier that is correct must rule that release non-breaking. A fixture of
// the distinct shapes from that report stands in for the full run so the test
// needs no network, no tags, and no git.
func TestClassifierMatchesShippedReleaseHistory(t *testing.T) {
	// Verbatim shapes from `go-apidiff v0.1.0` at v0.2.0.
	shipped := []string{
		"BuildSchema: changed from func(*github.com/spf13/cobra.Command, ...SchemaOption) Schema to func(*github.com/spf13/cobra.Command, ...SchemaOption) Schema",
		"CommandSchema: changed from CommandSchema to github.com/rshade/ax-go/schema.CommandSchema",
		"Envelope: changed from Envelope[T any] to github.com/rshade/ax-go/contract.Envelope[T]",
		"Error: changed from Error to github.com/rshade/ax-go/contract.Error",
		"ErrorOption: changed from ErrorOption to github.com/rshade/ax-go/contract.ErrorOption",
		"Metadata: changed from Metadata to github.com/rshade/ax-go/contract.Metadata",
		"Mode: changed from Mode to github.com/rshade/ax-go/contract.Mode",
		"ModeHuman: changed from Mode to github.com/rshade/ax-go/contract.Mode",
		"ModeJSON: changed from Mode to github.com/rshade/ax-go/contract.Mode",
		"NewError: changed from func(context.Context, string, string, ...ErrorOption) *Error to func(context.Context, string, string, ...ErrorOption) *Error",
		"ParseConfigOption: changed from ParseConfigOption to github.com/rshade/ax-go/config.Option",
		"SchemaOption: changed from SchemaOption to github.com/rshade/ax-go/schema.Option",
	}

	for _, item := range shipped {
		if !isTypeRelocation(item) {
			t.Errorf(
				"finding from the shipped v0.2.0 release classified as breaking:\n  %s\n"+
					"that release was a no-op for adopters, so the classifier is wrong",
				item,
			)
		}
	}

	if hasBreaking([]section{{pkg: "github.com/rshade/ax-go", incompatible: shipped}}) {
		t.Error("hasBreaking = true for the v0.2.0 findings, want false")
	}
}
