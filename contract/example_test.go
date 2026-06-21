package contract_test

import (
	"context"
	"fmt"
	"os"

	"github.com/rshade/ax-go/contract"
)

// ExampleNewEnvelope shows the strict success envelope produced by the
// import-isolated contract package.
func ExampleNewEnvelope() {
	type result struct {
		ID string `json:"id"`
	}

	env := contract.NewEnvelope(context.Background(), result{ID: "abc"})
	if err := contract.WriteJSON(os.Stdout, env); err != nil {
		fmt.Println("error:", err)
	}
	// Output: {"data":{"id":"abc"},"meta":{"trace_id":"00000000000000000000000000000000","span_id":"0000000000000000"}}
}

// ExampleNewError shows a structured error envelope and its deterministic exit
// code without importing the root runtime package.
func ExampleNewError() {
	err := contract.NewError(
		context.Background(),
		"config_too_large",
		"config exceeds maximum size of 1048576 bytes",
		contract.WithActionableFix("reduce the config or raise the limit"),
		contract.WithErrorExitCode(contract.ExitValidation),
	)

	fmt.Println(err)
	fmt.Println(contract.ErrorExitCode(err))
	// Output:
	// config exceeds maximum size of 1048576 bytes
	// 2
}

// ExampleMode shows output-mode precedence in the isolated contract package.
func ExampleMode() {
	mode, err := contract.ResolveMode("", "", false)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(mode)
	// Output: json
}

// ExampleEnvelope shows the generic success-envelope shape directly.
func ExampleEnvelope() {
	env := contract.Envelope[string]{
		Data: "hello",
		Meta: contract.Metadata{TraceID: contract.ZeroTraceID},
	}
	if err := contract.WriteJSON(os.Stdout, env); err != nil {
		fmt.Println("error:", err)
	}
	// Output: {"data":"hello","meta":{"trace_id":"00000000000000000000000000000000"}}
}
