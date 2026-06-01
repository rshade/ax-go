package ax

import (
	"bytes"
	"os"
	"testing"
)

func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\nwant: %s\ngot:  %s", path, want, got)
	}
}
