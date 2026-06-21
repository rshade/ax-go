package id_test

import (
	"fmt"

	"github.com/rshade/ax-go/id"
)

func ExampleNewIdempotencyKey() {
	key := id.NewIdempotencyKey()
	fmt.Println(len(key))
	// Output: 36
}

func ExampleNewEntityID() {
	entityID, err := id.NewEntityID()
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(len(entityID))
	// Output: 36
}
