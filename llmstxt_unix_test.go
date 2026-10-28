//go:build unix

package ax

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rshade/ax-go/contract"
)

func TestGenerateLLMsTxtRejectsPreambleThatGrowsPastTheCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opening.md")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	root := discoveryTree(t)
	writerDone := make(chan error, 1)
	go func() {
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			writerDone <- err
			return
		}
		defer file.Close()
		payload := append([]byte("# Tool\n\n> Summary.\n"), bytes.Repeat([]byte("x"), int(DefaultMaxConfigBytes)+1)...)
		_, err = file.Write(payload)
		writerDone <- err
	}()

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- GenerateLLMsTxt(context.Background(), &buf, root, WithPreambleFile(path))
	}()

	var err error
	select {
	case err = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("generation did not finish")
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %d bytes", buf.Len())
	}
	var got *contract.Error
	if !errors.As(err, &got) {
		t.Fatalf("error = %T %v, want *contract.Error", err, err)
	}
	if got.ErrorCode != "validation_error" || got.ExitCode() != 2 {
		t.Fatalf("error_code = %q exit = %d", got.ErrorCode, got.ExitCode())
	}
	if !strings.Contains(got.Error(), "exceeds 1 MiB") {
		t.Fatalf("error = %v, want the size cap", err)
	}

	select {
	case werr := <-writerDone:
		if werr != nil && !errors.Is(werr, syscall.EPIPE) {
			t.Fatalf("writer: %v", werr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not finish")
	}
}
