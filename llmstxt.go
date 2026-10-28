package ax

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/internal/schema"
)

const (
	llmsSchemaCommand = "__schema"
	llmsMCPCommand    = "mcp-server"
	llmsCommandsHead  = "Commands"
	llmsGeneratedNote = "Generated from the live command tree."
)

// Link is one curated destination in a discovery document. Notes may be empty.
type Link struct {
	Title string
	URL   string
	Notes string
}

// LinkSection is an author-ordered group of links. Heading and Links are both
// required. The generator does not sort or rewrite the links.
type LinkSection struct {
	Heading string
	Links   []Link
}

// LLMsTxtOption configures GenerateLLMsTxt. Later options replace earlier ones
// for the same setting. The opening, whether text or file, is required.
type LLMsTxtOption func(*llmsConfig)

type llmsConfig struct {
	preamble string
	file     string
	useFile  bool
	links    []LinkSection
}

// WithPreamble sets the curated opening. It must begin with a title line and
// contain a later summary quote. A later WithPreamble or WithPreambleFile
// replaces this value.
func WithPreamble(text string) LLMsTxtOption {
	return func(cfg *llmsConfig) {
		cfg.preamble = text
		cfg.file = ""
		cfg.useFile = false
	}
}

// WithPreambleFile reads the curated opening from path. The read is capped at
// DefaultMaxConfigBytes (1 MiB). A larger file, a missing file, or a cancelled
// context fails with validation_error (exit 2) and writes nothing. A later
// opening option replaces this value.
func WithPreambleFile(path string) LLMsTxtOption {
	return func(cfg *llmsConfig) {
		cfg.file = path
		cfg.preamble = ""
		cfg.useFile = true
	}
}

// WithLinks sets the curated link sections, in author order. They render after
// the opening and before the generated command reference. Nil clears any
// earlier sections.
func WithLinks(sections []LinkSection) LLMsTxtOption {
	return func(cfg *llmsConfig) {
		cfg.links = sections
	}
}

// GenerateLLMsTxt writes a static llms.txt discovery document for root to w.
// The opening and any link sections come from opts. The command and flag
// reference is reflected from root with the same rules as __schema, excluding
// hidden commands and the reserved __schema and mcp-server commands.
//
// Identical inputs produce byte-identical output. The document contains no
// timestamp, trace identifier, or host data. Validation failures return
// validation_error (exit 2) and write nothing. A cancelled ctx returns
// ctx.Err() and writes nothing. This function does not change __schema or
// register a runtime command.
func GenerateLLMsTxt(ctx context.Context, w io.Writer, root *cobra.Command, opts ...LLMsTxtOption) error {
	if ctx == nil {
		return llmsValidation(context.Background(), "context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if w == nil {
		return llmsValidation(ctx, "discovery document destination is required")
	}
	if root == nil {
		return llmsValidation(ctx, "command tree is required")
	}

	cfg := llmsConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	preamble, err := opening(ctx, cfg)
	if err != nil {
		return err
	}
	if err = validLinks(ctx, cfg.links); err != nil {
		return err
	}

	doc := renderDocument(preamble, cfg.links, schema.BuildCommand(root))
	if err = ctx.Err(); err != nil {
		return err
	}
	_, err = io.WriteString(w, doc)
	return err
}

func opening(ctx context.Context, cfg llmsConfig) (string, error) {
	preamble := cfg.preamble
	if cfg.useFile {
		var err error
		preamble, err = readPreamble(ctx, cfg.file)
		if err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(preamble) == "" {
		return "", llmsValidation(ctx, "curated opening is required")
	}
	if !validOpening(preamble) {
		return "", llmsValidation(ctx, "curated opening must start with a title and contain a summary quote")
	}
	return preamble, nil
}

func renderDocument(preamble string, sections []LinkSection, root schema.Command) string {
	var doc strings.Builder
	doc.WriteString(strings.TrimRight(preamble, "\n"))
	doc.WriteString("\n")
	for _, section := range sections {
		writef(&doc, "\n## %s\n\n", oneLine(section.Heading))
		for _, link := range section.Links {
			writef(&doc, "- [%s](%s)", oneLine(link.Title), oneLine(link.URL))
			if notes := oneLine(link.Notes); notes != "" {
				writef(&doc, ": %s", notes)
			}
			doc.WriteString("\n")
		}
	}
	doc.WriteString("\n## ")
	doc.WriteString(llmsCommandsHead)
	doc.WriteString("\n\n")
	doc.WriteString(llmsGeneratedNote)
	doc.WriteString("\n")
	writeCommands(&doc, root, "")
	return doc.String()
}

func readPreamble(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if path == "" {
		return "", llmsValidation(ctx, "curated opening file is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", llmsValidation(ctx, "curated opening file is not readable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", llmsValidation(ctx, "curated opening file is not readable")
	}
	if info.Size() > DefaultMaxConfigBytes {
		return "", llmsValidation(ctx, "curated opening file exceeds 1 MiB")
	}
	buf := make([]byte, info.Size())
	if _, readErr := io.ReadFull(file, buf); readErr != nil {
		return "", llmsValidation(ctx, "curated opening file is not readable")
	}
	return string(buf), nil
}

func validOpening(text string) bool {
	sawTitle := false
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !sawTitle {
			if !strings.HasPrefix(line, "# ") {
				return false
			}
			sawTitle = true
			continue
		}
		if strings.HasPrefix(line, "> ") {
			return true
		}
	}
	return false
}

func validLinks(ctx context.Context, sections []LinkSection) error {
	for _, section := range sections {
		if strings.TrimSpace(section.Heading) == "" || len(section.Links) == 0 {
			return llmsValidation(ctx, "a link section needs a heading and at least one link")
		}
		for _, link := range section.Links {
			if strings.TrimSpace(link.Title) == "" || strings.TrimSpace(link.URL) == "" {
				return llmsValidation(ctx, "a link needs a title and a URL")
			}
		}
	}
	return nil
}

func writeCommands(doc *strings.Builder, cmd schema.Command, parent string) {
	name := commandToken(cmd.Use)
	if name == llmsSchemaCommand || name == llmsMCPCommand {
		return
	}
	path := name
	if parent != "" {
		path = parent + " " + name
	}
	writef(doc, "\n### %s\n", path)
	if short := oneLine(cmd.Short); short != "" {
		writef(doc, "\n%s\n", short)
	}
	if long := oneLine(cmd.Long); long != "" && long != oneLine(cmd.Short) {
		writef(doc, "\n%s\n", long)
	}
	if example := strings.TrimRight(cmd.Example, "\n"); example != "" {
		doc.WriteString("\nExamples:\n\n")
		for line := range strings.SplitSeq(example, "\n") {
			writef(doc, "    %s\n", strings.TrimRight(line, "\r"))
		}
	}
	flags := append([]schema.Flag(nil), cmd.Flags...)
	sort.SliceStable(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
	if len(flags) > 0 {
		doc.WriteString("\n")
	}
	for _, flag := range flags {
		writeFlag(doc, flag)
	}

	children := append([]schema.Command(nil), cmd.Commands...)
	sort.SliceStable(children, func(i, j int) bool {
		return commandToken(children[i].Use) < commandToken(children[j].Use)
	})
	for _, child := range children {
		writeCommands(doc, child, path)
	}
}

func writeFlag(doc *strings.Builder, flag schema.Flag) {
	writef(doc, "- `--%s`", flag.Name)
	if flag.Shorthand != "" {
		writef(doc, ", `-%s`", flag.Shorthand)
	}
	writef(doc, " (%s", flag.Type)
	if flag.Required {
		doc.WriteString(", required")
	}
	doc.WriteString(")")
	if usage := oneLine(flag.Usage); usage != "" {
		writef(doc, ": %s", usage)
	}
	if flag.Default != "" {
		writef(doc, ". Default: `%s`", oneLine(flag.Default))
	}
	if len(flag.Enum) > 0 {
		quoted := make([]string, len(flag.Enum))
		for i, value := range flag.Enum {
			quoted[i] = "`" + oneLine(value) + "`"
		}
		writef(doc, ". Values: %s", strings.Join(quoted, ", "))
	}
	if example := oneLine(flag.Example); example != "" {
		writef(doc, ". Example: `%s`", example)
	}
	doc.WriteString(".\n")
}

// writef appends formatted text. strings.Builder.Write never fails, so the
// error from fmt.Fprintf is discarded on purpose.
func writef(doc *strings.Builder, format string, args ...any) {
	_, _ = fmt.Fprintf(doc, format, args...)
}

func commandToken(use string) string {
	use = strings.TrimSpace(use)
	if i := strings.IndexAny(use, " \t"); i >= 0 {
		return use[:i]
	}
	return use
}

func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func llmsValidation(ctx context.Context, message string) error {
	return contract.NewError(ctx, contract.ErrorCodeValidationError, message,
		contract.WithErrorExitCode(contract.ExitValidation))
}
