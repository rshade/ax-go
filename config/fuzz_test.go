package config

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rshade/ax-go/contract"
)

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"a":1}`), DefaultMaxBytes)
	f.Add([]byte("{}"), int64(2))
	f.Add([]byte("{}"), int64(1))
	f.Add([]byte("not json"), DefaultMaxBytes)
	f.Add([]byte(`{"a":1}`), int64(-1))
	f.Add([]byte(`{"a":1}`), MaxBytesCeiling+1)

	f.Fuzz(func(t *testing.T, data []byte, maxBytes int64) {
		var dst map[string]any
		err := Parse(context.Background(), bytes.NewReader(data), &dst, WithMaxBytes(maxBytes))
		if err == nil {
			return
		}

		var contractErr *contract.Error
		if !errors.As(err, &contractErr) {
			t.Fatalf("Parse returned non-*contract.Error: %T (%v)", err, err)
		}
		if contract.ErrorExitCode(err) != contractErr.ExitCode() {
			t.Fatalf("ErrorExitCode disagrees with envelope ExitCode")
		}
	})
}

func FuzzPatch(f *testing.F) {
	f.Add(`{"name":"ax","replicas":3}`, `[{"op":"replace","path":"/replicas","value":5}]`)
	f.Add(`{"a":{"b":{"c":1}}}`, `[{"op":"add","path":"/a/b/d","value":"new"}]`)
	f.Add(`{`, `[]`)
	f.Add(`{"a":1}`, `not json`)

	f.Fuzz(func(t *testing.T, existing, patch string) {
		_, _ = Patch(context.Background(), strings.NewReader(existing), []byte(patch))
	})
}
