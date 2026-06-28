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
