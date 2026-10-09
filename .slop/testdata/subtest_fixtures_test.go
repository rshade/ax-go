//go:build ignore

package slopfixture

import (
	"testing"

	"github.com/rshade/ax-go/axtest"
	"github.com/rshade/ax-go/internal/testutil"
)

func TestSubtestAssertionFixtures(t *testing.T) {
	t.Run("asserts nothing", func(t *testing.T) {
		_ = 1
	})
	t.Run("fatalf asserts", func(t *testing.T) {
		t.Fatalf("no")
	})
	t.Run("decode asserts", func(t *testing.T) {
		axtest.Decode(t, []byte(`{}`), &struct{}{})
	})
	t.Run("run and decode asserts", func(t *testing.T) {
		axtest.RunAndDecode(t, nil, nil, &struct{}{})
	})
	t.Run("compare outputs asserts", func(t *testing.T) {
		testutil.CompareOutputs(t, nil, nil)
	})
	t.Run("validate timestamps asserts", func(t *testing.T) {
		testutil.ValidateTimestamps(t, nil)
	})
	t.Run("fully typed asserts", func(t *testing.T) {
		testutil.AssertFullyTyped(t, nil)
	})
	t.Run("no forbidden imports asserts", func(t *testing.T) {
		testutil.AssertNoForbiddenImports(t, nil)
	})
	t.Run("run only", func(t *testing.T) {
		_, _ = axtest.Run(t, nil, nil)
	})
	t.Run("logging surface isolated asserts", func(t *testing.T) {
		testutil.AssertLoggingSurfaceIsolated(nil, t, "")
	})
	t.Run("no production import asserts", func(t *testing.T) {
		testutil.AssertNoProductionImport(nil, t, "", "", testutil.Profile{})
	})
	t.Run("config error helper asserts", func(t *testing.T) {
		assertConfigError(t, nil, "")
	})
	t.Run("patch error helper asserts", func(t *testing.T) {
		assertPatchError(t, nil, "")
	})
	t.Run("contract error helper asserts", func(t *testing.T) {
		assertContractError(t, nil, "")
	})
	t.Run("drift helper asserts", func(t *testing.T) {
		assertDrift(t, nil, nil)
	})
}
