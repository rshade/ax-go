package schema

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/rshade/ax-go/contract"
)

// Annotation keys under which declarations travel with the Cobra objects. The
// example is a flag annotation; the capability class and note are command
// annotations.
const (
	exampleAnnotationKey        = "github.com/rshade/ax-go/schema/example"
	capabilityAnnotationKey     = "github.com/rshade/ax-go/schema/capability"
	capabilityNoteAnnotationKey = "github.com/rshade/ax-go/schema/capability-note"
)

// ErrInvalidDeclaration is wrapped by every authoring error a declaration
// function returns. It is a programmer error, not bad agent input: if it reaches
// ax.Execute it maps to exit code 1.
var ErrInvalidDeclaration = errors.New("schema: invalid declaration")

// IsCapability reports whether class is one of the six vocabulary strings. The
// comparison is exact: case and surrounding whitespace must match.
func IsCapability(class string) bool {
	switch class {
	case "read-only", "create", "mutate", "delete", "external-network", "admin":
		return true
	default:
		return false
	}
}

// lookupFlag finds name in cmd.Flags(), then cmd.PersistentFlags(). The error
// wraps ErrInvalidDeclaration and names the command path.
func lookupFlag(cmd *cobra.Command, name string) (*pflag.Flag, error) {
	if cmd == nil {
		return nil, fmt.Errorf("%w: nil command", ErrInvalidDeclaration)
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag, nil
	}
	if flag := cmd.PersistentFlags().Lookup(name); flag != nil {
		return flag, nil
	}
	return nil, fmt.Errorf("%w: command %q has no flag --%s", ErrInvalidDeclaration, cmd.CommandPath(), name)
}

// enumValue wraps a flag's pflag.Value and owns its allowed-value set, so the
// set an agent sees in __schema and the set Set enforces are one fact. allowed
// keeps the author's CLI strings in declared order; canonical holds the same
// values in the form CanonicalEnumValue compares.
type enumValue struct {
	inner     pflag.Value
	flag      *pflag.Flag
	flagType  string
	allowed   []string
	canonical []string
}

// Set accepts the flag's live default unconditionally, so restoring the
// default (as the MCP dispatcher does between calls) can never fail. Any other
// value must canonicalise to a member; a non-member returns a validation_error
// *contract.Error (exit 2) without calling inner.Set, so the bound variable is
// untouched. The rejection never carries the raw input: stderr ships to log
// aggregation, and a value passed to the wrong flag may be a secret.
func (v *enumValue) Set(value string) error {
	if value == v.flag.DefValue {
		return v.inner.Set(value)
	}
	if canonical, err := CanonicalEnumValue(v.flagType, value); err == nil && slices.Contains(v.canonical, canonical) {
		return v.inner.Set(value)
	}
	return v.rejection()
}

func (v *enumValue) rejection() error {
	name := v.flag.Name
	suggestions := make([]string, 0, len(v.allowed))
	for _, allowed := range v.allowed {
		suggestions = append(suggestions, "--"+name+"="+allowed)
	}
	return contract.NewError(
		context.Background(),
		"validation_error",
		"flag --"+name+": value is not one of the allowed values",
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithErrorContext(map[string]any{"flag": name, "allowed": slices.Clone(v.allowed)}),
		contract.WithSuggestions(suggestions...),
	)
}

// String delegates to the wrapped value.
func (v *enumValue) String() string { return v.inner.String() }

// Type delegates to the wrapped value, so __schema keeps reporting the flag's
// real type.
func (v *enumValue) Type() string { return v.inner.Type() }

// DeclareFlagEnum restricts the named flag of cmd to values, enforced in the
// flag's Set at parse time. It validates everything before mutating anything,
// so a failed call leaves cmd unchanged. Every error wraps
// ErrInvalidDeclaration. The checks, in order: cmd is non-nil and owns the
// flag; the flag type is string, an integer scalar, or a custom non-slice
// pflag.Value; values is non-empty; each value parses as the type; no two
// values are equal after canonicalisation; a non-empty default is a member; an
// example already declared on the flag is a member. Re-declaring replaces the
// set on the existing wrapper instead of wrapping twice.
func DeclareFlagEnum(cmd *cobra.Command, name string, values []string) error {
	flag, err := lookupFlag(cmd, name)
	if err != nil {
		return err
	}
	fail := func(format string, args ...any) error {
		return fmt.Errorf("%w: command %q flag --%s: %s",
			ErrInvalidDeclaration, cmd.CommandPath(), name, fmt.Sprintf(format, args...))
	}

	inner := flag.Value
	if existing, ok := inner.(*enumValue); ok {
		inner = existing.inner
	}
	flagType := inner.Type()
	if !enumTypeSupported(inner) {
		return fail("type %s does not support an enum", flagType)
	}
	if len(values) == 0 {
		return fail("enum needs at least one value")
	}

	canonical := make([]string, 0, len(values))
	for _, value := range values {
		form, canonErr := CanonicalEnumValue(flagType, value)
		if canonErr != nil {
			return fail("enum value %q is not a valid %s: %v", value, flagType, canonErr)
		}
		if slices.Contains(canonical, form) {
			return fail("enum value %q duplicates another value", value)
		}
		canonical = append(canonical, form)
	}
	if !isMember(flagType, canonical, flag.DefValue, true) {
		return fail("default %q is not in the enum", flag.DefValue)
	}
	if example, ok := declaredExample(flag); ok && !isMember(flagType, canonical, example, false) {
		return fail("declared example %q is not in the enum", example)
	}

	allowed := slices.Clone(values)
	if existing, ok := flag.Value.(*enumValue); ok {
		existing.allowed, existing.canonical = allowed, canonical
		return nil
	}
	flag.Value = &enumValue{inner: inner, flag: flag, flagType: flagType, allowed: allowed, canonical: canonical}
	return nil
}

// enumTypeSupported reports whether an enum can be declared on a flag holding
// value: string, an integer scalar, or a custom type that is not a slice
// (wrapping a custom slice would hide its pflag.SliceValue behaviour).
func enumTypeSupported(value pflag.Value) bool {
	if _, isSlice := value.(pflag.SliceValue); isSlice {
		return false
	}
	_, err := CanonicalEnumValue(value.Type(), "0")
	return !errors.Is(err, errUnsupportedEnumType)
}

// isMember reports whether value canonicalises to one of canonical. When
// emptyIsMember is true, "" counts as a member: an empty default means "no
// default" and is exempt.
func isMember(flagType string, canonical []string, value string, emptyIsMember bool) bool {
	if value == "" && emptyIsMember {
		return true
	}
	form, err := CanonicalEnumValue(flagType, value)
	return err == nil && slices.Contains(canonical, form)
}

// declaredExample returns the example annotation when it is well formed:
// exactly one non-empty element.
func declaredExample(flag *pflag.Flag) (string, bool) {
	example := flag.Annotations[exampleAnnotationKey]
	if len(example) != 1 || example[0] == "" {
		return "", false
	}
	return example[0], true
}

// FlagEnum returns a copy of the allowed values declared on flag, in author
// order, or nil when none is declared. It fails closed: when flag.Value is not
// the enum wrapper (never declared, or re-wrapped by someone else) or a
// non-empty DefValue is no longer a member, it returns nil, so the schema never
// advertises a set that is not what Set enforces. A flag without an enum costs
// one type assertion and no allocation.
func FlagEnum(flag *pflag.Flag) []string {
	wrapper, ok := flag.Value.(*enumValue)
	if !ok {
		return nil
	}
	if !isMember(wrapper.flagType, wrapper.canonical, flag.DefValue, true) {
		return nil
	}
	return slices.Clone(wrapper.allowed)
}

// DeclareFlagExample attaches one example value, in CLI form, to the named
// flag of cmd. It validates before mutating, so a failed call leaves the
// previous example in place. Every error wraps ErrInvalidDeclaration: cmd is
// nil or does not own the flag, the example is empty, the example does not
// parse as the flag's known type (ValidateValue; custom types are unchecked),
// or the flag has an enum and the example is not a member. Re-declaring
// replaces the example.
func DeclareFlagExample(cmd *cobra.Command, name, example string) error {
	flag, err := lookupFlag(cmd, name)
	if err != nil {
		return err
	}
	if example == "" {
		return fmt.Errorf("%w: command %q flag --%s: example must not be empty",
			ErrInvalidDeclaration, cmd.CommandPath(), name)
	}
	if reason := exampleProblem(flag, example); reason != "" {
		return fmt.Errorf("%w: command %q flag --%s: example %q %s",
			ErrInvalidDeclaration, cmd.CommandPath(), name, example, reason)
	}
	if flag.Annotations == nil {
		flag.Annotations = make(map[string][]string)
	}
	flag.Annotations[exampleAnnotationKey] = []string{example}
	return nil
}

// exampleProblem returns why example is not acceptable on flag, or "" when it
// is: it must parse as the flag's type and, under an enum, be a member.
func exampleProblem(flag *pflag.Flag, example string) string {
	if err := ValidateValue(flag.Value.Type(), example); err != nil {
		return err.Error()
	}
	if wrapper, ok := flag.Value.(*enumValue); ok && !isMember(wrapper.flagType, wrapper.canonical, example, false) {
		return "is not in the enum"
	}
	return ""
}

// FlagExample returns the example declared on flag, or "" when none is. It
// fails closed: an annotation that is not exactly one non-empty element, or
// whose value no longer parses as the flag's type or is no longer an enum
// member (for example after a hand edit), yields "".
func FlagExample(flag *pflag.Flag) string {
	example, ok := declaredExample(flag)
	if !ok || exampleProblem(flag, example) != "" {
		return ""
	}
	return example
}

// DeclareCapability records class, and an optional free-form note, as cmd's
// side-effect classification. The note is trimmed, and an empty note removes
// any previous one. Re-declaring replaces both. It returns an error wrapping
// ErrInvalidDeclaration, leaving the annotations unchanged, when cmd is nil or
// class is not exactly one of the six vocabulary strings (IsCapability).
func DeclareCapability(cmd *cobra.Command, class, note string) error {
	if cmd == nil {
		return fmt.Errorf("%w: nil command", ErrInvalidDeclaration)
	}
	if !IsCapability(class) {
		return fmt.Errorf("%w: command %q: capability %q is not in the vocabulary",
			ErrInvalidDeclaration, cmd.CommandPath(), class)
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[capabilityAnnotationKey] = class
	if note = strings.TrimSpace(note); note != "" {
		cmd.Annotations[capabilityNoteAnnotationKey] = note
	} else {
		delete(cmd.Annotations, capabilityNoteAnnotationKey)
	}
	return nil
}

// CommandCapability reads the capability class and note stored in a command's
// annotations. It fails closed: ok is false, and class and note are empty,
// when no class is stored or the stored class is not in the vocabulary.
func CommandCapability(annotations map[string]string) (class, note string, ok bool) {
	class = annotations[capabilityAnnotationKey]
	if !IsCapability(class) {
		return "", "", false
	}
	return class, annotations[capabilityNoteAnnotationKey], true
}
