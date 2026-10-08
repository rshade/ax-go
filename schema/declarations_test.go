package schema

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/mcp"
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
		})
	}
}

func TestDeclareAcceptsValidInput(t *testing.T) {
	cmd := &cobra.Command{Use: "app"}
	if err := DeclarePrompt(cmd, Prompt{Name: "p", Template: "t"}); err != nil {
		t.Fatalf("DeclarePrompt = %v", err)
	}
	if err := DeclareResource(cmd, Resource{URI: "app://x", Name: "x"}); err != nil {
		t.Fatalf("DeclareResource = %v", err)
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
		golden  string
	}{
		{
			name: "prompt",
			declare: func(t *testing.T, first, second *cobra.Command) {
				mustDeclare(t, DeclarePrompt(first, Prompt{Name: "triage", Template: "first"}))
				mustDeclare(t, DeclarePrompt(second, Prompt{Name: "triage", Template: "second"}))
			},
			golden: "schema_duplicate_declaration.golden.json",
		},
		{
			name: "resource",
			declare: func(t *testing.T, first, second *cobra.Command) {
				mustDeclare(t, DeclareResource(first, Resource{URI: "app://docs", Name: "first"}))
				mustDeclare(t, DeclareResource(second, Resource{URI: "app://docs", Name: "second"}))
			},
			golden: "schema_duplicate_resource_declaration.golden.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newSchemaTestCommand()
			tc.declare(t, root, root.Commands()[0])

			for _, format := range []string{"--as=ax", "--as=mcp"} {
				stdout, err := runSchema(t, root, format)
				if len(stdout) != 0 {
					t.Fatalf("%s wrote stdout on a duplicate: %s", format, stdout)
				}
				if got := contract.ErrorExitCode(err); got != contract.ExitValidation {
					t.Fatalf("%s exit code = %d, want %d (err %v)", format, got, contract.ExitValidation, err)
				}
				var stderr bytes.Buffer
				if writeErr := contract.WriteError(&stderr, err); writeErr != nil {
					t.Fatalf("WriteError: %v", writeErr)
				}
				assertGolden(
					t,
					filepath.Join("..", "testdata", tc.golden),
					testutil.MaskNonDeterministic(stderr.Bytes()),
				)
			}

			first := func(schema MCPSchema) string {
				if len(schema.Prompts) > 0 {
					return schema.Prompts[0].Template
				}
				return schema.Resources[0].Name
			}(BuildMCPSchema(root))
			if first != "first" ||
				len(
					BuildSchema(root).Command.Commands[0].Prompts,
				)+len(
					BuildSchema(root).Command.Commands[0].Resources,
				) != 0 {
				t.Fatalf("builders did not keep the first declaration in walk order (first = %q)", first)
			}
		})
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

	for range 2 {
		got, err := runSchema(t, root, "--as=mcp")
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("projection changed after caller mutation (err %v)\ngot:  %s\nwant: %s", err, got, want)
		}
	}
}
