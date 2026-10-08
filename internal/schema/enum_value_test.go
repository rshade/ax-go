package schema

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rshade/ax-go/contract"
)

func TestEnumValueSet(t *testing.T) {
	t.Run("member updates bound variable", func(t *testing.T) {
		var output string
		cmd := &cobra.Command{Use: "app"}
		cmd.Flags().StringVar(&output, "output", "json", "output")
		mustDeclareEnum(t, cmd, "output", "json", "table")

		if err := cmd.Flags().Lookup("output").Value.Set("table"); err != nil {
			t.Fatalf("Set(table) error = %v", err)
		}
		if output != "table" {
			t.Fatalf("bound variable = %q, want table", output)
		}
	})

	t.Run("non-canonical int member accepted", func(t *testing.T) {
		var n int
		cmd := &cobra.Command{Use: "app"}
		cmd.Flags().IntVar(&n, "n", 1, "n")
		mustDeclareEnum(t, cmd, "n", "1", "3")

		if err := cmd.Flags().Lookup("n").Value.Set("03"); err != nil {
			t.Fatalf("Set(03) error = %v", err)
		}
		if n != 3 {
			t.Fatalf("bound variable = %d, want 3", n)
		}
	})

	t.Run("non-member rejected without echoing input", func(t *testing.T) {
		var output string
		cmd := &cobra.Command{Use: "app"}
		cmd.Flags().StringVar(&output, "output", "json", "output")
		mustDeclareEnum(t, cmd, "output", "json", "table", "yaml")

		const secret = "s3cr3t-token"
		err := cmd.Flags().Lookup("output").Value.Set(secret)
		var contractErr *contract.Error
		if !errors.As(err, &contractErr) {
			t.Fatalf("Set error = %T %v, want *contract.Error", err, err)
		}
		if contractErr.ErrorCode != "validation_error" || contractErr.ExitCode() != contract.ExitValidation {
			t.Fatalf("error = %q exit %d, want validation_error exit 2", contractErr.ErrorCode, contractErr.ExitCode())
		}
		wantContext := map[string]any{"flag": "output", "allowed": []string{"json", "table", "yaml"}}
		if !reflect.DeepEqual(contractErr.Context, wantContext) {
			t.Fatalf("Context = %#v, want %#v", contractErr.Context, wantContext)
		}
		wantSuggestions := []string{"--output=json", "--output=table", "--output=yaml"}
		if !reflect.DeepEqual(contractErr.Suggestions, wantSuggestions) {
			t.Fatalf("Suggestions = %v, want %v", contractErr.Suggestions, wantSuggestions)
		}
		if strings.Contains(contractErr.Message, secret) {
			t.Fatalf("Message %q echoes the raw input", contractErr.Message)
		}
		if output != "json" {
			t.Fatalf("bound variable = %q, want unchanged json", output)
		}
	})

	t.Run("String and Type delegate", func(t *testing.T) {
		cmd := &cobra.Command{Use: "app"}
		cmd.Flags().Int("n", 3, "n")
		mustDeclareEnum(t, cmd, "n", "1", "3")
		value := cmd.Flags().Lookup("n").Value
		if value.String() != "3" || value.Type() != "int" {
			t.Fatalf("String/Type = %q/%q, want 3/int", value.String(), value.Type())
		}
	})

	t.Run("live default always accepted", func(t *testing.T) {
		var output string
		cmd := &cobra.Command{Use: "app"}
		cmd.Flags().StringVar(&output, "output", "", "output")
		mustDeclareEnum(t, cmd, "output", "json", "table")
		value := cmd.Flags().Lookup("output").Value

		if err := value.Set("table"); err != nil {
			t.Fatalf("Set(table) error = %v", err)
		}
		if err := value.Set(""); err != nil {
			t.Fatalf("Set(default) error = %v", err)
		}
		if output != "" {
			t.Fatalf("bound variable = %q, want reset to empty default", output)
		}
	})

	t.Run("parse error unwraps to contract error", func(t *testing.T) {
		flags := pflag.NewFlagSet("app", pflag.ContinueOnError)
		flags.SetOutput(io.Discard)
		flags.String("output", "json", "output")
		cmd := &cobra.Command{Use: "app"}
		cmd.Flags().AddFlagSet(flags)
		mustDeclareEnum(t, cmd, "output", "json", "table")

		err := flags.Parse([]string{"--output=xml"})
		var invalid *pflag.InvalidValueError
		if !errors.As(err, &invalid) {
			t.Fatalf("Parse error = %T %v, want *pflag.InvalidValueError", err, err)
		}
		var contractErr *contract.Error
		if !errors.As(err, &contractErr) {
			t.Fatalf("Parse error does not unwrap to *contract.Error: %v", err)
		}
	})
}

func mustDeclareEnum(t *testing.T, cmd *cobra.Command, flag string, values ...string) {
	t.Helper()
	if err := DeclareFlagEnum(cmd, flag, values); err != nil {
		t.Fatalf("DeclareFlagEnum(%s): %v", flag, err)
	}
}
