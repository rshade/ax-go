package schema

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	internalschema "github.com/rshade/ax-go/internal/schema"
)

func TestMetadataNonDeterministicTagsMatchBuiltInLocators(t *testing.T) {
	metadataType := reflect.TypeFor[contract.Metadata]()
	got := make([]string, 0, metadataType.NumField())
	for fieldIndex := range metadataType.NumField() {
		field := metadataType.Field(fieldIndex)
		if field.Tag.Get("ax") != "nondeterministic" {
			continue
		}
		jsonName := field.Tag.Get("json")
		if comma := slices.Index([]byte(jsonName), ','); comma >= 0 {
			jsonName = jsonName[:comma]
		}
		got = append(got, "meta."+jsonName)
	}
	slices.Sort(got)

	want := []string{"meta.idempotency_key", "meta.span_id", "meta.trace_id"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("contract.Metadata non-deterministic locators = %v, want %v", got, want)
	}
}

func TestBuildSchemaReflectsCommandTree(t *testing.T) {
	root := newSchemaTestCommand()
	schema := BuildSchema(root, WithSchemaVersion("v0.1.0"))

	var stdout bytes.Buffer
	if err := contract.WriteJSON(&stdout, schema); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, filepath.Join("..", "testdata", "schema_ax.golden.json"), stdout.Bytes())

	if schema.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", schema.SchemaVersion, SchemaVersion)
	}
	if schema.Tool != "app" {
		t.Fatalf("Tool = %q, want app", schema.Tool)
	}
	if schema.Version != "v0.1.0" {
		t.Fatalf("Version = %q, want v0.1.0", schema.Version)
	}
	if len(schema.Command.Commands) != 1 {
		t.Fatalf("Commands length = %d, want 1", len(schema.Command.Commands))
	}
}

func TestBuildSchemaErrorEnvelopeNonDeterministicFields(t *testing.T) {
	got := BuildSchema(newSchemaTestCommand()).ErrorEnvelope.NonDeterministicFields
	want := []string{"trace_id"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ErrorEnvelope.NonDeterministicFields = %v, want %v", got, want)
	}
}

func TestBuildSchemaCommandNonDeterministicFields(t *testing.T) {
	type payload struct {
		GeneratedAt string `json:"generated_at" ax:"nondeterministic"`
	}

	root := &cobra.Command{Use: "app"}
	raw := &cobra.Command{Use: "raw"}
	root.AddCommand(raw)
	WithNonDeterministicFields[payload](root)

	built := BuildSchema(root)
	wantRegistered := []string{
		"data.generated_at",
		"meta.idempotency_key",
		"meta.span_id",
		"meta.trace_id",
	}
	if !reflect.DeepEqual(built.Command.NonDeterministicFields, wantRegistered) {
		t.Errorf(
			"registered NonDeterministicFields = %v, want %v",
			built.Command.NonDeterministicFields,
			wantRegistered,
		)
	}
	if built.Command.Commands[0].NonDeterministicFields == nil {
		t.Fatal("unregistered NonDeterministicFields is nil, want explicit empty slice")
	}
	if len(built.Command.Commands[0].NonDeterministicFields) != 0 {
		t.Errorf(
			"unregistered NonDeterministicFields = %v, want empty",
			built.Command.Commands[0].NonDeterministicFields,
		)
	}

	WithNonDeterministicFields[payload](nil)
}

func TestNonDeterministicFieldRenameUpdatesLocator(t *testing.T) {
	type beforeRename struct {
		Foo string `json:"foo" ax:"nondeterministic"`
	}
	type afterRename struct {
		Bar string `json:"bar" ax:"nondeterministic"`
	}

	before := internalschema.DataLocators(reflect.TypeFor[beforeRename]())
	after := internalschema.DataLocators(reflect.TypeFor[afterRename]())

	if !reflect.DeepEqual(before, []string{"data.foo"}) {
		t.Errorf("before rename locators = %v, want [data.foo]", before)
	}
	if !reflect.DeepEqual(after, []string{"data.bar"}) {
		t.Errorf("after rename locators = %v, want [data.bar]", after)
	}
}

func TestRemovingNonDeterministicTagDropsExactlyOneLocator(t *testing.T) {
	type fullyTagged struct {
		EntityID    string `json:"entity_id"    ax:"nondeterministic"`
		GeneratedAt string `json:"generated_at" ax:"nondeterministic"`
	}
	type tagRemoved struct {
		EntityID    string `json:"entity_id"    ax:"nondeterministic"`
		GeneratedAt string `json:"generated_at"`
	}

	fullCommand := &cobra.Command{Use: "full"}
	reducedCommand := &cobra.Command{Use: "reduced"}
	WithNonDeterministicFields[fullyTagged](fullCommand)
	WithNonDeterministicFields[tagRemoved](reducedCommand)

	full := BuildSchema(fullCommand).Command.NonDeterministicFields
	reduced := BuildSchema(reducedCommand).Command.NonDeterministicFields
	missing := make([]string, 0, 1)
	for _, locator := range full {
		if !slices.Contains(reduced, locator) {
			missing = append(missing, locator)
		}
	}

	wantMissing := []string{"data.generated_at"}
	if !reflect.DeepEqual(missing, wantMissing) {
		t.Errorf("locators removed with tag = %v, want %v; full=%v reduced=%v", missing, wantMissing, full, reduced)
	}
	if len(full)-len(reduced) != 1 {
		t.Errorf("locator count changed by %d, want 1; full=%v reduced=%v", len(full)-len(reduced), full, reduced)
	}
}

func TestBuildMCPSchemaGolden(t *testing.T) {
	root := newSchemaTestCommand()

	var stdout bytes.Buffer
	if err := contract.WriteJSON(&stdout, BuildMCPSchema(root)); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, filepath.Join("..", "testdata", "schema_mcp.golden.json"), stdout.Bytes())
}

func TestBuildMCPSchemaAdvertisesMultiValueFlagsAsArrays(t *testing.T) {
	root := &cobra.Command{Use: "app", Short: "test app"}
	root.Flags().StringSlice("tags", []string{"default"}, "tags to apply")

	built := BuildMCPSchema(root)
	props, ok := built.Tools[0].InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties is %T, want map[string]any", built.Tools[0].InputSchema["properties"])
	}
	tags, ok := props["tags"].(map[string]any)
	if !ok {
		t.Fatalf("tags schema is %T, want map[string]any", props["tags"])
	}
	if tags["type"] != "array" {
		t.Fatalf("tags.type = %q, want array", tags["type"])
	}
	items, ok := tags["items"].(map[string]any)
	if !ok {
		t.Fatalf("tags.items is %T, want map[string]any", tags["items"])
	}
	if items["type"] != "string" {
		t.Fatalf("tags.items.type = %q, want string", items["type"])
	}
	defaultValue, ok := tags["default"].([]any)
	if !ok {
		t.Fatalf("tags.default is %T, want []any", tags["default"])
	}
	if len(defaultValue) != 1 || defaultValue[0] != "default" {
		t.Fatalf("tags.default = %#v, want %#v", defaultValue, []any{"default"})
	}
}

func TestNewSchemaCommandRejectsUnknownFormat(t *testing.T) {
	root := newSchemaTestCommand()
	cmd := NewSchemaCommand(root)
	cmd.SetArgs([]string{"--as=xml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("NewSchemaCommand returned nil error for unknown format")
	}
	var contractErr *contract.Error
	if !errors.As(err, &contractErr) {
		t.Fatalf("error type = %T, want *contract.Error", err)
	}
	if contractErr.ErrorCode != "validation_error" {
		t.Fatalf("ErrorCode = %q, want validation_error", contractErr.ErrorCode)
	}
}

func newSchemaTestCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app run --name demo",
	}
	root.PersistentFlags().String("config", "", "config file")
	run := &cobra.Command{
		Use:     "run",
		Short:   "run something",
		Example: "app run --name demo",
	}
	run.Flags().String("name", "", "name to use")
	root.AddCommand(run)

	return root
}

func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\ngot:  %s\nwant: %s", path, got, want)
	}
}
