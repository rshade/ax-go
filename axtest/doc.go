// Package axtest is a full-lifecycle command test helper for CLIs built on
// ax-go. It wraps ax.Execute so a test can run a command tree through the same
// startup lifecycle a production binary uses — agent-safety flags mounted, mode
// resolved, context populated — and get back captured stdout, stderr, and the
// resulting exit code.
//
// This package is designed to be imported only from _test.go files. It depends
// on the full root ax package and Cobra without restriction: unlike the
// import-isolated contract packages (config, contract, id, logging, mcp,
// schema), axtest isolates discoverability and a stable home for test tooling,
// not binary size (research.md), so there is no size motivation to keep its
// dependency graph small. What it enforces instead runs the other direction —
// TestAxtestIsOnlyImportedFromTests (import_isolation_test.go) fails the build
// if any non-test source file anywhere else in this module imports axtest.
package axtest
