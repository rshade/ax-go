package schema

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

const (
	promptsAnnotationKey   = "github.com/rshade/ax-go/schema/prompts"
	resourcesAnnotationKey = "github.com/rshade/ax-go/schema/resources"

	// maxResourceURIBytes bounds adopter-supplied URIs so a declaration can
	// never inflate __schema without limit (Constitution IX).
	maxResourceURIBytes = 2048
)

// Violation reasons are stable machine values surfaced in the
// invalid_schema_declaration envelope's context.reason.
const (
	ReasonNilCommand            = "nil_command"
	ReasonRequired              = "required"
	ReasonInvalidCharset        = "invalid_charset"
	ReasonInvalidUTF8           = "invalid_utf8"
	ReasonDuplicate             = "duplicate"
	ReasonUndeclaredPlaceholder = "undeclared_placeholder"
	ReasonNotAbsolute           = "not_absolute"
	ReasonTooLong               = "too_long"
	ReasonInvalidCharacter      = "invalid_character"
)

// Field labels shared by several Violation sites.
const (
	fieldName = "name"
	fieldURI  = "uri"
)

// Conflict kinds name the namespace a duplicate key was found in.
const (
	KindPrompt   = "prompt"
	KindResource = "resource"
)

// Prompt is the internal prompt declaration and its annotation encoding.
type Prompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Template    string           `json:"template"`
}

// PromptArgument is one declared input of a Prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Resource is the internal resource declaration and its annotation encoding.
// It deliberately has no content field: Phase 1 resources are metadata only.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mime_type,omitempty"`
}

// Violation identifies the first rule a declaration breaks. Field uses the
// declaration's JSON field names, indexed for arguments ("arguments[1].name").
type Violation struct {
	Field  string
	Reason string
}

// Conflict reports a prompt name or resource URI declared more than once in
// the non-hidden tree, with the command paths of the first two occurrences.
type Conflict struct {
	Kind     string
	Key      string
	Commands []string
}

// ValidatePrompt returns the first rule prompt breaks, or nil when it is valid.
func ValidatePrompt(prompt Prompt) *Violation {
	if v := checkName(fieldName, prompt.Name); v != nil {
		return v
	}
	if v := checkUTF8("title", prompt.Title); v != nil {
		return v
	}
	if v := checkUTF8("description", prompt.Description); v != nil {
		return v
	}

	declared := make(map[string]struct{}, len(prompt.Arguments))
	for i, arg := range prompt.Arguments {
		prefix := "arguments[" + strconv.Itoa(i) + "]."
		if v := checkName(prefix+"name", arg.Name); v != nil {
			return v
		}
		if _, dup := declared[arg.Name]; dup {
			return &Violation{Field: prefix + "name", Reason: ReasonDuplicate}
		}
		declared[arg.Name] = struct{}{}
		if v := checkUTF8(prefix+"title", arg.Title); v != nil {
			return v
		}
		if v := checkUTF8(prefix+"description", arg.Description); v != nil {
			return v
		}
	}

	if prompt.Template == "" {
		return &Violation{Field: "template", Reason: ReasonRequired}
	}
	if v := checkUTF8("template", prompt.Template); v != nil {
		return v
	}
	for _, name := range templatePlaceholders(prompt.Template) {
		if _, ok := declared[name]; !ok {
			return &Violation{Field: "template", Reason: ReasonUndeclaredPlaceholder}
		}
	}
	return nil
}

// ValidateResource returns the first rule resource breaks, or nil when it is
// valid. A URI must be absolute (non-empty scheme), at most
// maxResourceURIBytes, and free of whitespace and control characters.
func ValidateResource(resource Resource) *Violation {
	if v := checkURI(resource.URI); v != nil {
		return v
	}
	if resource.Name == "" {
		return &Violation{Field: fieldName, Reason: ReasonRequired}
	}
	for _, field := range []struct{ name, value string }{
		{fieldName, resource.Name},
		{"title", resource.Title},
		{"description", resource.Description},
		{"mime_type", resource.MIMEType},
	} {
		if v := checkUTF8(field.name, field.value); v != nil {
			return v
		}
	}
	if hasControlOrSpace(resource.MIMEType) {
		return &Violation{Field: "mime_type", Reason: ReasonInvalidCharacter}
	}
	return nil
}

// AddPrompt validates prompt and appends it to cmd's prompt annotation,
// preserving every other annotation. It rejects a prompt whose name is already
// declared on cmd. On any violation cmd is left untouched.
func AddPrompt(cmd *cobra.Command, prompt Prompt) *Violation {
	if cmd == nil {
		return &Violation{Field: "cmd", Reason: ReasonNilCommand}
	}
	if v := ValidatePrompt(prompt); v != nil {
		return v
	}
	existing := Prompts(cmd.Annotations)
	for _, declared := range existing {
		if declared.Name == prompt.Name {
			return &Violation{Field: fieldName, Reason: ReasonDuplicate}
		}
	}
	setAnnotation(cmd, promptsAnnotationKey, append(existing, prompt))
	return nil
}

// AddResource validates resource and appends it to cmd's resource annotation,
// preserving every other annotation. It rejects a resource whose URI is
// already declared on cmd. On any violation cmd is left untouched.
func AddResource(cmd *cobra.Command, resource Resource) *Violation {
	if cmd == nil {
		return &Violation{Field: "cmd", Reason: ReasonNilCommand}
	}
	if v := ValidateResource(resource); v != nil {
		return v
	}
	existing := Resources(cmd.Annotations)
	for _, declared := range existing {
		if declared.URI == resource.URI {
			return &Violation{Field: fieldURI, Reason: ReasonDuplicate}
		}
	}
	setAnnotation(cmd, resourcesAnnotationKey, append(existing, resource))
	return nil
}

// Prompts returns the prompts stored in annotations in declaration order. It
// fails closed: an undecodable value yields nil, and an entry that fails
// ValidatePrompt (a hand-written annotation) is dropped.
func Prompts(annotations map[string]string) []Prompt {
	return decodeValid(annotations, promptsAnnotationKey, ValidatePrompt)
}

// Resources returns the resources stored in annotations in declaration order,
// failing closed exactly as Prompts does.
func Resources(annotations map[string]string) []Resource {
	return decodeValid(annotations, resourcesAnnotationKey, ValidateResource)
}

// WalkDeclarationCommands visits cmd and its descendants in pre-order, pruning
// hidden subtrees and nothing else. It is the same pruning rule BuildCommand
// applies, so the ax-native tree and the MCP aggregation see one set of
// declarations; reserved commands are deliberately not pruned.
func WalkDeclarationCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	if cmd.Hidden {
		return
	}
	visit(cmd)
	for _, child := range cmd.Commands() {
		WalkDeclarationCommands(child, visit)
	}
}

// FindDuplicate returns the first prompt name, then the first resource URI,
// declared more than once in the non-hidden tree, or nil. Every projected entry
// counts, so a hand-written same-command duplicate repeats that command's path.
func FindDuplicate(root *cobra.Command) *Conflict {
	promptPaths := map[string]string{}
	resourcePaths := map[string]string{}
	var promptConflict, resourceConflict *Conflict

	WalkDeclarationCommands(root, func(cmd *cobra.Command) {
		path := cmd.CommandPath()
		for _, prompt := range Prompts(cmd.Annotations) {
			promptConflict = firstConflict(promptConflict, promptPaths, KindPrompt, prompt.Name, path)
		}
		for _, resource := range Resources(cmd.Annotations) {
			resourceConflict = firstConflict(resourceConflict, resourcePaths, KindResource, resource.URI, path)
		}
	})

	if promptConflict != nil {
		return promptConflict
	}
	return resourceConflict
}

// CollectDeclarations aggregates every declaration in the non-hidden tree in
// walk order, then declaration order, keeping the first prompt per name and the
// first resource per URI. It is the non-erroring projection FindDuplicate guards.
func CollectDeclarations(root *cobra.Command) ([]Prompt, []Resource) {
	dedup := NewDeduper()
	var prompts []Prompt
	var resources []Resource
	WalkDeclarationCommands(root, func(cmd *cobra.Command) {
		prompts = append(prompts, dedup.Prompts(Prompts(cmd.Annotations))...)
		resources = append(resources, dedup.Resources(Resources(cmd.Annotations))...)
	})
	return prompts, resources
}

// Deduper keeps the first declaration of each prompt name and resource URI
// across successive calls, so per-command projections stay first-wins over a
// whole walk.
type Deduper struct {
	prompts   map[string]struct{}
	resources map[string]struct{}
}

// NewDeduper returns an empty Deduper.
func NewDeduper() *Deduper {
	return &Deduper{prompts: map[string]struct{}{}, resources: map[string]struct{}{}}
}

// Prompts returns the prompts whose names have not been seen, marking them
// seen. It returns nil rather than an empty slice so omitempty fields vanish.
func (d *Deduper) Prompts(prompts []Prompt) []Prompt {
	return keepFirst(d.prompts, prompts, func(p Prompt) string { return p.Name })
}

// Resources returns the resources whose URIs have not been seen, marking them
// seen.
func (d *Deduper) Resources(resources []Resource) []Resource {
	return keepFirst(d.resources, resources, func(r Resource) string { return r.URI })
}

// templatePlaceholders returns the argument names referenced by template, in
// order of appearance, including repeats. A placeholder is exactly "{{name}}"
// where name satisfies validName; any other brace text is literal. Each "{{"
// is paired with the next "}}" and accepted only when the whole span between
// them is a valid name, so "{{{x}}}" is literal (its span is "{x"). A rejected
// "{{" resumes the scan just after itself, so a literal never hides a later
// placeholder.
func templatePlaceholders(template string) []string {
	var names []string
	for rest := template; ; {
		open := strings.Index(rest, "{{")
		if open < 0 {
			return names
		}
		rest = rest[open+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return names
		}
		if name := rest[:end]; validName(name) {
			names = append(names, name)
			rest = rest[end+2:]
		}
	}
}

// validName reports whether name matches the MCP name rule ^[a-zA-Z0-9_.-]+$.
func validName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !isNameRune(r) {
			return false
		}
	}
	return true
}

func isNameRune(r rune) bool {
	return r < utf8.RuneSelf &&
		(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '_' || r == '.' || r == '-')
}

func checkName(field, name string) *Violation {
	if name == "" {
		return &Violation{Field: field, Reason: ReasonRequired}
	}
	if !validName(name) {
		return &Violation{Field: field, Reason: ReasonInvalidCharset}
	}
	return nil
}

func checkUTF8(field, value string) *Violation {
	if !utf8.ValidString(value) {
		return &Violation{Field: field, Reason: ReasonInvalidUTF8}
	}
	return nil
}

func checkURI(uri string) *Violation {
	switch {
	case uri == "":
		return &Violation{Field: fieldURI, Reason: ReasonRequired}
	case len(uri) > maxResourceURIBytes:
		return &Violation{Field: fieldURI, Reason: ReasonTooLong}
	case !utf8.ValidString(uri):
		return &Violation{Field: fieldURI, Reason: ReasonInvalidUTF8}
	case hasControlOrSpace(uri):
		return &Violation{Field: fieldURI, Reason: ReasonInvalidCharacter}
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme == "" {
		return &Violation{Field: fieldURI, Reason: ReasonNotAbsolute}
	}
	return nil
}

func hasControlOrSpace(value string) bool {
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func firstConflict(found *Conflict, seen map[string]string, kind, key, path string) *Conflict {
	if found != nil {
		return found
	}
	if first, dup := seen[key]; dup {
		return &Conflict{Kind: kind, Key: key, Commands: []string{first, path}}
	}
	seen[key] = path
	return nil
}

func setAnnotation[T any](cmd *cobra.Command, key string, declarations []T) {
	// Declarations hold only strings, bools, and slices of such structs, so
	// encoding cannot fail. Encoding also copies the caller's values, which is
	// what makes a declaration immune to later mutation (FR-008).
	encoded, _ := json.Marshal(declarations)
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[key] = string(encoded)
}

func decodeValid[T any](annotations map[string]string, key string, validate func(T) *Violation) []T {
	raw, ok := annotations[key]
	if !ok {
		return nil
	}
	var decoded []T
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil
	}
	var valid []T
	for _, declaration := range decoded {
		if validate(declaration) == nil {
			valid = append(valid, declaration)
		}
	}
	return valid
}

func keepFirst[T any](seen map[string]struct{}, declarations []T, key func(T) string) []T {
	var kept []T
	for _, declaration := range declarations {
		k := key(declaration)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		kept = append(kept, declaration)
	}
	return kept
}
