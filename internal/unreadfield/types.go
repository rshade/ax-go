package unreadfield

import (
	"fmt"
	"strings"
)

// Build tags are stable public contract (AGENTS.md, Build Configurations).
const (
	tagNoGRPC = "ax_no_grpc"
	tagNoOTLP = "ax_no_otlp"
)

// FieldKey identifies one candidate field independently of build
// configuration and machine. Package is the unit's import path without any
// " [p.test]" variant suffix (an external test package keeps its _test
// suffix). Type is the declared type name; a type declared inside a function
// is suffixed "@<base filename>:line:col", and an anonymous struct type is
// rendered "struct{...}@<base filename>:line:col". Base filenames keep the
// key free of machine paths; Package already names the directory.
type FieldKey struct {
	Package string
	Type    string
	Field   string
}

// Position is a physical source location: //line directives are ignored.
// Run reports File slash-separated and relative to the analyzed module root;
// Analyze reports it as the file set recorded the parsed file's name.
type Position struct {
	File string
	Line int
	Col  int
}

// Finding is one field that is assigned in at least one composite literal
// and read nowhere. Assigned is sorted, deduplicated, and never empty.
type Finding struct {
	Key      FieldKey
	Declared Position
	Assigned []Position
}

// Result is the outcome of analyzing one unit in one build configuration.
// Assigned lists every candidate field set by at least one composite literal;
// Findings is the sorted subset of Assigned that is never read.
type Result struct {
	Assigned []FieldKey
	Findings []Finding
}

// Report is the outcome of Run across every configuration. Findings holds
// only fields that are findings in every configuration in which they are
// assigned, sorted by key, with assignment positions unioned across
// configurations.
type Report struct {
	Configurations int
	Packages       int
	Fields         int
	Findings       []Finding
}

// Configuration is one build-tag set passed to go list -tags.
type Configuration struct {
	Name string
	Tags []string
}

// AnalysisError reports that a configuration could not be loaded or
// type-checked, so no findings were computed for it. It wraps the cause, which
// may itself wrap context.DeadlineExceeded, context.Canceled, or
// fs.ErrPermission.
type AnalysisError struct {
	Configuration string
	Err           error
}

// Error implements error.
func (e *AnalysisError) Error() string {
	return fmt.Sprintf("configuration %s: %v", e.Configuration, e.Err)
}

// Unwrap returns the cause so errors.Is and errors.As reach it.
func (e *AnalysisError) Unwrap() error { return e.Err }

// String renders the position as file:line:col.
func (p Position) String() string {
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
}

// String renders the key as package.Type.Field.
func (k FieldKey) String() string {
	return k.Package + "." + k.Type + "." + k.Field
}

// Message is the diagnostic text both front ends emit for f.
func (f Finding) Message() string {
	return fmt.Sprintf("struct field %s.%s is assigned but never read", f.Key.Type, f.Key.Field)
}

// String renders f for the gate's envelope cause:
// "<declared>: <key> (assigned at <pos>, <pos>)".
func (f Finding) String() string {
	assigned := make([]string, len(f.Assigned))
	for i, p := range f.Assigned {
		assigned[i] = p.String()
	}
	return fmt.Sprintf("%s: %s (assigned at %s)", f.Declared, f.Key, strings.Join(assigned, ", "))
}

// BuildConfigurations returns the four supported build-tag configurations in
// canonical order: default, ax_no_grpc, ax_no_otlp, and both. It returns a
// fresh slice on every call. TestBuildTagMatrix keeps it equal to the
// Makefile's BUILD_TAG_MATRIX.
func BuildConfigurations() []Configuration {
	return []Configuration{
		{Name: "default"},
		{Name: tagNoGRPC, Tags: []string{tagNoGRPC}},
		{Name: tagNoOTLP, Tags: []string{tagNoOTLP}},
		{Name: tagNoGRPC + "+" + tagNoOTLP, Tags: []string{tagNoGRPC, tagNoOTLP}},
	}
}
