// Package unreadfield reports struct fields that a composite literal assigns
// and no code reads: the table-test field (expectError bool) that every case
// sets and no assertion consults.
//
// The package is harness-agnostic and imports only the standard library.
// Analyze classifies one type-checked unit from the values every Go analysis
// harness already holds (a file set, syntax, the package, and types.Info), so
// the go/analysis adapter in the analyzer subpackage and the slopcheck gate
// share one classification and cannot disagree. Run loads a module tree with
// go list once per build configuration and intersects the results; it is what
// the gate (internal/cmd/slopcheck) enforces.
//
// Classification fails toward silence. A field counts as read whenever its
// value could be observed without being named (equality, interface
// conversion, reflection, code outside the package), because a blocking gate
// must not report a field a reflection-based reader consumes. Nothing in this
// package keeps state between calls.
package unreadfield
