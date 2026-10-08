package schema

import (
	"context"
	"errors"
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

// Violation reasons for flag and capability declarations, alongside the
// prompt/resource reasons; surfaced in the invalid_schema_declaration
// envelope's context.reason.
const (
	ReasonFlagNotFound    Reason = "flag_not_found"
	ReasonUnsupportedType Reason = "unsupported_type"
	ReasonInvalidValue    Reason = "invalid_value"
	ReasonNotInEnum       Reason = "not_in_enum"
	ReasonNotVocabulary   Reason = "not_in_vocabulary"
)

// Violation fields for flag and capability declarations.
const (
	fieldCmd     = "cmd"
	fieldFlag    = "flag"
	fieldValues  = "values"
	fieldDefault = "default"
	fieldExample = "example"
	fieldClass   = "class"
)

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

// lookupFlag finds name in cmd.Flags(), then cmd.PersistentFlags(), or
// reports why it cannot.
func lookupFlag(cmd *cobra.Command, name string) (*pflag.Flag, *Violation) {
	if cmd == nil {
		return nil, &Violation{Field: fieldCmd, Reason: ReasonNilCommand}
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag, nil
	}
	if flag := cmd.PersistentFlags().Lookup(name); flag != nil {
		return flag, nil
	}
	return nil, &Violation{Field: fieldFlag, Reason: ReasonFlagNotFound}
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

// AddFlagEnum restricts the named flag of cmd to values, enforced in the
// flag's Set at parse time. It validates everything before mutating anything,
// so a non-nil *Violation means cmd is unchanged. The checks, in order: cmd is
// non-nil (cmd/nil_command) and owns the flag (flag/flag_not_found); the flag
// type is string, an integer scalar, or a custom non-slice pflag.Value
// (flag/unsupported_type); values is non-empty (values/required); each value
// parses as the type (values/invalid_value); no two values are equal after
// canonicalisation (values/duplicate); a non-empty default is a member
// (default/not_in_enum); an example already declared on the flag is a member
// (example/not_in_enum). Re-declaring replaces the set on the existing wrapper
// instead of wrapping twice.
func AddFlagEnum(cmd *cobra.Command, name string, values []string) *Violation {
	flag, violation := lookupFlag(cmd, name)
	if violation != nil {
		return violation
	}

	inner := flag.Value
	if existing, ok := inner.(*enumValue); ok {
		inner = existing.inner
	}
	flagType := inner.Type()
	if !enumTypeSupported(inner) {
		return &Violation{Field: fieldFlag, Reason: ReasonUnsupportedType}
	}
	if len(values) == 0 {
		return &Violation{Field: fieldValues, Reason: ReasonRequired}
	}

	canonical := make([]string, 0, len(values))
	for _, value := range values {
		form, err := CanonicalEnumValue(flagType, value)
		if err != nil {
			return &Violation{Field: fieldValues, Reason: ReasonInvalidValue}
		}
		if slices.Contains(canonical, form) {
			return &Violation{Field: fieldValues, Reason: ReasonDuplicate}
		}
		canonical = append(canonical, form)
	}
	if !isMember(flagType, canonical, flag.DefValue, true) {
		return &Violation{Field: fieldDefault, Reason: ReasonNotInEnum}
	}
	if example, ok := declaredExample(flag); ok && !isMember(flagType, canonical, example, false) {
		return &Violation{Field: fieldExample, Reason: ReasonNotInEnum}
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

// AddFlagExample attaches one example value, in CLI form, to the named flag
// of cmd. It validates before mutating, so a non-nil *Violation leaves the
// previous example in place: cmd is nil (cmd/nil_command) or does not own the
// flag (flag/flag_not_found), the example is empty (example/required), it does
// not parse as the flag's known type (example/invalid_value; custom types are
// unchecked), or the flag has an enum and the example is not a member
// (example/not_in_enum). Re-declaring replaces the example.
func AddFlagExample(cmd *cobra.Command, name, example string) *Violation {
	flag, violation := lookupFlag(cmd, name)
	if violation != nil {
		return violation
	}
	if example == "" {
		return &Violation{Field: fieldExample, Reason: ReasonRequired}
	}
	if reason := exampleProblem(flag, example); reason != "" {
		return &Violation{Field: fieldExample, Reason: reason}
	}
	if flag.Annotations == nil {
		flag.Annotations = make(map[string][]string)
	}
	flag.Annotations[exampleAnnotationKey] = []string{example}
	return nil
}

// exampleProblem returns the Violation reason example fails on flag, or ""
// when it is acceptable: it must parse as the flag's type and, under an enum,
// be a member.
func exampleProblem(flag *pflag.Flag, example string) Reason {
	if err := ValidateValue(flag.Value.Type(), example); err != nil {
		return ReasonInvalidValue
	}
	if wrapper, ok := flag.Value.(*enumValue); ok && !isMember(wrapper.flagType, wrapper.canonical, example, false) {
		return ReasonNotInEnum
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

// AddCapability records class, and an optional free-form note, as cmd's
// side-effect classification. The note is trimmed, and an empty note removes
// any previous one. Re-declaring replaces both. It returns a *Violation,
// leaving the annotations unchanged, when cmd is nil (cmd/nil_command) or
// class is not exactly one of the six vocabulary strings
// (class/not_in_vocabulary).
func AddCapability(cmd *cobra.Command, class, note string) *Violation {
	if cmd == nil {
		return &Violation{Field: fieldCmd, Reason: ReasonNilCommand}
	}
	if !IsCapability(class) {
		return &Violation{Field: fieldClass, Reason: ReasonNotVocabulary}
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
// annotations, returning (class, note, ok). It fails closed: ok is false, and
// class and note are empty, when no class is stored or the stored class is not
// in the vocabulary.
func CommandCapability(annotations map[string]string) (string, string, bool) {
	class := annotations[capabilityAnnotationKey]
	if !IsCapability(class) {
		return "", "", false
	}
	return class, annotations[capabilityNoteAnnotationKey], true
}
