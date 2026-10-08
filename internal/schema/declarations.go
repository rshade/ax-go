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

	openDelim  = "{{"
	closeDelim = "}}"
)

// Reason is a stable machine value surfaced as context.reason in the
// invalid_schema_declaration and validation_error envelopes. Typing it keeps a
// misspelled new reason from compiling as a fresh public value.
type Reason string

// Violation and annotation-health reasons.
const (
	ReasonNilCommand            Reason = "nil_command"
	ReasonRequired              Reason = "required"
	ReasonInvalidCharset        Reason = "invalid_charset"
	ReasonInvalidUTF8           Reason = "invalid_utf8"
	ReasonDuplicate             Reason = "duplicate"
	ReasonUndeclaredPlaceholder Reason = "undeclared_placeholder"
	ReasonNotAbsolute           Reason = "not_absolute"
	ReasonMalformed             Reason = "malformed"
	ReasonTooLong               Reason = "too_long"
	ReasonInvalidCharacter      Reason = "invalid_character"
	ReasonCorruptAnnotation     Reason = "corrupt_annotation"
)

// Kind names the declaration namespace a problem was found in.
type Kind string

// Declaration kinds. Prompts are keyed by name, resources by URI.
const (
	KindPrompt   Kind = "prompt"
	KindResource Kind = "resource"
)

// Field labels shared by several Violation sites.
const (
	fieldName       = "name"
	fieldURI        = "uri"
	fieldAnnotation = "annotation"
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
// declaration's JSON field names, indexed for arguments ("arguments[1].name"),
// or "cmd" / "annotation" for problems with the target command.
type Violation struct {
	Field  string
	Reason Reason
}

// Conflict reports a declaration problem found by walking the tree: either a
// prompt name or resource URI declared twice (Reason duplicate, Commands holds
// the first two declaring paths), or an annotation that does not project
// cleanly (Reason corrupt_annotation, Commands holds the one offending path).
type Conflict struct {
	Kind     Kind
	Key      string
	Reason   Reason
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
		if v := checkName(prefix+fieldName, arg.Name); v != nil {
			return v
		}
		if _, dup := declared[arg.Name]; dup {
			return &Violation{Field: prefix + fieldName, Reason: ReasonDuplicate}
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
// valid. A URI must be at most maxResourceURIBytes, free of whitespace and
// control characters, parseable, and carry a non-empty scheme. A MIME type may
// contain spaces (as in "text/plain; charset=utf-8") but no control characters.
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
	if strings.ContainsFunc(resource.MIMEType, unicode.IsControl) {
		return &Violation{Field: "mime_type", Reason: ReasonInvalidCharacter}
	}
	return nil
}

// AddPrompt validates prompt and appends it to cmd's prompt annotation,
// preserving every other annotation. It rejects a prompt whose name is already
// declared on cmd, and refuses to rewrite an existing prompt annotation that
// does not decode or holds an invalid entry (corrupt_annotation), so a write
// never silently discards what was there. On any violation cmd is unchanged.
func AddPrompt(cmd *cobra.Command, prompt Prompt) *Violation {
	return addDeclaration(cmd, prompt, promptsAnnotationKey, fieldName, ValidatePrompt, promptName)
}

// AddResource validates resource and appends it to cmd's resource annotation,
// with the same preservation and corrupt-annotation rules as AddPrompt;
// resources are keyed by URI.
func AddResource(cmd *cobra.Command, resource Resource) *Violation {
	return addDeclaration(cmd, resource, resourcesAnnotationKey, fieldURI, ValidateResource, resourceURI)
}

// Prompts returns the prompts stored in annotations in declaration order. It
// fails closed: an undecodable value yields nil, and an entry that fails
// ValidatePrompt (a hand-written annotation) is dropped. FindCorrupt reports
// both cases so __schema can refuse to emit a silently reduced contract.
func Prompts(annotations map[string]string) []Prompt {
	valid, _ := decodeValid(annotations, promptsAnnotationKey, ValidatePrompt)
	return valid
}

// Resources returns the resources stored in annotations in declaration order,
// failing closed exactly as Prompts does.
func Resources(annotations map[string]string) []Resource {
	valid, _ := decodeValid(annotations, resourcesAnnotationKey, ValidateResource)
	return valid
}

// WalkDeclarationCommands visits cmd and then, in pre-order, every descendant
// not under a hidden child. cmd itself is always visited, even when hidden,
// mirroring BuildCommand exactly (it prunes hidden children, never the root),
// so the ax-native tree and the MCP aggregation see one set of declarations.
// Reserved commands are deliberately not pruned.
func WalkDeclarationCommands(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		WalkDeclarationCommands(child, visit)
	}
}

// FindDuplicate reports a prompt-name conflict in preference to a resource-URI
// conflict; within a kind, the first repeat in walk order wins. It returns nil
// when every key is unique. Every projected entry counts, so a hand-written
// same-command duplicate repeats that command's path.
func FindDuplicate(root *cobra.Command) *Conflict {
	if c := findDuplicate(root, KindPrompt, Prompts, promptName); c != nil {
		return c
	}
	return findDuplicate(root, KindResource, Resources, resourceURI)
}

// FindCorrupt returns the first command in walk order whose prompt (checked
// before resource) annotation is present but does not project cleanly — it
// fails to decode or holds an entry that fails validation — or nil.
func FindCorrupt(root *cobra.Command) *Conflict {
	var found *Conflict
	WalkDeclarationCommands(root, func(cmd *cobra.Command) {
		if found != nil {
			return
		}
		_, promptsClean := decodeValid(cmd.Annotations, promptsAnnotationKey, ValidatePrompt)
		_, resourcesClean := decodeValid(cmd.Annotations, resourcesAnnotationKey, ValidateResource)
		switch {
		case !promptsClean:
			found = corruptConflict(KindPrompt, promptsAnnotationKey, cmd)
		case !resourcesClean:
			found = corruptConflict(KindResource, resourcesAnnotationKey, cmd)
		}
	})
	return found
}

// CollectDeclarations aggregates every declaration WalkDeclarationCommands
// reaches, in walk order then declaration order, keeping the first prompt per
// name and the first resource per URI. Both results are nil when nothing is
// declared. It is the non-erroring projection FindDuplicate guards.
func CollectDeclarations(root *cobra.Command) ([]Prompt, []Resource) {
	dedup := NewDeduper()
	var prompts []Prompt
	var resources []Resource
	WalkDeclarationCommands(root, func(cmd *cobra.Command) {
		prompts = append(prompts, dedup.FirstPrompts(Prompts(cmd.Annotations))...)
		resources = append(resources, dedup.FirstResources(Resources(cmd.Annotations))...)
	})
	return prompts, resources
}

// Deduper keeps the first declaration of each prompt name and resource URI
// across successive calls, so per-command projections stay first-wins over a
// whole walk. It must be created with NewDeduper; the zero value panics.
type Deduper struct {
	prompts   map[string]struct{}
	resources map[string]struct{}
}

// NewDeduper returns an empty Deduper.
func NewDeduper() *Deduper {
	return &Deduper{prompts: map[string]struct{}{}, resources: map[string]struct{}{}}
}

// FirstPrompts returns the prompts whose names have not been seen, marking them
// seen, or nil when none are new.
func (d *Deduper) FirstPrompts(prompts []Prompt) []Prompt {
	return keepFirst(d.prompts, prompts, promptName)
}

// FirstResources returns the resources whose URIs have not been seen, marking
// them seen, or nil when none are new.
func (d *Deduper) FirstResources(resources []Resource) []Resource {
	return keepFirst(d.resources, resources, resourceURI)
}

// templatePlaceholders returns the argument names referenced by template, in
// order of appearance, including repeats. A placeholder is exactly "{{name}}"
// where name satisfies validName; any other brace text is literal. Each "{{"
// is paired with the next "}}" and accepted only when the whole span between
// them is a valid name, so "{{{x}}}" is literal (its span is "{x"). A rejected
// "{{" resumes the scan just after itself, so a literal never hides a later
// placeholder.
//
// Every "{{" the scan reaches inside a rejected span pairs with that span's
// "}}", and each but the last encloses another "{{", so only the last can be
// valid. The scan jumps straight to that last reachable "{{" instead of
// re-searching for "}}" from each one, which keeps it linear on brace runs.
func templatePlaceholders(template string) []string {
	var names []string
	for rest := template; ; {
		open := strings.Index(rest, openDelim)
		if open < 0 {
			return names
		}
		rest = rest[open+len(openDelim):]
		end := strings.Index(rest, closeDelim)
		if end < 0 {
			return names
		}
		span := rest[:end]
		if validName(span) {
			names = append(names, span)
			rest = rest[end+len(closeDelim):]
			continue
		}
		if last := lastReachableOpen(span); last >= 0 {
			rest = rest[last:]
		}
	}
}

// lastReachableOpen returns the offset of the last "{{" in span that a
// left-to-right scan resuming two bytes after each "{{" reaches, or -1. In a
// run of braces this steps by two, so it differs from strings.LastIndex.
func lastReachableOpen(span string) int {
	last := -1
	for i := 0; ; {
		j := strings.Index(span[i:], openDelim)
		if j < 0 {
			return last
		}
		last = i + j
		i = last + len(openDelim)
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
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
		r == '_' || r == '.' || r == '-'
}

func promptName(prompt Prompt) string { return prompt.Name }

func resourceURI(resource Resource) string { return resource.URI }

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
	}
	if v := checkUTF8(fieldURI, uri); v != nil {
		return v
	}
	if strings.ContainsFunc(uri, isControlOrSpace) {
		return &Violation{Field: fieldURI, Reason: ReasonInvalidCharacter}
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return &Violation{Field: fieldURI, Reason: ReasonMalformed}
	}
	if parsed.Scheme == "" {
		return &Violation{Field: fieldURI, Reason: ReasonNotAbsolute}
	}
	return nil
}

func isControlOrSpace(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}

func addDeclaration[T any](
	cmd *cobra.Command,
	declaration T,
	annotationKey, keyField string,
	validate func(T) *Violation,
	key func(T) string,
) *Violation {
	if cmd == nil {
		return &Violation{Field: "cmd", Reason: ReasonNilCommand}
	}
	if v := validate(declaration); v != nil {
		return v
	}
	existing, clean := decodeValid(cmd.Annotations, annotationKey, validate)
	if !clean {
		return &Violation{Field: fieldAnnotation, Reason: ReasonCorruptAnnotation}
	}
	for _, declared := range existing {
		if key(declared) == key(declaration) {
			return &Violation{Field: keyField, Reason: ReasonDuplicate}
		}
	}

	// Declarations hold only strings, bools, and slices of such structs, so
	// encoding cannot fail. Encoding also copies the caller's values, which is
	// what makes a declaration immune to later mutation (FR-008).
	encoded, _ := json.Marshal(append(existing, declaration))
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[annotationKey] = string(encoded)
	return nil
}

// decodeValid decodes the declarations stored under key, dropping entries that
// fail validate. clean is false when the key is present but undecodable or any
// entry was dropped; an absent key is clean.
func decodeValid[T any](annotations map[string]string, key string, validate func(T) *Violation) ([]T, bool) {
	raw, ok := annotations[key]
	if !ok {
		return nil, true
	}
	var decoded []T
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, false
	}
	var valid []T
	for _, declaration := range decoded {
		if validate(declaration) == nil {
			valid = append(valid, declaration)
		}
	}
	return valid, len(valid) == len(decoded)
}

func findDuplicate[T any](
	root *cobra.Command,
	kind Kind,
	decode func(map[string]string) []T,
	key func(T) string,
) *Conflict {
	firstPath := map[string]string{}
	var found *Conflict
	WalkDeclarationCommands(root, func(cmd *cobra.Command) {
		if found != nil {
			return
		}
		path := cmd.CommandPath()
		for _, declaration := range decode(cmd.Annotations) {
			k := key(declaration)
			if first, dup := firstPath[k]; dup {
				found = &Conflict{Kind: kind, Key: k, Reason: ReasonDuplicate, Commands: []string{first, path}}
				return
			}
			firstPath[k] = path
		}
	})
	return found
}

func corruptConflict(kind Kind, annotationKey string, cmd *cobra.Command) *Conflict {
	return &Conflict{
		Kind:     kind,
		Key:      annotationKey,
		Reason:   ReasonCorruptAnnotation,
		Commands: []string{cmd.CommandPath()},
	}
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
