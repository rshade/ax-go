package schema

import (
	"github.com/spf13/cobra"

	internalschema "github.com/rshade/ax-go/internal/schema"
)

// Capability is a command's side-effect class from ax-go's fixed vocabulary.
// It is the machine-comparable value an agent branches on, and its meaning is
// identical across every ax-go CLI. Only the six Capability constants are
// valid; WithCapability rejects any other value.
type Capability string

const (
	// CapabilityReadOnly marks a command that observes state only. An agent may
	// call it speculatively.
	CapabilityReadOnly Capability = "read-only"
	// CapabilityCreate marks a command that creates new state. A retry may
	// create a duplicate unless the call carries --idempotency-key.
	CapabilityCreate Capability = "create"
	// CapabilityMutate marks a command that changes existing state and may
	// overwrite it.
	CapabilityMutate Capability = "mutate"
	// CapabilityDelete marks a command that removes state.
	CapabilityDelete Capability = "delete"
	// CapabilityExternalNetwork marks a command that reaches a network endpoint
	// outside the host. It says nothing about whether state changes.
	CapabilityExternalNetwork Capability = "external-network"
	// CapabilityAdmin marks a privileged or administrative operation.
	CapabilityAdmin Capability = "admin"
)

// ErrInvalidDeclaration is wrapped (%w) by every authoring error returned from
// WithFlagEnum, WithFlagExample, and WithCapability, so errors.Is identifies
// them. It is a programmer error, not bad agent input: if it reaches ax.Execute
// it maps to exit code 1, not 2.
var ErrInvalidDeclaration = internalschema.ErrInvalidDeclaration

// WithFlagEnum declares the only values the named flag accepts. The flag is
// looked up in cmd.Flags(), then cmd.PersistentFlags(); a persistent flag's
// enum applies on every descendant command.
//
// On success __schema lists values, in the given order, as the flag's enum,
// and __schema --as=mcp emits them as a JSON-Schema enum typed to the property.
// Enforcement happens in the flag's Set while flags are parsed, which is before
// PersistentPreRunE and RunE: a value outside the set is rejected with
// error_code validation_error and exit code 2 even under --dry-run, through
// both ax.Execute and the mcp-server dispatcher. The rejection envelope carries
// the flag name, the allowed values and one --flag=value suggestion per value,
// and never the rejected input. The flag's own default is always accepted, and
// an empty default is exempt from membership. Integer flags compare
// numerically (03 matches 3); string and custom types compare exactly. Calling
// WithFlagEnum again replaces the set.
//
// Supported types are string, the integer scalars (int, int8-int64, uint,
// uint8-uint64) and custom pflag.Value types that are not slices. Any other
// type, a nil cmd, a missing flag, an empty or unparsable value list,
// duplicates after canonicalisation, a non-empty default outside the set, or a
// declared example outside the set returns an error wrapping
// ErrInvalidDeclaration and leaves cmd unchanged.
//
// After a successful call flag.Value is a wrapper, no longer the concrete
// pflag type: do not type-assert it.
func WithFlagEnum(cmd *cobra.Command, flag string, values ...string) error {
	return internalschema.DeclareFlagEnum(cmd, flag, values)
}

// WithFlagExample attaches one example value to the named flag, written in CLI
// form exactly as it would appear on argv; for a slice flag write the elements
// as CSV, for example "a,b". The flag is looked up in cmd.Flags(), then
// cmd.PersistentFlags().
//
// On success __schema emits the value as the flag's example, and
// __schema --as=mcp emits it as a one-element JSON-Schema examples array typed
// to the property. Calling WithFlagExample again replaces the example.
//
// It returns an error wrapping ErrInvalidDeclaration, and leaves the previous
// example in place, when cmd is nil or the flag is not found, the example is
// empty, the example does not parse as the flag's built-in type, or the flag
// has an enum (WithFlagEnum) and the example is not a member. Values for custom
// pflag.Value types are not type-checked.
func WithFlagExample(cmd *cobra.Command, flag string, example string) error {
	return internalschema.DeclareFlagExample(cmd, flag, example)
}

// WithCapability classifies cmd's side effects with one class from the fixed
// vocabulary. note is optional free-form detail, for example
// "idempotent by name"; surrounding whitespace is trimmed and an empty note is
// omitted. Calling WithCapability again replaces both class and note.
//
// On success the command node in __schema carries capability {class, note}.
// The __schema --as=mcp tool carries the same capability object plus the
// standard MCP annotations hints (see MCPToolAnnotations), and the live
// mcp-server tool carries the hints. Hidden and reserved commands keep the
// declaration but stay out of the MCP tool set. A command that never calls
// WithCapability is unclassified: the field is omitted and no class is
// implied.
//
// It returns an error wrapping ErrInvalidDeclaration, leaving cmd unchanged,
// when cmd is nil or class is not exactly one of the six Capability constants.
func WithCapability(cmd *cobra.Command, class Capability, note string) error {
	return internalschema.DeclareCapability(cmd, string(class), note)
}
