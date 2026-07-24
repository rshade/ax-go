// Package logcore is the zerolog-backed logger implementation shared by the two
// public logging surfaces: the root package ax and the import-isolated package
// logging. Both alias the types declared here, so a logger obtained from either
// surface is the same type backed by the same construction path, the same
// backend, and the same trace-correlation hook.
//
// logcore is NOT a public surface. The Go toolchain forbids any module outside
// github.com/rshade/ax-go from importing it, and that path restriction is what
// carries Constitution Principle VI's no-pluggable-backend guardrail: Sink and
// LabelSanctioner must be exported so lokiWriter (which stays in package ax) can
// satisfy them across the package boundary, and an exported interface would
// otherwise invite an external backend registration. No external consumer can
// reach these names, so no external backend can be registered.
//
// The guardrail is therefore path-enforced rather than type-enforced. That is a
// deliberate, recorded narrowing: an ax-go maintainer could add a second backend
// without a compiler complaint, where an unexported interface would have
// objected. Review holds that line. Adding a second logger implementation or a
// second backend remains forbidden by Principle VI.
//
// logcore must never import root ax, and its dependency set is closed: stdlib,
// github.com/rs/zerolog, the OpenTelemetry trace API (never the SDK), and
// github.com/rshade/ax-go/contract for the zero-value ID constants. It contains
// no Loki-specific identifier; the direct-push addon reaches it only through the
// generic Sink and LabelSanctioner seams.
package logcore
