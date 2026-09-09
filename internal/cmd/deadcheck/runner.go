package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const analysisTimeout = 5 * time.Minute

// cappedBuffer bounds both subprocess streams at the write boundary. Returning
// an error stops copying oversized output; the context also bounds execution.
type cappedBuffer struct{ buffer bytes.Buffer }

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) > maxReportBytes-b.buffer.Len() {
		return 0, fmt.Errorf("deadcode output exceeds %d bytes", maxReportBytes)
	}
	return b.buffer.Write(p)
}

func deadcode(ctx context.Context, tags string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, analysisTimeout)
	defer cancel()
	// deadcode's default module regex ends in a word boundary, which
	// excludes the root external-test package because underscore is a word
	// character. Include that synthetic package explicitly.
	filter := "-filter=^" + regexp.QuoteMeta(modulePath) + "($|/|_test$)"
	// #nosec G204 -- tags and filter are fixed policy; no shell interprets arguments.
	cmd := exec.CommandContext(ctx, "deadcode", "-test", "-json", "-tags="+tags, filter, "./...")
	var stdout, stderr cappedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("deadcode: %w", ctx.Err())
		}
		return nil, fmt.Errorf("deadcode: %w: %s", err, strings.TrimSpace(stderr.buffer.String()))
	}
	if stderr.buffer.Len() != 0 {
		return nil, fmt.Errorf("deadcode wrote unexpected diagnostics: %s", strings.TrimSpace(stderr.buffer.String()))
	}
	return stdout.buffer.Bytes(), nil
}
