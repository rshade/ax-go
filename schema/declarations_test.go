package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/mcp"
	internalschema "github.com/rshade/ax-go/internal/schema"
	"github.com/rshade/ax-go/internal/testutil"
)

func TestDeclareRejectsInvalidInputWithEnvelope(t *testing.T) {
	cases := []struct {
		name      string
		declare   func() error
		wantField string
		wantRsn   string
	}{
		{
			name:      "prompt on nil command",
			declare:   func() error { return DeclarePrompt(nil, Prompt{Name: "p", Template: "t"}) },
			wantField: "cmd",
			wantRsn:   "nil_command",
		},
		{
			name: "prompt with undeclared placeholder",
			declare: func() error {
				return DeclarePrompt(&cobra.Command{Use: "app"}, Prompt{Name: "p", Template: "{{x}}"})
			},
			wantField: "template",
			wantRsn:   "undeclared_placeholder",
		},
		{
			name:      "resource on nil command",
			declare:   func() error { return DeclareResource(nil, Resource{URI: "app://x", Name: "x"}) },
			wantField: "cmd",
			wantRsn:   "nil_command",
		},
		{
			name: "relative resource URI",
			declare: func() error {
				return DeclareResource(&cobra.Command{Use: "app"}, Resource{URI: "docs/pricing", Name: "x"})
			},
			wantField: "uri",
			wantRsn:   "not_absolute",
		},
		{
			name: "prompt over a hand-corrupted annotation",
			declare: func() error {
				cmd := &cobra.Command{Use: "app", Annotations: map[string]string{
					"github.com/rshade/ax-go/schema/prompts": "{not json",
				}}
				return DeclarePrompt(cmd, Prompt{Name: "p", Template: "t"})
			},
			wantField: "annotation",
			wantRsn:   "corrupt_annotation",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.declare()
			var envelope *contract.Error
			if !errors.As(err, &envelope) {
				t.Fatalf("error = %T %v, want *contract.Error", err, err)
			}
			if envelope.ErrorCode != "invalid_schema_declaration" {
				t.Fatalf("ErrorCode = %q, want invalid_schema_declaration", envelope.ErrorCode)
			}
			if got := contract.ErrorExitCode(err); got != contract.ExitValidation {
				t.Fatalf("exit code = %d, want %d", got, contract.ExitValidation)
			}
			if envelope.Context["field"] != tc.wantField || envelope.Context["reason"] != tc.wantRsn {
				t.Fatalf("context = %v, want field %q reason %q", envelope.Context, tc.wantField, tc.wantRsn)
			}
			if envelope.ActionableFix == "" {
				t.Fatal("invalid_schema_declaration carries no actionable_fix")
			}
		})
	}
}

func TestDeclarationErrorBoundsEchoedKey(t *testing.T) {
	uri := "app://" + strings.Repeat("a", 1<<20)
	err := DeclareResource(&cobra.Command{Use: "app"}, Resource{URI: uri, Name: "x"})
	if err == nil {
		t.Fatal("DeclareResource accepted a 1 MiB URI")
	}
	if got := len(err.Error()); got > 512 {
		t.Fatalf("error message is %d bytes; an oversized URI must not be echoed in full", got)
	}
}

// TestPromptConvertersCarryEveryField guards the field-by-field Prompt
// converters: a field added to the public, internal, and MCP Prompt types
// compiles without touching them, so this fails unless every field round-trips
// into both projections.
func TestPromptConvertersCarryEveryField(t *testing.T) {
	prompt := Prompt{Arguments: []PromptArgument{{}}}
	fillStrings(reflect.ValueOf(&prompt).Elem())

	root := &cobra.Command{Use: "app"}
	prompt.Template = "uses {{" + prompt.Arguments[0].Name + "}}"
	mustDeclare(t, DeclarePrompt(root, prompt))

	if got := BuildSchema(root).Command.Prompts; !reflect.DeepEqual(got, []Prompt{prompt}) {
		t.Fatalf("ax-native projection lost a field:\ngot:  %+v\nwant: %+v", got, prompt)
	}
	mcpPrompt := BuildMCPSchema(root).Prompts[0]
	assertSameFields(t, reflect.ValueOf(prompt), reflect.ValueOf(mcpPrompt))
}

func fillStrings(v reflect.Value) {
	for i := range v.NumField() {
		field := v.Field(i)
		switch kind := field.Kind(); {
		case kind == reflect.String:
			field.SetString("x" + v.Type().Field(i).Name)
		case kind == reflect.Bool:
			field.SetBool(true)
		case kind == reflect.Slice:
			for j := range field.Len() {
				fillStrings(field.Index(j))
			}
		}
	}
}

func assertSameFields(t *testing.T, want, got reflect.Value) {
	t.Helper()
	if want.NumField() != got.NumField() {
		t.Fatalf("%s has %d fields, %s has %d", want.Type(), want.NumField(), got.Type(), got.NumField())
	}
	for i := range want.NumField() {
		name := want.Type().Field(i).Name
		gotField := got.FieldByName(name)
		if !gotField.IsValid() {
			t.Fatalf("%s.%s has no counterpart in %s", want.Type(), name, got.Type())
		}
		if want.Field(i).Kind() == reflect.Slice {
			if want.Field(i).Len() != gotField.Len() {
				t.Fatalf("%s.%s length differs", want.Type(), name)
			}
			for j := range want.Field(i).Len() {
				assertSameFields(t, want.Field(i).Index(j), gotField.Index(j))
			}
			continue
		}
		if want.Field(i).Interface() != gotField.Interface() {
			t.Fatalf("%s.%s = %v in the MCP projection, want %v", want.Type(), name, gotField, want.Field(i))
		}
	}
}

// newDeclarationTestCommand builds the SC-002 fixture: declarations on a
// runnable root, a non-runnable group, an MCP-excluded leaf, and a hidden
// subtree whose declarations must never be projected.
func newDeclarationTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	root := newSchemaTestCommand()
	group := &cobra.Command{Use: "reports", Short: "report commands"}
	serve := &cobra.Command{
		Use:   "serve",
		Short: "serve forever",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	hidden := &cobra.Command{Use: "debug", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
	mcp.MarkExcluded(serve)
	group.AddCommand(serve)
	root.AddCommand(group, hidden)

	mustDeclare(t, DeclarePrompt(root, Prompt{
		Name:        "triage-spike",
		Title:       "Triage a cost spike",
		Description: "Find and explain the top cost driver.",
		Arguments: []PromptArgument{
			{Name: "window", Title: "Window", Description: "lookback, e.g. 7d", Required: true},
			{Name: "team", Description: "optional owning team"},
		},
		Template: "Run `app run --name {{window}}`, then summarize for {{team}}.",
	}))
	mustDeclare(t, DeclareResource(root, Resource{
		URI:         "app://docs/pricing-model",
		Name:        "pricing-model",
		Title:       "Pricing model",
		Description: "How app prices line items.",
		MIMEType:    "text/markdown",
	}))
	mustDeclare(t, DeclarePrompt(group, Prompt{Name: "weekly-report", Template: "Run every reports command."}))
	mustDeclare(t, DeclareResource(serve, Resource{URI: "app://docs/serve", Name: "serve-guide"}))
	mustDeclare(t, DeclarePrompt(hidden, Prompt{Name: "debug-only", Template: "never projected"}))
	mustDeclare(t, DeclareResource(hidden, Resource{URI: "app://debug", Name: "debug"}))
	return root
}

func mustDeclare(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("declaration rejected: %v", err)
	}
}

func runSchema(t *testing.T, root *cobra.Command, args ...string) ([]byte, error) {
	t.Helper()
	cmd := NewSchemaCommand(root, WithSchemaVersion("v0.1.0"))
	// Execute silences these on the real root; standalone, Cobra would
	// print usage to stdout on error and mask the stream contract.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.Bytes(), err
}

func TestSchemaProjectsDeclarationsGolden(t *testing.T) {
	cases := []struct {
		format string
		golden string
	}{
		{format: "--as=ax", golden: "schema_ax_declarations.golden.json"},
		{format: "--as=mcp", golden: "schema_mcp_declarations.golden.json"},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			stdout, err := runSchema(t, newDeclarationTestCommand(t), tc.format)
			if err != nil {
				t.Fatalf("__schema %s: %v", tc.format, err)
			}
			assertGolden(t, filepath.Join("..", "testdata", tc.golden), stdout)
		})
	}
}

func TestBuildSchemaPlacesDeclarationsOnDeclaringCommand(t *testing.T) {
	schema := BuildSchema(newDeclarationTestCommand(t))

	if len(schema.Command.Prompts) != 1 || schema.Command.Prompts[0].Name != "triage-spike" {
		t.Fatalf("root prompts = %+v, want triage-spike only", schema.Command.Prompts)
	}
	for _, child := range schema.Command.Commands {
		if child.Use == "run" && (child.Prompts != nil || child.Resources != nil) {
			t.Fatalf("undeclaring command carries declarations: %+v", child)
		}
	}
}

func TestSchemaFailsClosedOnDuplicateDeclaration(t *testing.T) {
	cases := []struct {
		name    string
		declare func(t *testing.T, first, second *cobra.Command)
		kept    func(MCPSchema) string
		golden  string
	}{
		{
			name: "prompt",
			declare: func(t *testing.T, first, second *cobra.Command) {
				mustDeclare(t, DeclarePrompt(first, Prompt{Name: "triage", Template: "first"}))
				mustDeclare(t, DeclarePrompt(second, Prompt{Name: "triage", Template: "second"}))
			},
			kept:   func(s MCPSchema) string { return s.Prompts[0].Template },
			golden: "schema_duplicate_declaration.golden.json",
		},
		{
			name: "resource",
			declare: func(t *testing.T, first, second *cobra.Command) {
				mustDeclare(t, DeclareResource(first, Resource{URI: "app://docs", Name: "first"}))
				mustDeclare(t, DeclareResource(second, Resource{URI: "app://docs", Name: "second"}))
			},
			kept:   func(s MCPSchema) string { return s.Resources[0].Name },
			golden: "schema_duplicate_resource_declaration.golden.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newSchemaTestCommand()
			tc.declare(t, root, root.Commands()[0])
			assertSchemaFailsClosed(t, root, tc.golden)

			if got := tc.kept(BuildMCPSchema(root)); got != "first" {
				t.Fatalf("BuildMCPSchema kept %q, want the first declaration in walk order", got)
			}
			if child := BuildSchema(root).Command.Commands[0]; child.Prompts != nil || child.Resources != nil {
				t.Fatalf("BuildSchema kept the second declaration on %q", child.Use)
			}
		})
	}
}

func assertSchemaFailsClosed(t *testing.T, root *cobra.Command, golden string) {
	t.Helper()

	for _, format := range []string{"--as=ax", "--as=mcp"} {
		stdout, err := runSchema(t, root, format)
		if len(stdout) != 0 {
			t.Fatalf("%s wrote stdout on a failed declaration check: %s", format, stdout)
		}
		if got := contract.ErrorExitCode(err); got != contract.ExitValidation {
			t.Fatalf("%s exit code = %d, want %d (err %v)", format, got, contract.ExitValidation, err)
		}
		var stderr bytes.Buffer
		if writeErr := contract.WriteError(&stderr, err); writeErr != nil {
			t.Fatalf("WriteError: %v", writeErr)
		}
		assertGolden(t, filepath.Join("..", "testdata", golden), testutil.MaskNonDeterministic(stderr.Bytes()))
	}
}

func TestSchemaFailsClosedOnCorruptAnnotation(t *testing.T) {
	root := newSchemaTestCommand()
	root.Commands()[0].Annotations = map[string]string{
		"github.com/rshade/ax-go/schema/resources": `[{"uri":"relative","name":"hand-written"}]`,
	}
	assertSchemaFailsClosed(t, root, "schema_corrupt_declaration.golden.json")
}

// TestDeclarationProjectionsAgreeUnderHiddenRoot pins format parity when the
// root itself is hidden: BuildCommand keeps a hidden root, so its declarations
// must reach both projections and the duplicate check.
func TestDeclarationProjectionsAgreeUnderHiddenRoot(t *testing.T) {
	root := newSchemaTestCommand()
	root.Hidden = true
	mustDeclare(t, DeclarePrompt(root, Prompt{Name: "root-prompt", Template: "t"}))
	mustDeclare(t, DeclareResource(root.Commands()[0], Resource{URI: "app://run", Name: "run"}))

	ax, mcpSchema := BuildSchema(root), BuildMCPSchema(root)
	if len(ax.Command.Prompts) != 1 || len(mcpSchema.Prompts) != 1 || len(mcpSchema.Resources) != 1 {
		t.Fatalf("projections disagree: ax prompts %+v, mcp prompts %+v, mcp resources %+v",
			ax.Command.Prompts, mcpSchema.Prompts, mcpSchema.Resources)
	}

	mustDeclare(t, DeclarePrompt(root.Commands()[0], Prompt{Name: "root-prompt", Template: "u"}))
	if _, err := runSchema(t, root, "--as=mcp"); contract.ErrorExitCode(err) != contract.ExitValidation {
		t.Fatalf("duplicate under a hidden root: err = %v, want validation_error exit 2", err)
	}
}

func TestSchemaDeclarationOutputIsDeterministic(t *testing.T) {
	for _, format := range []string{"--as=ax", "--as=mcp"} {
		want, err := runSchema(t, newDeclarationTestCommand(t), format)
		if err != nil {
			t.Fatalf("__schema %s: %v", format, err)
		}
		for run := range 10 {
			got, err := runSchema(t, newDeclarationTestCommand(t), format)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("%s run %d differs (err %v)\ngot:  %s\nwant: %s", format, run, err, got, want)
			}
		}
	}
}

// TestDeclarationTypesCannotCarryLiveState enforces the constitutional
// narrowing on prompts and resources (Principle VI): they are static,
// read-only declarations, so their types may hold only strings, bools, and
// slices of such structs. A func, chan, pointer, map, or interface field would
// make run-record or live state representable and must fail this test.
func TestDeclarationTypesCannotCarryLiveState(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeFor[Prompt](),
		reflect.TypeFor[PromptArgument](),
		reflect.TypeFor[Resource](),
		reflect.TypeFor[MCPPrompt](),
		reflect.TypeFor[MCPPromptArgument](),
		reflect.TypeFor[MCPResource](),
	} {
		assertStaticShape(t, typ, typ.Name())
	}
}

func assertStaticShape(t *testing.T, typ reflect.Type, path string) {
	t.Helper()
	for field := range typ.Fields() {
		fieldPath := path + "." + field.Name
		kind := field.Type.Kind()
		if kind == reflect.String || kind == reflect.Bool {
			continue
		}
		if kind != reflect.Slice {
			t.Errorf(
				"%s has kind %s; prompts and resources must be static, read-only declarations (Constitution VI)",
				fieldPath,
				kind,
			)
			continue
		}
		elem := field.Type.Elem()
		if elem.Kind() != reflect.Struct {
			t.Errorf("%s is a slice of %s; declarations may only hold slices of static structs", fieldPath, elem.Kind())
			continue
		}
		assertStaticShape(t, elem, fieldPath+"[]")
	}
}

func TestDeclarationsAreCapturedByValue(t *testing.T) {
	root := newSchemaTestCommand()
	prompt := Prompt{
		Name:      "triage",
		Arguments: []PromptArgument{{Name: "window", Required: true}},
		Template:  "since {{window}}",
	}
	resource := Resource{URI: "app://docs", Name: "docs"}
	mustDeclare(t, DeclarePrompt(root, prompt))
	mustDeclare(t, DeclareResource(root, resource))
	want, err := runSchema(t, root, "--as=mcp")
	if err != nil {
		t.Fatalf("__schema: %v", err)
	}

	prompt.Arguments[0].Name = "mutated"
	prompt.Template = "mutated"
	resource.URI = "app://mutated"

	got, err := runSchema(t, root, "--as=mcp")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("projection changed after caller mutation (err %v)\ngot:  %s\nwant: %s", err, got, want)
	}
}

// TestEveryDeclarationReasonHasAFix keeps actionable_fix populated: a reason
// added without a remedy would ship invalid_schema_declaration envelopes that
// tell an agent nothing about how to recover.
func TestEveryDeclarationReasonHasAFix(t *testing.T) {
	for _, reason := range []internalschema.Reason{
		internalschema.ReasonNilCommand,
		internalschema.ReasonRequired,
		internalschema.ReasonInvalidCharset,
		internalschema.ReasonInvalidUTF8,
		internalschema.ReasonDuplicate,
		internalschema.ReasonUndeclaredPlaceholder,
		internalschema.ReasonNotAbsolute,
		internalschema.ReasonMalformed,
		internalschema.ReasonTooLong,
		internalschema.ReasonInvalidCharacter,
		internalschema.ReasonCorruptAnnotation,
		internalschema.ReasonFlagNotFound,
		internalschema.ReasonUnsupportedType,
		internalschema.ReasonInvalidValue,
		internalschema.ReasonNotInEnum,
		internalschema.ReasonNotVocabulary,
	} {
		if declarationFix(reason, "") == "" {
			t.Errorf("reason %q has no actionable_fix", reason)
		}
	}
	if fix := declarationFix("unknown_reason", ""); fix != "" {
		t.Errorf("unknown reason fix = %q, want empty", fix)
	}
}

func TestResourceContentIsNeverProjected(t *testing.T) {
	root := newSchemaTestCommand()
	mustDeclare(t, DeclareResource(root, Resource{
		URI: "app://docs/x", Name: "x", MIMEType: "text/markdown", Content: "SECRET-BODY",
	}))

	for _, args := range [][]string{nil, {"--as=mcp"}} {
		out, err := runSchema(t, root, args...)
		if err != nil {
			t.Fatalf("__schema %v: %v", args, err)
		}
		if bytes.Contains(out, []byte("SECRET-BODY")) || bytes.Contains(out, []byte(`"content"`)) {
			t.Errorf("__schema %v projected resource content: %s", args, out)
		}
	}

	direct, err := json.Marshal(Resource{URI: "app://x", Name: "x", Content: "SECRET-BODY"})
	if err != nil || bytes.Contains(direct, []byte("SECRET-BODY")) {
		t.Fatalf("marshaling Resource leaked Content (err %v): %s", err, direct)
	}
	if got := BuildSchema(root).Command.Resources[0].Content; got != "SECRET-BODY" {
		t.Errorf(
			"BuildSchema Content = %q, want the declared content retained in memory (only marshaling omits it)",
			got,
		)
	}
}

func TestDeclareRejectsOversizedContentAndTemplate(t *testing.T) {
	cases := []struct {
		name    string
		declare func() error
		field   string
		fixHas  string
	}{
		{
			name: "content over 1 MiB",
			declare: func() error {
				return DeclareResource(&cobra.Command{Use: "app"}, Resource{
					URI: "app://x", Name: "x", Content: strings.Repeat("a", 1<<20+1),
				})
			},
			field:  "content",
			fixHas: "1 MiB",
		},
		{
			name: "template over 64 KiB",
			declare: func() error {
				return DeclarePrompt(&cobra.Command{Use: "app"}, Prompt{
					Name: "p", Template: strings.Repeat("a", 64<<10+1),
				})
			},
			field:  "template",
			fixHas: "64 KiB",
		},
		{
			name: "uri over 2048 bytes",
			declare: func() error {
				return DeclareResource(&cobra.Command{Use: "app"}, Resource{
					URI: "app://" + strings.Repeat("a", 2048), Name: "x",
				})
			},
			field:  "uri",
			fixHas: "2048",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var envelope *contract.Error
			if err := tc.declare(); !errors.As(err, &envelope) {
				t.Fatalf("declare error = %v, want *contract.Error", err)
			}
			if envelope.ErrorCode != invalidDeclarationCode || envelope.Context["field"] != tc.field ||
				envelope.Context["reason"] != "too_long" {
				t.Fatalf("envelope = %+v, want %s too_long on %s", envelope, invalidDeclarationCode, tc.field)
			}
			if !strings.Contains(envelope.ActionableFix, tc.fixHas) {
				t.Fatalf("actionable_fix = %q, want it to name %q", envelope.ActionableFix, tc.fixHas)
			}
		})
	}
}
