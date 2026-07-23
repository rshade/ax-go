package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const miniBaseline = `{"schema_version":1,"features":[{"id":"const:Answer","signature":"untyped int"},{"id":"func:Do","signature":"func()"}]}`

const miniAudit = `{"schema_version":1,"audited_at":"2026-07-19","records":[` +
	`{"id":"const:Answer","kind":"const","owner":"","name":"Answer","signature":"untyped int","classification":"supported","rationale":"Fixture feature.","disposition":"keep-public","internal_target":"","replacement":"","compatibility_strategy":"Keep the public selector unchanged.","lifecycle":"live","first_published":"","deprecated_in":"","removed_in":"","downstream_checked_at":"","downstream_evidence":[]},` +
	`{"id":"func:Do","kind":"func","owner":"","name":"Do","signature":"func()","classification":"supported","rationale":"Fixture feature.","disposition":"keep-public","internal_target":"","replacement":"","compatibility_strategy":"Keep the public selector unchanged.","lifecycle":"live","first_published":"","deprecated_in":"","removed_in":"","downstream_checked_at":"","downstream_evidence":[]}` +
	`]}`

func miniLeakRecord(id, kind, owner, name, signature string) string {
	return `{"id":"` + id + `","kind":"` + kind + `","owner":"` + owner + `","name":"` + name + `","signature":"` + signature + `",` +
		`"classification":"implementation-leak","rationale":"Fixture leak.","disposition":"relocate-with-forwarder",` +
		`"internal_target":"internal/fixture","replacement":"fixture.New","compatibility_strategy":"Root forwarder preserves name, type, and semantics.",` +
		`"lifecycle":"live","first_published":"v0.1.0","deprecated_in":"","removed_in":"",` +
		`"downstream_checked_at":"2026-07-19","downstream_evidence":["downstream: no indexed consumers","in-repo: internal/cmd/x"]}`
}

func writeArtifact(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "artifact.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	return path
}

func TestParseBaseline(t *testing.T) {
	t.Parallel()
	oversize := strings.Repeat(" ", 1<<20+1)
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: miniBaseline},
		{name: "valid indented", input: "{\n  \"schema_version\": 1,\n  \"features\": [\n" +
			"    {\"id\": \"const:Answer\", \"signature\": \"untyped int\"},\n" +
			"    {\"id\": \"func:Do\", \"signature\": \"func()\"}\n  ]\n}"},
		{name: "unknown field", input: `{"schema_version":1,"features":[],"bogus":true}`, wantErr: true},
		{name: "trailing value", input: miniBaseline + ` {}`, wantErr: true},
		{name: "trailing garbage", input: miniBaseline + `x`, wantErr: true},
		{name: "schema version 2", input: `{"schema_version":2,"features":[]}`, wantErr: true},
		{name: "missing schema_version", input: `{"features":[]}`, wantErr: true},
		{name: "missing features", input: `{"schema_version":1}`, wantErr: true},
		{name: "unsorted", input: `{"schema_version":1,"features":[{"id":"func:Do","signature":"func()"},` +
			`{"id":"const:Answer","signature":"untyped int"}]}`, wantErr: true},
		{
			name: "duplicate id",
			input: `{"schema_version":1,"features":[{"id":"const:Answer","signature":"untyped int"},` +
				`{"id":"const:Answer","signature":"untyped int"}]}`,
			wantErr: true,
		},
		{name: "empty id", input: `{"schema_version":1,"features":[{"id":"","signature":"func()"}]}`, wantErr: true},
		{
			name:    "empty signature",
			input:   `{"schema_version":1,"features":[{"id":"func:Do","signature":""}]}`,
			wantErr: true,
		},
		{name: "not an object", input: `[1,2,3]`, wantErr: true},
		{name: "not json", input: `hello`, wantErr: true},
		{name: "oversize", input: oversize, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := parseBaseline([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseBaseline(%s) succeeded, want error", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseBaseline(%s): %v", tc.name, err)
			}
			if doc == nil || doc.SchemaVersion != 1 {
				t.Fatalf("doc = %+v, want schema_version 1", doc)
			}
		})
	}
}

func TestParseAudit(t *testing.T) {
	t.Parallel()
	leak := miniLeakRecord("func:Do", "func", "", "Do", "func()")
	oversize := strings.Repeat(" ", 1<<20+1)
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid supported", input: miniAudit},
		{name: "valid leak", input: `{"schema_version":1,"audited_at":"2026-07-19","records":[` + leak + `]}`},
		{
			name: "valid removed",
			input: `{"schema_version":1,"audited_at":"2026-07-19","records":[` + strings.Replace(
				leak,
				`"lifecycle":"live","first_published":"v0.1.0","deprecated_in":"","removed_in":""`,
				`"lifecycle":"removed","first_published":"v0.1.0","deprecated_in":"v0.4.0","removed_in":"v0.5.0"`,
				1,
			) + `]}`,
		},
		{
			name:    "unknown field",
			input:   `{"schema_version":1,"audited_at":"2026-07-19","records":[],"bogus":1}`,
			wantErr: true,
		},
		{name: "trailing value", input: miniAudit + ` 42`, wantErr: true},
		{name: "missing audited_at", input: `{"schema_version":1,"records":[]}`, wantErr: true},
		{name: "bad audited_at", input: `{"schema_version":1,"audited_at":"19/07/2026","records":[]}`, wantErr: true},
		{
			name:    "bad kind enum",
			input:   strings.Replace(miniAudit, `"kind":"const"`, `"kind":"constant"`, 1),
			wantErr: true,
		},
		{
			name: "supported with relocate disposition",
			input: strings.Replace(
				miniAudit,
				`"disposition":"keep-public"`,
				`"disposition":"relocate-with-forwarder"`,
				1,
			),
			wantErr: true,
		},
		{
			name:    "supported with deprecated lifecycle",
			input:   strings.Replace(miniAudit, `"lifecycle":"live"`, `"lifecycle":"deprecated"`, 1),
			wantErr: true,
		},
		{
			name:    "supported with replacement set",
			input:   strings.Replace(miniAudit, `"replacement":""`, `"replacement":"x.Y"`, 1),
			wantErr: true,
		},
		{
			name: "supported with checked_at set",
			input: strings.Replace(
				miniAudit,
				`"downstream_checked_at":""`,
				`"downstream_checked_at":"2026-07-19"`,
				1,
			),
			wantErr: true,
		},
		{
			name: "leak with keep-public",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"disposition":"relocate-with-forwarder"`,
				`"disposition":"keep-public"`,
				1,
			),
			wantErr: true,
		},
		{
			name: "leak missing internal_target",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"internal_target":"internal/fixture"`,
				`"internal_target":""`,
				1,
			),
			wantErr: true,
		},
		{
			name: "leak internal helpers forbidden",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"internal_target":"internal/fixture"`,
				`"internal_target":"internal/helpers"`,
				1,
			),
			wantErr: true,
		},
		{
			name: "leak missing replacement",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"replacement":"fixture.New"`,
				`"replacement":""`,
				1,
			),
			wantErr: true,
		},
		{
			name: "leak missing compatibility strategy",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"compatibility_strategy":"Root forwarder preserves name, type, and semantics."`,
				`"compatibility_strategy":""`,
				1,
			),
			wantErr: true,
		},
		{
			name: "leak missing checked_at",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"downstream_checked_at":"2026-07-19"`,
				`"downstream_checked_at":""`,
				1,
			),
			wantErr: true,
		},
		{
			name: "leak missing evidence",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"downstream_evidence":["downstream: no indexed consumers","in-repo: internal/cmd/x"]`,
				`"downstream_evidence":null`,
				1,
			),
			wantErr: true,
		},
		{
			name:    "empty rationale",
			input:   strings.Replace(miniAudit, `"rationale":"Fixture feature."`, `"rationale":""`, 1),
			wantErr: true,
		},
		{
			name:    "multiline rationale",
			input:   strings.Replace(miniAudit, `"rationale":"Fixture feature."`, `"rationale":"two\nlines"`, 1),
			wantErr: true,
		},
		{
			name: "removable without deprecated_in",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"lifecycle":"live"`,
				`"lifecycle":"removable"`,
				1,
			),
			wantErr: true,
		},
		{
			name: "removed without removed_in",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"lifecycle":"live","first_published":"v0.1.0","deprecated_in":"","removed_in":""`,
				`"lifecycle":"removed","first_published":"v0.1.0","deprecated_in":"v0.4.0","removed_in":""`,
				1,
			),
			wantErr: true,
		},
		{
			name: "live with removed_in",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"removed_in":""`,
				`"removed_in":"v9.9.9"`,
				1,
			),
			wantErr: true,
		},
		{
			name: "bad first_published",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"first_published":"v0.1.0"`,
				`"first_published":"0.1.0"`,
				1,
			),
			wantErr: true,
		},
		{
			name: "unsorted records",
			input: `{"schema_version":1,"audited_at":"2026-07-19","records":[` + miniAuditRecord(
				"func:Do",
			) + `,` + miniAuditRecord(
				"const:Answer",
			) + `]}`,
			wantErr: true,
		},
		{
			name: "duplicate record ids",
			input: `{"schema_version":1,"audited_at":"2026-07-19","records":[` + miniAuditRecord(
				"func:Do",
			) + `,` + miniAuditRecord(
				"func:Do",
			) + `]}`,
			wantErr: true,
		},
		{
			name: "unsorted evidence",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"downstream_evidence":["downstream: no indexed consumers","in-repo: internal/cmd/x"]`,
				`"downstream_evidence":["z","a"]`,
				1,
			),
			wantErr: true,
		},
		{
			name: "duplicate evidence",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"downstream_evidence":["downstream: no indexed consumers","in-repo: internal/cmd/x"]`,
				`"downstream_evidence":["a","a"]`,
				1,
			),
			wantErr: true,
		},
		{
			name:    "id contradicts kind",
			input:   strings.Replace(miniAudit, `"kind":"const"`, `"kind":"type"`, 1),
			wantErr: true,
		},
		{
			name: "member record with an empty owner",
			input: `{"schema_version":1,"audited_at":"2026-07-19","records":[` + miniLeakRecord(
				"field:Labels.Environment", "field", "", "Environment", "string",
			) + `]}`,
			wantErr: true,
		},
		{
			name: "leak internal_target with a trailing slash",
			input: strings.Replace(
				`{"schema_version":1,"audited_at":"2026-07-19","records":[`+leak+`]}`,
				`"internal_target":"internal/fixture"`,
				`"internal_target":"internal/fixture/"`,
				1,
			),
			wantErr: true,
		},
		{name: "oversize", input: oversize, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := parseAudit([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseAudit(%s) succeeded, want error", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAudit(%s): %v", tc.name, err)
			}
			if doc == nil || doc.SchemaVersion != 1 {
				t.Fatalf("doc = %+v, want schema_version 1", doc)
			}
		})
	}
}

func miniAuditRecord(id string) string {
	kind := strings.SplitN(id, ":", 2)[0]
	name := strings.SplitN(id, ":", 2)[1]
	sig := "func()"
	if kind == "const" {
		sig = "untyped int"
	}
	return `{"id":"` + id + `","kind":"` + kind + `","owner":"","name":"` + name + `","signature":"` + sig + `",` +
		`"classification":"supported","rationale":"Fixture feature.","disposition":"keep-public","internal_target":"","replacement":"",` +
		`"compatibility_strategy":"Keep the public selector unchanged.","lifecycle":"live","first_published":"",` +
		`"deprecated_in":"","removed_in":"","downstream_checked_at":"","downstream_evidence":[]}`
}

// identityRecord builds a record whose non-identity fields are already valid,
// so an identity assertion fails only for the identity reason under test.
func identityRecord(id, kind, owner, name string) auditRecord {
	return auditRecord{
		ID:        id,
		Kind:      kind,
		Owner:     owner,
		Name:      name,
		Signature: "func()",
		Rationale: "Fixture feature.",
	}
}

func TestValidateRecordIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		record  auditRecord
		wantErr bool
	}{
		{name: "package scope const", record: identityRecord("const:Answer", kindConst, "", "Answer")},
		{name: "package scope var", record: identityRecord("var:Default", kindVar, "", "Default")},
		{name: "package scope func", record: identityRecord("func:Execute", kindFunc, "", "Execute")},
		{name: "package scope type", record: identityRecord("type:Logger", kindType, "", "Logger")},
		{
			name:   "field member",
			record: identityRecord("field:Labels.Environment", kindField, "Labels", "Environment"),
		},
		{
			name:   "interface method member",
			record: identityRecord("interface-method:Logger.Info", kindInterfaceMethod, "Logger", "Info"),
		},
		{name: "value receiver method", record: identityRecord("method:Mode.String", kindMethod, "Mode", "String")},
		{
			name:   "pointer receiver method",
			record: identityRecord("method:*Error.Unwrap", kindMethod, "Error", "Unwrap"),
		},
		{
			name:    "kind contradicts id prefix",
			record:  identityRecord("func:Execute", kindType, "", "Execute"),
			wantErr: true,
		},
		{
			name:    "name contradicts id",
			record:  identityRecord("func:Execute", kindFunc, "", "Run"),
			wantErr: true,
		},
		{
			name:    "member id with empty owner",
			record:  identityRecord("field:Labels.Environment", kindField, "", "Environment"),
			wantErr: true,
		},
		{
			name:    "owner contradicts id",
			record:  identityRecord("field:Labels.Environment", kindField, "Metadata", "Environment"),
			wantErr: true,
		},
		{
			name:    "package scope kind with spurious owner",
			record:  identityRecord("func:Labels.Execute", kindFunc, "Labels", "Execute"),
			wantErr: true,
		},
		{
			name:    "pointer marker on a non-method kind",
			record:  identityRecord("field:*Labels.Environment", kindField, "Labels", "Environment"),
			wantErr: true,
		},
		{
			name:    "pointer marker misplaced on a method",
			record:  identityRecord("method:Error.*Unwrap", kindMethod, "Error", "Unwrap"),
			wantErr: true,
		},
		{name: "unknown kind", record: identityRecord("thing:Execute", "thing", "", "Execute"), wantErr: true},
		{name: "empty id", record: identityRecord("", kindFunc, "", "Execute"), wantErr: true},
		{name: "empty name", record: identityRecord("func:", kindFunc, "", ""), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateRecordIdentity(tc.record)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateRecordIdentity(%+v) succeeded, want error", tc.record)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateRecordIdentity(%+v): %v", tc.record, err)
			}
		})
	}
}

func TestValidRelocationTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		target string
		want   bool
	}{
		{target: "internal/config", want: true},
		{target: "internal/telemetry/otel", want: true},
		{target: "internal/cmd/benchcheck", want: true},
		{target: "internal/mcp_server", want: true},
		{target: "internal/id-gen", want: true},
		{target: "internal/x9", want: true},
		{target: ""},
		{target: "internal"},
		{target: "internal/"},
		{target: "internal/config/"},
		{target: "internal//config"},
		{target: "internal/config//otel"},
		{target: "internal/-config"},
		{target: "internal/_config"},
		{target: "internal/config-"},
		{target: "internal/config__otel"},
		{target: "internal/config/-otel"},
		{target: "internal/Config"},
		{target: "config"},
		{target: "internalx/config"},
		{target: "internal/helpers"},
		{target: "internal/helpers/json"},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			if got := validRelocationTarget(tc.target); got != tc.want {
				t.Fatalf("validRelocationTarget(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestDiffBaseline(t *testing.T) {
	t.Parallel()
	live := []Feature{
		{ID: "const:Answer", Kind: "const", Name: "Answer", Signature: "untyped int"},
		{ID: "func:Added", Kind: "func", Name: "Added", Signature: "func()"},
		{ID: "func:Changed", Kind: "func", Name: "Changed", Signature: "func() string"},
	}
	base := &baselineDoc{SchemaVersion: 1, Features: []baselineFeature{
		{ID: "const:Answer", Signature: "untyped int"},
		{ID: "func:Changed", Signature: "func() int"},
		{ID: "func:Removed", Signature: "func()"},
	}}
	drift := diffBaseline(live, base)
	want := []DriftItem{
		{ID: "func:Added", Drift: "added", Expected: "", Actual: "func()"},
		{ID: "func:Changed", Drift: "signature-changed", Expected: "func() int", Actual: "func() string"},
		{ID: "func:Removed", Drift: "missing", Expected: "func()", Actual: ""},
	}
	assertDrift(t, drift, want)
}

func TestCrossValidateAudit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		base  *baselineDoc
		audit *auditDoc
		want  []DriftItem
	}{
		{
			name:  "audit row missing for baseline id",
			base:  &baselineDoc{SchemaVersion: 1, Features: []baselineFeature{{ID: "func:Do", Signature: "func()"}}},
			audit: &auditDoc{SchemaVersion: 1, AuditedAt: "2026-07-19", Records: []auditRecord{}},
			want: []DriftItem{
				{ID: "func:Do", Drift: "audit-missing", Expected: "active audit record", Actual: "absent"},
			},
		},
		{
			name: "active audit row absent from baseline",
			base: &baselineDoc{SchemaVersion: 1, Features: []baselineFeature{}},
			audit: &auditDoc{
				SchemaVersion: 1,
				AuditedAt:     "2026-07-19",
				Records:       []auditRecord{{ID: "func:Do", Lifecycle: "live"}},
			},
			want: []DriftItem{
				{
					ID:       "func:Do",
					Drift:    "audit-state-invalid",
					Expected: "absent from live baseline or transitioned to removed",
					Actual:   "active audit record",
				},
			},
		},
		{
			name: "removed audit row present in baseline",
			base: &baselineDoc{SchemaVersion: 1, Features: []baselineFeature{{ID: "func:Do", Signature: "func()"}}},
			audit: &auditDoc{
				SchemaVersion: 1,
				AuditedAt:     "2026-07-19",
				Records:       []auditRecord{{ID: "func:Do", Lifecycle: "removed"}},
			},
			want: []DriftItem{
				{
					ID:       "func:Do",
					Drift:    "audit-state-invalid",
					Expected: "absent from live baseline",
					Actual:   "removed audit record",
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertDrift(t, crossValidateAudit(tc.base, tc.audit), tc.want)
		})
	}
}

func TestDeprecationGaps(t *testing.T) {
	t.Parallel()
	const src = `package fix

// Old does things.
//
// Deprecated: Use New. Removal is eligible only after a published pre-v1 minor release carries this notice.
func Old() {}

func New() {}

// Legacy is retained.
type Legacy struct{}

// OldMethod does things.
//
// Deprecated: Use New. Removal is eligible only after a published pre-v1 minor release carries this notice.
func (Legacy) OldMethod() {}
`
	tests := []struct {
		name string
		src  string
		rows []auditRecord
		inv  []Feature
		want []DriftItem
	}{
		{
			name: "notices present",
			src:  src,
			rows: []auditRecord{
				{ID: "func:Old", Lifecycle: "deprecated"},
				{ID: "method:Legacy.OldMethod", Lifecycle: "deprecated"},
			},
			want: nil,
		},
		{
			name: "missing func notice",
			src:  `package fix\n\nfunc Old() {}\n`,
			rows: []auditRecord{{ID: "func:Old", Lifecycle: "deprecated"}},
			want: []DriftItem{
				{
					ID:       "func:Old",
					Drift:    "deprecation-missing",
					Expected: "Deprecated: paragraph on the source declaration",
					Actual:   "absent",
				},
			},
		},
		{
			name: "declaration absent from source",
			src:  `package fix\n\nfunc Other() {}\n`,
			rows: []auditRecord{{ID: "func:Old", Lifecycle: "deprecated"}},
			want: []DriftItem{
				{
					ID:       "func:Old",
					Drift:    "deprecation-missing",
					Expected: "Deprecated: paragraph on the source declaration",
					Actual:   "absent",
				},
			},
		},
		{
			name: "live rows need no notice",
			src:  `package fix\n\nfunc Old() {}\n`,
			rows: []auditRecord{{ID: "func:Old", Lifecycle: "live"}},
			want: nil,
		},
		{
			name: "promoted and alias selectors use declaration notices",
			src: `package fix

type Inner struct {
	// Deprecated: Use NewX.
	X int
}

// Deprecated: Use NewM.
func (Inner) M() {}

type Outer struct{ Inner }

type Alias = Inner
`,
			rows: []auditRecord{
				{ID: "field:Alias.X", Lifecycle: "deprecated"},
				{ID: "field:Outer.X", Lifecycle: "deprecated"},
				{ID: "method:Alias.M", Lifecycle: "deprecated"},
				{ID: "method:Outer.M", Lifecycle: "deprecated"},
			},
			inv: []Feature{
				{ID: "field:Alias.X", sourceID: "field:Inner.X"},
				{ID: "field:Outer.X", sourceID: "field:Inner.X"},
				{ID: "method:Alias.M", sourceID: "method:Inner.M"},
				{ID: "method:Outer.M", sourceID: "method:Inner.M"},
			},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src := strings.ReplaceAll(tc.src, `\n`, "\n")
			if err := os.WriteFile(filepath.Join(dir, "fix.go"), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			audit := &auditDoc{SchemaVersion: 1, AuditedAt: "2026-07-19", Records: tc.rows}
			drift, err := deprecationGaps(dir, audit, []profileScan{{
				profile:  Profile{GOOS: goosLinux, GOARCH: goarchAMD64},
				features: tc.inv,
				files:    []string{"fix.go"},
			}})
			if err != nil {
				t.Fatalf("deprecationGaps: %v", err)
			}
			assertDrift(t, drift, tc.want)
		})
	}
}

func TestDeprecationGapsRequireNoticeInEachBuildProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		"fix_nonwindows.go": `//go:build !windows

package fix

func Old() {}
`,
		"fix_windows.go": `//go:build windows

package fix

// Deprecated: Use New.
func Old() {}
`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	gomod := []byte("module fixture/deprecationprofiles\n\ngo 1.26\n")
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), gomod, 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	profiles := []Profile{
		{GOOS: goosLinux, GOARCH: goarchAMD64},
		{GOOS: goosWindows, GOARCH: goarchAMD64},
	}
	scans, err := scanAll(context.Background(), dir, profiles)
	if err != nil {
		t.Fatalf("scanAll: %v", err)
	}
	if _, profileDrift := reconcile(scans, profiles); len(profileDrift) != 0 {
		t.Fatalf("profile drift = %v, want invariant surface", profileDrift)
	}
	audit := &auditDoc{Records: []auditRecord{{ID: "func:Old", Lifecycle: lifecycleDeprecated}}}
	drift, err := deprecationGaps(dir, audit, scans)
	if err != nil {
		t.Fatalf("deprecationGaps: %v", err)
	}
	assertDrift(t, drift, []DriftItem{{
		ID:       "func:Old",
		Drift:    driftDeprecationMissing,
		Expected: detailDeprecationNotice,
		Actual:   detailAbsent,
	}})
}

func TestValidateLifecycleReleaseTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		record  auditRecord
		wantErr bool
	}{
		{name: "live without tags", record: auditRecord{Lifecycle: "live"}},
		{name: "deprecated without tags", record: auditRecord{Lifecycle: "deprecated"}},
		{
			name:   "removable with deprecated tag",
			record: auditRecord{Lifecycle: "removable", DeprecatedIn: "v0.4.0"},
		},
		{
			name:   "removed with both tags",
			record: auditRecord{Lifecycle: "removed", DeprecatedIn: "v0.4.0", RemovedIn: "v0.5.0"},
		},
		{
			name:    "live with deprecated tag",
			record:  auditRecord{Lifecycle: "live", DeprecatedIn: "v0.4.0"},
			wantErr: true,
		},
		{
			name:    "live with removed tag",
			record:  auditRecord{Lifecycle: "live", RemovedIn: "v0.5.0"},
			wantErr: true,
		},
		{
			name:    "deprecated with deprecated tag",
			record:  auditRecord{Lifecycle: "deprecated", DeprecatedIn: "v0.4.0"},
			wantErr: true,
		},
		{
			name:    "deprecated with removed tag",
			record:  auditRecord{Lifecycle: "deprecated", RemovedIn: "v0.5.0"},
			wantErr: true,
		},
		{
			name:    "removable without deprecated tag",
			record:  auditRecord{Lifecycle: "removable"},
			wantErr: true,
		},
		{
			name: "removable with removed tag",
			record: auditRecord{
				Lifecycle:    "removable",
				DeprecatedIn: "v0.4.0",
				RemovedIn:    "v0.5.0",
			},
			wantErr: true,
		},
		{
			name:    "removed without deprecated tag",
			record:  auditRecord{Lifecycle: "removed", RemovedIn: "v0.5.0"},
			wantErr: true,
		},
		{
			name:    "removed without removed tag",
			record:  auditRecord{Lifecycle: "removed", DeprecatedIn: "v0.4.0"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateLifecycle(tc.record)
			if tc.wantErr && err == nil {
				t.Fatal("validateLifecycle succeeded, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateLifecycle: %v", err)
			}
		})
	}
}

func assertDrift(t *testing.T, got, want []DriftItem) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("drift = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("drift %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func FuzzParseBaseline(f *testing.F) {
	f.Add(miniBaseline)
	f.Add(`{}`)
	f.Add(`{"schema_version":1}`)
	f.Add(`not json`)
	f.Add(`{"schema_version":1,"features":null}`)
	f.Fuzz(func(t *testing.T, data string) {
		doc, err := parseBaseline([]byte(data))
		if err == nil && doc == nil {
			t.Fatal("nil doc without error")
		}
	})
}

func FuzzParseAudit(f *testing.F) {
	f.Add(miniAudit)
	f.Add(`{}`)
	f.Add(`{"schema_version":1,"audited_at":"2026-07-19"}`)
	f.Add(`not json`)
	f.Fuzz(func(t *testing.T, data string) {
		doc, err := parseAudit([]byte(data))
		if err == nil && doc == nil {
			t.Fatal("nil doc without error")
		}
	})
}

// --- CLI stream/exit/golden tests (T004) ---

func runMini(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	cfg := runConfig{Dir: dir, Profiles: []Profile{{GOOS: "linux", GOARCH: "amd64"}}}
	var out, errOut bytes.Buffer
	code := run(context.Background(), cfg, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func assertGolden(t *testing.T, got, name string) {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("output mismatch:\n got: %q\nwant: %q", got, string(want))
	}
}

func decodeEnvelope(t *testing.T, stderr string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(stderr), &env); err != nil {
		t.Fatalf("stderr is not one JSON envelope: %q (%v)", stderr, err)
	}
	return env
}

func writeBaselineAudit(t *testing.T, baseline, audit string) (string, string) {
	t.Helper()
	return writeArtifact(t, baseline), writeArtifact(t, audit)
}

func TestRunPass(t *testing.T) {
	dir := fixtureDir(t, "mini")
	base, audit := writeBaselineAudit(t, miniBaseline, miniAudit)
	code, out, errOut := runMini(t, dir, "-baseline", base, "-audit", audit)
	if code != 0 || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q, want 0 with empty stderr", code, errOut)
	}
	assertGolden(t, out, "check_pass.golden.json")
}

func TestRunList(t *testing.T) {
	dir := fixtureDir(t, "mini")
	code, out, errOut := runMini(t, dir, "-list")
	if code != 0 || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q, want 0 with empty stderr", code, errOut)
	}
	assertGolden(t, out, "list_mini.golden.json")
}

func TestRunAuditSeed(t *testing.T) {
	dir := fixtureDir(t, "mini")
	code, out, errOut := runMini(t, dir, "-audit-seed")
	if code != 0 || errOut != "" {
		t.Fatalf("exit = %d, stderr = %q, want 0 with empty stderr", code, errOut)
	}
	assertGolden(t, out, "audit_seed_mini.golden.json")
	if _, err := parseAudit([]byte(out)); err == nil {
		t.Fatal("audit seed must remain intentionally invalid until reviewed")
	}
}

func TestRunDrift(t *testing.T) {
	dir := fixtureDir(t, "mini")
	baseOnly := `{"schema_version":1,"features":[{"id":"const:Answer","signature":"untyped int"}]}`
	auditOnly := `{"schema_version":1,"audited_at":"2026-07-19","records":[` + miniAuditRecord("const:Answer") + `]}`
	base, audit := writeBaselineAudit(t, baseOnly, auditOnly)
	code, out, errOut := runMini(t, dir, "-baseline", base, "-audit", audit)
	if code != 2 || out != "" {
		t.Fatalf("exit = %d, stdout = %q, want exit 2 with empty stdout", code, out)
	}
	assertGolden(t, errOut, "check_drift.golden.json")
}

func TestRunInvalidArtifacts(t *testing.T) {
	t.Parallel()
	dir := fixtureDir(t, "mini")
	tests := []struct {
		name     string
		baseline string
		audit    string
		args     []string
		wantCode int
	}{
		{name: "malformed baseline", baseline: `{"schema_version":1,"features":[}`, audit: miniAudit, wantCode: 2},
		{
			name:     "unknown baseline field",
			baseline: `{"schema_version":1,"features":[],"x":1}`,
			audit:    miniAudit,
			wantCode: 2,
		},
		{name: "missing baseline file", baseline: "", audit: miniAudit, wantCode: 2},
		{name: "unknown flag", args: []string{"-bogus"}, wantCode: 2},
		{name: "positional argument", args: []string{"extra"}, wantCode: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := tc.args
			if args == nil {
				basePath := filepath.Join(t.TempDir(), "nonexistent.json")
				if tc.baseline != "" {
					basePath = writeArtifact(t, tc.baseline)
				}
				args = []string{"-baseline", basePath, "-audit", writeArtifact(t, tc.audit)}
			}
			code, out, errOut := runMini(t, dir, args...)
			if code != tc.wantCode || out != "" {
				t.Fatalf("exit = %d, stdout = %q, want exit %d with empty stdout", code, out, tc.wantCode)
			}
			if strings.Count(errOut, "\n") != 1 {
				t.Fatalf("stderr must be exactly one envelope line, got %q", errOut)
			}
			env := decodeEnvelope(t, errOut)
			if env["error_code"] != "invalid_surface_artifact" {
				t.Fatalf("error_code = %v, want invalid_surface_artifact", env["error_code"])
			}
		})
	}
}

func TestRunPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission denial is not observable as root")
	}
	dir := fixtureDir(t, "mini")
	base := writeArtifact(t, miniBaseline)
	if err := os.Chmod(base, 0o200); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	code, out, errOut := runMini(t, dir, "-baseline", base, "-audit", writeArtifact(t, miniAudit))
	if code != 4 || out != "" {
		t.Fatalf("exit = %d, stdout = %q, want exit 4 with empty stdout", code, out)
	}
	env := decodeEnvelope(t, errOut)
	if env["error_code"] != "surface_permission" {
		t.Fatalf("error_code = %v, want surface_permission", env["error_code"])
	}
}

func TestRunDeterminism(t *testing.T) {
	dir := fixtureDir(t, "mini")
	base, audit := writeBaselineAudit(t, miniBaseline, miniAudit)
	args := []string{"-baseline", base, "-audit", audit}
	_, out1, _ := runMini(t, dir, args...)
	_, out2, _ := runMini(t, dir, args...)
	if out1 != out2 {
		t.Fatalf("repeated runs differ:\n%q\n%q", out1, out2)
	}
	_, list1, _ := runMini(t, dir, "-list")
	_, list2, _ := runMini(t, dir, "-list")
	if list1 != list2 {
		t.Fatalf("repeated scans differ:\n%q\n%q", list1, list2)
	}
}
