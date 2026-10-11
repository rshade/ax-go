package contract

import (
	"slices"
	"testing"

	"github.com/rshade/ax-go/internal/testutil"
)

// TestErrorCodesAreRegistered is the drift guard for the error-code registry
// (spec 033). Every site in this module's library code that sets an
// error_code must name a registered contract constant, so a new code cannot
// reach an agent without being listed in KnownErrorCodes or the authoring-time
// exclusion. The checker parses every file whatever its build constraints, so
// this test covers all four tag configurations and every platform from any
// one of them. Its ability to fail is proven against a fixture in
// internal/testutil's TestFindErrorCodeViolationsFixture.
func TestErrorCodesAreRegistered(t *testing.T) {
	violations, err := testutil.FindErrorCodeViolations(testutil.ErrorCodeGuard{
		ModuleDir:       testutil.RepoRoot(t),
		ModulePath:      "github.com/rshade/ax-go",
		RootPackageName: "ax",
		Registered:      slices.Concat(KnownErrorCodes(), authoringTimeErrorCodes()),
		ExcludedDirs: []testutil.ExcludedDir{
			{
				Dir: "internal/cmd",
				Reason: "this repository's gate tools define their own codes and never run inside an adopting CLI; " +
					"spec 032 FR-001 excludes them from known_codes",
			},
			{
				Dir:    "examples",
				Reason: "adopter code; its codes are adopter-defined, which spec 032 FR-001 excludes from known_codes",
			},
		},
		PassThroughs: []testutil.PassThrough{
			{
				File: "error.go", Func: "NewError", Param: "code",
				Reason: "the root constructor forwards its caller's code to contract.NewError",
			},
			{
				File: "contract/error.go", Func: "NewError", Param: "code",
				Reason: "the constructor stores its caller's code in the ErrorCode field",
			},
		},
	})
	if err != nil {
		t.Fatalf("FindErrorCodeViolations() error = %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("%d error_code sites do not name a registered contract constant. Name an existing "+
			"contract.ErrorCode* constant, or add one to contract/codes.go and list it in KnownErrorCodes "+
			"(runtime) or authoringTimeErrorCodes:\n%s",
			len(violations), testutil.FormatErrorCodeViolations(violations))
	}
}
