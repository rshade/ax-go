package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBoundedOutput(t *testing.T) {
	var output cappedBuffer
	// Hide strings.Reader.WriteTo so io.Copy exercises any ReaderFrom method.
	input := struct{ io.Reader }{strings.NewReader(strings.Repeat("x", maxReportBytes+1))}
	if _, err := io.Copy(&output, input); err == nil {
		t.Fatal("oversized subprocess output accepted")
	}
}

func TestDeadcodeProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable uses a POSIX shell")
	}
	for _, tc := range []struct {
		name, script string
		fail         bool
	}{
		{"valid", `test "$1" = -test && test "$2" = -json && test "$3" = -tags=ax_no_grpc && test "$4" = '-filter=^github\.com/rshade/ax-go($|/|_test$)' && test "$5" = ./... || exit 9
printf 'null'`, false},
		{"diagnostic", "echo warning >&2\nprintf 'null'", true},
		{"build_failure", "echo 'load failed' >&2\nexit 1", true},
		{"malformed", "printf 'not json'", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(dir, "deadcode"),
				[]byte("#!/bin/sh\n"+tc.script+"\n"),
				0o700,
			); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			data, err := deadcode(t.Context(), "ax_no_grpc")
			if (err != nil) != tc.fail {
				t.Fatalf("output %q error %v", data, err)
			}
			if tc.name == "valid" && string(data) != "null" {
				t.Errorf("output = %q", data)
			}
		})
	}
}

func TestDeadcodeMissingAndCanceled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := deadcode(t.Context(), ""); err == nil {
		t.Fatal("missing executable passed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := deadcode(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
