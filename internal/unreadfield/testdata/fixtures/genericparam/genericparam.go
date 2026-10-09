package genericparam

import "fmt"

type pointed struct {
	value int
}

type listed struct {
	value int
}

type plain struct {
	value int // want `struct field plain\.value is assigned but never read`
}

func viaPointer[T any](p *T) string { return fmt.Sprint(*p) }

func viaSlice[T any](items []T) string { return fmt.Sprint(items) }

func keep(p *plain) *plain { return p }

func Use() string {
	keep(&plain{value: 3})
	return viaPointer(&pointed{value: 1}) + viaSlice([]listed{{value: 2}})
}
