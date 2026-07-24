package ax

import (
	"os"
	"strings"
	"testing"
)

func TestDocumentationExplainsPublicImportChoices(t *testing.T) {
	readme := readTextFile(t, "README.md")
	for _, want := range []string{
		`import ax "github.com/rshade/ax-go"`,
		`"github.com/rshade/ax-go/config"`,
		`"github.com/rshade/ax-go/contract"`,
		`"github.com/rshade/ax-go/id"`,
		`"github.com/rshade/ax-go/schema"`,
		`"github.com/rshade/ax-go/logging"`,
		"Use the root package for full CLI runtime",
		"Use isolated contract packages for thin consumers",
		"Use the isolated logging package",
		// The choosing-a-surface table is the first thing a consumer needs and
		// the easiest thing to lose in a docs refactor, so its rows are asserted
		// rather than trusted. Loki staying root-only is the load-bearing row:
		// a reader who misses it will look for log shipping in the isolated
		// package and not find it.
		"### Choosing a surface",
		"| Logging only, smallest binary | `logging` |",
		"| Logging **plus** Loki direct push | root `ax` |",
	} {
		assertContains(t, readme, want)
	}
}

func TestIntegrationDocumentationNamesRootRuntimeImport(t *testing.T) {
	readme := readTextFile(t, "examples/integration/README.md")
	for _, want := range []string{
		`import ax "github.com/rshade/ax-go"`,
		"full CLI runtime",
		"isolated contract packages",
	} {
		assertContains(t, readme, want)
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected documentation to contain %q", needle)
	}
}
