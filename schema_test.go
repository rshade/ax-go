package ax

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/schema"
)

func TestBuildSchemaReflectsCommandTree(t *testing.T) {
	root := newSchemaTestCommand()
	schema := BuildSchema(root, WithSchemaVersion("v0.1.0"))

	var stdout bytes.Buffer
	if err := WriteJSON(&stdout, schema); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_ax.golden.json", stdout.Bytes())

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
	if schema.Command.Commands[0].Use != "run" {
		t.Fatalf("child Use = %q, want run", schema.Command.Commands[0].Use)
	}
	if len(schema.Command.Flags) != 1 || schema.Command.Flags[0].Name != "config" {
		t.Fatalf("root flags = %#v, want config flag", schema.Command.Flags)
	}
}

func TestBuildMCPSchemaGolden(t *testing.T) {
	root := newSchemaTestCommand()

	var stdout bytes.Buffer
	if err := WriteJSON(&stdout, BuildMCPSchema(root)); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_mcp.golden.json", stdout.Bytes())
}

func TestBuildSchemaReflectsYesAsOrdinaryBooleanFlag(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	root.PersistentFlags().Bool("yes", false, "confirm a confirmation-gated operation")
	built := BuildSchema(root)
	if len(built.Command.Flags) != 1 || built.Command.Flags[0].Name != "yes" ||
		built.Command.Flags[0].Type != "bool" || built.Command.Flags[0].Default != "false" {
		t.Fatalf("yes schema flag = %#v, want ordinary false bool flag", built.Command.Flags)
	}
}

func TestRootSchemaOutputMatchesIsolatedPackage(t *testing.T) {
	root := newSchemaTestCommand()

	var rootOut bytes.Buffer
	if err := WriteJSON(&rootOut, BuildSchema(root, WithSchemaVersion("v0.1.0"))); err != nil {
		t.Fatalf("root WriteJSON returned error: %v", err)
	}
	var isolatedOut bytes.Buffer
	if err := WriteJSON(&isolatedOut, schema.BuildSchema(root, schema.WithSchemaVersion("v0.1.0"))); err != nil {
		t.Fatalf("isolated WriteJSON returned error: %v", err)
	}
	if !bytes.Equal(rootOut.Bytes(), isolatedOut.Bytes()) {
		t.Fatalf(
			"root schema diverged from isolated schema\nroot:     %s\nisolated: %s",
			rootOut.Bytes(),
			isolatedOut.Bytes(),
		)
	}

	rootOut.Reset()
	isolatedOut.Reset()
	if err := WriteJSON(&rootOut, BuildMCPSchema(root)); err != nil {
		t.Fatalf("root MCP WriteJSON returned error: %v", err)
	}
	if err := WriteJSON(&isolatedOut, schema.BuildMCPSchema(root)); err != nil {
		t.Fatalf("isolated MCP WriteJSON returned error: %v", err)
	}
	if !bytes.Equal(rootOut.Bytes(), isolatedOut.Bytes()) {
		t.Fatalf(
			"root MCP schema diverged from isolated schema\nroot:     %s\nisolated: %s",
			rootOut.Bytes(),
			isolatedOut.Bytes(),
		)
	}
}

func newSchemaTestCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app run --name demo",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	root.PersistentFlags().String("config", "", "config file")
	run := &cobra.Command{
		Use:     "run",
		Short:   "run something",
		Example: "app run --name demo",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	run.Flags().String("name", "", "name to use")
	root.AddCommand(run)

	return root
}

func TestDeclareFlagEnumAuthoringErrorContract(t *testing.T) {
	cmd := &cobra.Command{Use: "app"}
	cmd.Flags().Bool("force", false, "force")
	var axErr *Error
	if err := DeclareFlagEnum(cmd, "force", "true"); !errors.As(err, &axErr) {
		t.Fatalf("ax.DeclareFlagEnum error = %T %v, want *ax.Error", err, err)
	}
	if axErr.ErrorCode != "invalid_schema_declaration" || axErr.ExitCode() != ExitValidation {
		t.Fatalf("error = %q exit %d, want invalid_schema_declaration exit 2", axErr.ErrorCode, axErr.ExitCode())
	}
	want := map[string]any{"field": "flag", "reason": "unsupported_type"}
	if !reflect.DeepEqual(axErr.Context, want) {
		t.Fatalf("context = %#v, want %#v", axErr.Context, want)
	}
}

func TestCapabilityConstantsMatchSchema(t *testing.T) {
	pairs := []struct{ root, isolated schema.Capability }{
		{CapabilityReadOnly, schema.CapabilityReadOnly},
		{CapabilityCreate, schema.CapabilityCreate},
		{CapabilityMutate, schema.CapabilityMutate},
		{CapabilityDelete, schema.CapabilityDelete},
		{CapabilityExternalNetwork, schema.CapabilityExternalNetwork},
		{CapabilityAdmin, schema.CapabilityAdmin},
	}
	for _, pair := range pairs {
		if pair.root != pair.isolated {
			t.Errorf("ax constant %q != schema constant %q", pair.root, pair.isolated)
		}
	}
	cmd := &cobra.Command{Use: "app"}
	if err := DeclareCapability(cmd, CapabilityMutate, ""); err != nil {
		t.Fatalf("ax.DeclareCapability: %v", err)
	}
	if got := BuildSchema(cmd).Command.Capability; got == nil || got.Class != schema.CapabilityMutate {
		t.Fatalf("Capability = %+v, want mutate", got)
	}
}

// newEnrichedSchemaTestCommand mirrors the schema package's enriched fixture
// through the ax facade, so both packages pin the same golden bytes.
func newEnrichedSchemaTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	noop := func(*cobra.Command, []string) error { return nil }
	root := &cobra.Command{Use: "app", Short: "test app", RunE: noop}
	root.PersistentFlags().String("region", "us", "deployment region")
	root.Flags().String("config", "", "config file")

	deploy := &cobra.Command{Use: "deploy", Short: "deploy a release", RunE: noop}
	deploy.Flags().String("output", "json", "output format")
	deploy.Flags().Int("replicas", 1, "replica count")
	deploy.Flags().StringSlice("tags", nil, "release tags")
	deploy.Flags().Duration("timeout", 30*time.Second, "deadline")
	if err := deploy.MarkFlagRequired("output"); err != nil {
		t.Fatalf("MarkFlagRequired: %v", err)
	}
	status := &cobra.Command{Use: "status", Short: "show release status", RunE: noop}
	secret := &cobra.Command{Use: "secret", Short: "rotate secrets", Hidden: true, RunE: noop}
	root.AddCommand(deploy, status, secret)

	for _, declare := range []error{
		DeclareFlagEnum(root, "region", "us", "eu"),
		DeclareFlagExample(root, "region", "eu"),
		DeclareFlagEnum(deploy, "output", "json", "table", "yaml"),
		DeclareFlagEnum(deploy, "replicas", "1", "3", "5"),
		DeclareFlagExample(deploy, "tags", "a,b"),
		DeclareFlagExample(deploy, "timeout", "45s"),
		DeclareCapability(deploy, CapabilityMutate, "idempotent by release name"),
		DeclareCapability(status, CapabilityReadOnly, ""),
		DeclareCapability(secret, CapabilityAdmin, ""),
	} {
		if declare != nil {
			t.Fatalf("declaration failed: %v", declare)
		}
	}
	return root
}

func TestBuildSchemaEnrichedGolden(t *testing.T) {
	var stdout bytes.Buffer
	if err := WriteJSON(
		&stdout,
		BuildSchema(newEnrichedSchemaTestCommand(t), WithSchemaVersion("v0.1.0")),
	); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_ax_enriched.golden.json", stdout.Bytes())
}

func TestBuildMCPSchemaEnrichedGolden(t *testing.T) {
	var stdout bytes.Buffer
	if err := WriteJSON(&stdout, BuildMCPSchema(newEnrichedSchemaTestCommand(t))); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}
	assertGolden(t, "testdata/schema_mcp_enriched.golden.json", stdout.Bytes())
}
