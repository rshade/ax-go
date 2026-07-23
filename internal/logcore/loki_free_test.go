package logcore

import (
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lokiIdentifiers returns the tokens whose presence in this package's CODE would
// mean the Loki direct-push addon has been coupled back into the core logger.
// Constitution Principle VIII forbids that coupling outright; FR-010 removed the
// two *lokiWriter type assertions that used to sit in logger.go, and this test is
// the standing assertion that they do not come back under a new name.
func lokiIdentifiers() []string {
	return []string{
		"loki",
		"Loki",
		"lokiWriter",
		"labelPair",
		"AX_LOKI_URL",
	}
}

// TestPackageContainsNoLokiIdentifier covers C-13.
//
// It scans source rather than inspecting the type graph because the failure mode
// it guards against is a human reintroducing a concrete-type shortcut — which
// reads perfectly at the call site and is invisible to every other gate. The sink
// seam must stay generic: logcore knows about Sink and LabelSanctioner, never
// about who implements them.
//
// Comments are deliberately excluded from the scan, and that exclusion is the
// contract, not a loophole. C-13 forbids a Loki-specific IDENTIFIER, which is a
// code construct. Prose explaining WHY the addon must stay in package ax is the
// most valuable thing a future maintainer can read at this boundary, and a test
// that forbade naming the constraint would delete its own rationale. Scanning
// tokens instead of raw bytes is therefore both stricter (a "loki" substring
// inside an unrelated word no longer trips it) and narrower in exactly the right
// place.
func TestPackageContainsNoLokiIdentifier(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// This file necessarily names the forbidden tokens in order to look for
		// them, so it excludes itself.
		if name == "loki_free_test.go" {
			continue
		}

		scanned++
		for _, found := range scanLokiTokens(t, name) {
			t.Errorf(
				"%s references Loki-specific identifier %q in code: "+
					"the log-shipping addon must reach logcore only through the generic "+
					"Sink and LabelSanctioner seams (FR-010, Constitution Principle VIII)",
				name, found,
			)
		}
	}

	// A scan that examined nothing would pass silently, which is the exact shape
	// of gate this feature's tasks warn against.
	if scanned == 0 {
		t.Fatal("scanned no Go files; the C-13 assertion would pass without enforcing anything")
	}
}

// scanLokiTokens returns every forbidden identifier appearing in a non-comment
// token of the named file. Identifiers match case-insensitively on substring so a
// rename to lokiSink or LokiLabelWriter is still caught; string literals are
// checked too, because AX_LOKI_URL would arrive as one.
func scanLokiTokens(t *testing.T, name string) []string {
	t.Helper()

	content, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	fset := token.NewFileSet()
	file := fset.AddFile(name, fset.Base(), len(content))

	var s scanner.Scanner
	s.Init(file, content, func(pos token.Position, msg string) {
		t.Fatalf("scan %s: %s: %s", name, pos, msg)
	}, 0) // mode 0: comments are not emitted as tokens

	var found []string
	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		if tok != token.IDENT && tok != token.STRING {
			continue
		}
		lower := strings.ToLower(lit)
		for _, identifier := range lokiIdentifiers() {
			if strings.Contains(lower, strings.ToLower(identifier)) {
				found = append(found, lit)
				break
			}
		}
	}
	return found
}
