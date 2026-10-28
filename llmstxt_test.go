package ax

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
)

func TestGenerateLLMsTxt(t *testing.T) {
	root := discoveryTree(t)
	opts := []LLMsTxtOption{
		WithPreamble("# Zeta\n\n> A sorted command reference.\n\nExtra prose stays.\n"),
		WithLinks([]LinkSection{
			{
				Heading: "Zed",
				Links: []Link{
					{Title: "Later", URL: "https://example.test/later", Notes: "second"},
					{Title: "Earlier", URL: "https://example.test/earlier"},
				},
			},
			{
				Heading: "Alpha",
				Links:   []Link{{Title: "Home", URL: "https://example.test", Notes: "first\nsection"}},
			},
		}),
	}

	var first, second bytes.Buffer
	if err := GenerateLLMsTxt(context.Background(), &first, root, opts...); err != nil {
		t.Fatalf("GenerateLLMsTxt: %v", err)
	}
	if err := GenerateLLMsTxt(context.Background(), &second, discoveryTree(t), opts...); err != nil {
		t.Fatalf("second GenerateLLMsTxt: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("two generations of the same tree differ")
	}

	want, err := os.ReadFile("testdata/llms.golden.txt")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(first.Bytes(), want) {
		t.Fatalf("document mismatch\n got:\n%s\nwant:\n%s", first.Bytes(), want)
	}
	for _, absent := range []string{"secret", "__schema", "mcp-server", "buried", "llms-full"} {
		if strings.Contains(first.String(), absent) {
			t.Errorf("document contains %q", absent)
		}
	}
}

func TestGenerateLLMsTxtValidation(t *testing.T) {
	t.Parallel()

	huge := bytes.Repeat([]byte("x"), int(DefaultMaxConfigBytes)+1)
	exact := make([]byte, DefaultMaxConfigBytes)
	copy(exact, []byte("# Tool\n\n> Summary.\n"))

	tests := []struct {
		name    string
		build   func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption)
		wantLit string
		wantIs  error
		empty   bool
	}{
		{
			name:    "missing opening",
			wantLit: "validation_error",
			empty:   true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), &bytes.Buffer{}, discoveryTree(t), nil
			},
		},
		{
			name:    "title-less opening",
			wantLit: "validation_error",
			empty:   true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), &bytes.Buffer{}, discoveryTree(t),
					[]LLMsTxtOption{WithPreamble("not a title\n\n> quote\n")}
			},
		},
		{
			name:    "opening without summary",
			wantLit: "validation_error",
			empty:   true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), &bytes.Buffer{}, discoveryTree(t),
					[]LLMsTxtOption{WithPreamble("# Title\n\nprose only\n")}
			},
		},
		{
			name:    "missing file",
			wantLit: "validation_error",
			empty:   true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), &bytes.Buffer{}, discoveryTree(t),
					[]LLMsTxtOption{WithPreambleFile(t.TempDir() + "/missing.md")}
			},
		},
		{
			name: "file of exactly 1 MiB",
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				path := t.TempDir() + "/opening.md"
				if err := os.WriteFile(path, exact, 0o600); err != nil {
					t.Fatalf("write preamble: %v", err)
				}
				return context.Background(), &bytes.Buffer{}, discoveryTree(t),
					[]LLMsTxtOption{WithPreambleFile(path)}
			},
		},
		{
			name:    "file of 1 MiB plus one byte",
			wantLit: "validation_error",
			empty:   true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				path := t.TempDir() + "/opening.md"
				if err := os.WriteFile(path, huge, 0o600); err != nil {
					t.Fatalf("write preamble: %v", err)
				}
				return context.Background(), &bytes.Buffer{}, discoveryTree(t),
					[]LLMsTxtOption{WithPreambleFile(path)}
			},
		},
		{
			name:    "nil destination",
			wantLit: "validation_error",
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), nil, discoveryTree(t),
					[]LLMsTxtOption{WithPreamble("# Title\n\n> Summary.\n")}
			},
		},
		{
			name:    "nil command",
			wantLit: "validation_error",
			empty:   true,
			build: func(*testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				return context.Background(), &bytes.Buffer{}, nil,
					[]LLMsTxtOption{WithPreamble("# Title\n\n> Summary.\n")}
			},
		},
		{
			name:   "cancelled context",
			wantIs: context.Canceled,
			empty:  true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, &bytes.Buffer{}, discoveryTree(t),
					[]LLMsTxtOption{WithPreamble("# Title\n\n> Summary.\n")}
			},
		},
		{
			name:    "empty link section",
			wantLit: "validation_error",
			empty:   true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), &bytes.Buffer{}, discoveryTree(t), []LLMsTxtOption{
					WithPreamble("# Title\n\n> Summary.\n"),
					WithLinks([]LinkSection{{Heading: "Docs"}}),
				}
			},
		},
		{
			name:    "link missing title",
			wantLit: "validation_error",
			empty:   true,
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), &bytes.Buffer{}, discoveryTree(t), []LLMsTxtOption{
					WithPreamble("# Title\n\n> Summary.\n"),
					WithLinks([]LinkSection{{Heading: "Docs", Links: []Link{{URL: "https://example.test"}}}}),
				}
			},
		},
		{
			name: "later opening replaces a missing file",
			build: func(t *testing.T) (context.Context, io.Writer, *cobra.Command, []LLMsTxtOption) {
				t.Helper()
				return context.Background(), &bytes.Buffer{}, discoveryTree(t), []LLMsTxtOption{
					WithPreambleFile(t.TempDir() + "/missing.md"),
					WithPreamble("# Title\n\n> Summary.\n"),
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, dest, root, opts := tc.build(t)
			err := GenerateLLMsTxt(ctx, dest, root, opts...)
			if buf, ok := dest.(*bytes.Buffer); ok {
				if tc.empty && buf.Len() != 0 {
					t.Fatalf("wrote %d bytes on failure", buf.Len())
				}
				if !tc.empty && buf.Len() == 0 {
					t.Fatal("success wrote nothing")
				}
			}
			if tc.wantIs != nil {
				if !errors.Is(err, tc.wantIs) {
					t.Fatalf("error = %v, want %v", err, tc.wantIs)
				}
				return
			}
			if tc.wantLit == "" {
				if err != nil {
					t.Fatalf("GenerateLLMsTxt: %v", err)
				}
				return
			}
			var got *contract.Error
			if !errors.As(err, &got) {
				t.Fatalf("error = %T %v, want *contract.Error", err, err)
			}
			if got.ErrorCode != tc.wantLit {
				t.Fatalf("error_code = %q, want %q", got.ErrorCode, tc.wantLit)
			}
			if got.ExitCode() != 2 {
				t.Fatalf("exit = %d, want 2", got.ExitCode())
			}
		})
	}
}

func TestGenerateLLMsTxtDoesNotTouchSchema(t *testing.T) {
	root := discoveryTree(t)
	before := schemaBytes(t, root)
	var doc bytes.Buffer
	if err := GenerateLLMsTxt(context.Background(), &doc, root,
		WithPreamble("# Title\n\n> Summary.\n"),
	); err != nil {
		t.Fatalf("GenerateLLMsTxt: %v", err)
	}
	after := schemaBytes(t, root)
	if !bytes.Equal(before, after) {
		t.Fatalf("schema changed\nbefore: %s\nafter: %s", before, after)
	}
	if hasCommand(root, "__llms") {
		t.Fatal("generator registered __llms")
	}
	if strings.Contains(doc.String(), "llms-full") {
		t.Fatal("document is an llms-full body")
	}
	if !strings.Contains(doc.String(), "## Commands\n") {
		t.Fatal("document missing the generated command reference")
	}
}

func discoveryTree(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{
		Use:     "zeta-cli",
		Short:   "root short\nwith a break",
		Long:    "root long stays distinct",
		Example: "zeta-cli alpha\nzeta-cli zeta",
	}
	root.PersistentFlags().StringP("region", "r", "us", "region\nto use")
	root.Flags().String("config", "", "config file")
	if err := root.MarkFlagRequired("config"); err != nil {
		t.Fatalf("MarkFlagRequired: %v", err)
	}
	if err := DeclareFlagExample(root, "config", "app.hujson"); err != nil {
		t.Fatalf("DeclareFlagExample: %v", err)
	}

	alpha := &cobra.Command{Use: "alpha", Short: "first child", Long: "first child"}
	alpha.Flags().String("name", "ada", "who\nto greet")
	if err := DeclareFlagEnum(alpha, "name", "ada", "grace"); err != nil {
		t.Fatalf("DeclareFlagEnum: %v", err)
	}
	zeta := &cobra.Command{Use: "zeta [name]", Short: "later child"}
	hidden := &cobra.Command{Use: "secret", Hidden: true, Short: "hidden command"}
	hidden.AddCommand(&cobra.Command{Use: "nested", Short: "also hidden"})
	schemaCmd := &cobra.Command{Use: "__schema", Short: "schema"}
	mcpCmd := &cobra.Command{Use: "mcp-server", Short: "mcp"}
	mcpCmd.AddCommand(&cobra.Command{Use: "buried", Short: "must not appear"})

	root.AddCommand(zeta, hidden, schemaCmd, mcpCmd, alpha)
	return root
}

func schemaBytes(t *testing.T, root *cobra.Command) []byte {
	t.Helper()
	encoded, err := json.Marshal(BuildSchema(root))
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	return encoded
}

func hasCommand(root *cobra.Command, name string) bool {
	var found bool
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Name() == name {
			found = true
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
	return found
}
