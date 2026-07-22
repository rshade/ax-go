package schema

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

const (
	envelopeAnnotationKey = "github.com/rshade/ax-go/schema/envelope"
	locatorsAnnotationKey = "github.com/rshade/ax-go/schema/non-deterministic-fields"
	maxLocatorDepth       = 64
)

// DataLocators returns sorted, unique data.* locators for exported fields
// marked ax:"nondeterministic". It follows JSON field names through nested
// structs and container element types, bounds recursion, and returns a non-nil
// empty slice for unsupported types.
//
// It does not resolve encoding/json's field-shadowing precedence: if an
// embedded struct's tagged field shares a JSON name with a shallower field
// (which wins at marshal time and makes the embedded field unreachable in
// real output), this still reports a locator for the shadowed field. Avoid
// reusing a JSON field name across embedding depths in a type passed to
// WithNonDeterministicFields.
func DataLocators(t reflect.Type) []string {
	locators := make(map[string]struct{})
	walkDataLocators(t, []string{"data"}, 0, make(map[reflect.Type]bool), locators)
	return sortedKeys(locators)
}

// RegisterEnvelope marks cmd as emitting the standard success envelope and
// stores its precomputed data.* locators. A nil command is ignored; repeated
// calls overwrite the prior locator registration.
func RegisterEnvelope(cmd *cobra.Command, dataLocators []string) {
	if cmd == nil {
		return
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}

	// Encoding a []string has no unsupported values and cannot fail.
	encoded, _ := json.Marshal(sortedUnique(dataLocators))
	cmd.Annotations[envelopeAnnotationKey] = "true"
	cmd.Annotations[locatorsAnnotationKey] = string(encoded)
}

// NonDeterministicFields returns the sorted, unique non-deterministic locator
// contract stored in annotations. Unregistered or malformed annotations fail
// closed to a non-nil empty slice or the registered envelope's built-in fields.
func NonDeterministicFields(annotations map[string]string) []string {
	if annotations[envelopeAnnotationKey] != "true" {
		return []string{}
	}

	locators := []string{
		"meta.idempotency_key",
		"meta.span_id",
		"meta.trace_id",
	}
	var dataLocators []string
	if err := json.Unmarshal([]byte(annotations[locatorsAnnotationKey]), &dataLocators); err == nil {
		for _, locator := range dataLocators {
			if strings.HasPrefix(locator, "data.") {
				locators = append(locators, locator)
			}
		}
	}
	return sortedUnique(locators)
}

func walkDataLocators(
	t reflect.Type,
	path []string,
	depth int,
	active map[reflect.Type]bool,
	locators map[string]struct{},
) {
	if depth >= maxLocatorDepth {
		return
	}
	t = locatorElementType(t)
	if t == nil || t.Kind() != reflect.Struct || active[t] {
		return
	}

	active[t] = true
	defer delete(active, t)

	for fieldIndex := range t.NumField() {
		field := t.Field(fieldIndex)
		if field.PkgPath != "" {
			continue
		}

		name, skip := jsonFieldName(field)
		if skip {
			continue
		}
		fieldPath := path
		if !anonymousStructField(field, name) {
			if name == "" {
				name = field.Name
			}
			fieldPath = appendPath(path, name)
		}

		if field.Tag.Get("ax") == "nondeterministic" {
			locators[strings.Join(fieldPath, ".")] = struct{}{}
		}
		walkDataLocators(field.Type, fieldPath, depth+1, active, locators)
	}
}

func locatorElementType(t reflect.Type) reflect.Type {
	for t != nil {
		kind := t.Kind()
		if kind != reflect.Array && kind != reflect.Map && kind != reflect.Pointer && kind != reflect.Slice {
			return t
		}
		t = t.Elem()
	}
	return nil
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	name, _, _ := strings.Cut(tag, ",")
	return name, false
}

func anonymousStructField(field reflect.StructField, jsonName string) bool {
	if !field.Anonymous || jsonName != "" {
		return false
	}
	t := locatorElementType(field.Type)
	return t != nil && t.Kind() == reflect.Struct
}

func appendPath(path []string, segment string) []string {
	next := make([]string, len(path)+1)
	copy(next, path)
	next[len(path)] = segment
	return next
}

func sortedUnique(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		unique[value] = struct{}{}
	}
	return sortedKeys(unique)
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
