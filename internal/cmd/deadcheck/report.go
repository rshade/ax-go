package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"io"
	"slices"
	"strings"
)

const (
	modulePath     = "github.com/rshade/ax-go"
	maxReportBytes = 16 << 20
)

// These structs mirror the pinned deadcode -json protocol, not ax's output.
type deadPackage struct {
	Name  string         `json:"Name"`
	Path  string         `json:"Path"`
	Funcs []deadFunction `json:"Funcs"`
}

type deadFunction struct {
	Name      string   `json:"Name"`
	Position  position `json:"Position"`
	Generated bool     `json:"Generated"`
	Marker    bool     `json:"Marker"`
}

type position struct {
	File string `json:"File"`
	Line int    `json:"Line"`
	Col  int    `json:"Col"`
}

type finding struct {
	Symbol   string
	Position position
}

func parseReport(data []byte) (map[string]finding, error) {
	if len(data) > maxReportBytes {
		return nil, fmt.Errorf("deadcode report exceeds %d bytes", maxReportBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var packages []deadPackage
	if err := decoder.Decode(&packages); err != nil {
		return nil, fmt.Errorf("decode deadcode report: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("expected exactly one deadcode JSON document")
	}
	findings := make(map[string]finding)
	for _, pkg := range packages {
		if pkg.Name == "" || pkg.Path == "" || len(pkg.Funcs) == 0 {
			return nil, errors.New("deadcode package lacks name, path, or functions")
		}
		for _, fn := range pkg.Funcs {
			if fn.Name == "" || fn.Position.File == "" || fn.Position.Line <= 0 || fn.Position.Col <= 0 {
				return nil, fmt.Errorf("invalid deadcode function in %s", pkg.Path)
			}
			if !inScope(pkg.Path, fn.Name) {
				continue
			}
			key := pkg.Path + "." + fn.Name
			entry := finding{Symbol: key, Position: fn.Position}
			// Test variants can repeat a source function. Pick the same diagnostic
			// regardless of report order; positions are not cross-build identity.
			prior, exists := findings[key]
			if !exists || describeFinding(entry) < describeFinding(prior) {
				findings[key] = entry
			}
		}
	}
	return findings, nil
}

func inScope(pkg, name string) bool {
	// External test packages use the tested import path plus _test, including
	// the root package where that suffix is not separated by a slash.
	pkg = strings.TrimSuffix(pkg, "_test")
	if pkg != modulePath && !strings.HasPrefix(pkg, modulePath+"/") {
		return false
	}
	if strings.Contains(pkg+"/", "/internal/") {
		return true
	}
	// deadcode renders methods as receiver.Method. Exported methods remain
	// library API even when their receiver type is hidden behind a constructor.
	method := name[strings.LastIndex(name, ".")+1:]
	return !ast.IsExported(method)
}

func describeFinding(f finding) string {
	return fmt.Sprintf("%s:%d:%d: %s", f.Position.File, f.Position.Line, f.Position.Col, f.Symbol)
}

func describeFindings(findings map[string]finding) string {
	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		lines = append(lines, describeFinding(f))
	}
	slices.Sort(lines)
	return strings.Join(lines, "; ")
}
