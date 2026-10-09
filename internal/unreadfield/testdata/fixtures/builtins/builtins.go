package builtins

import "fmt"

type boxed struct {
	value int
}

type deleted struct {
	key int
}

type kept struct {
	value int // want `struct field kept\.value is assigned but never read`
}

func Append() string {
	s := append([]any{}, boxed{value: 1})
	return fmt.Sprint(len(s))
}

func Delete(m map[deleted]int) {
	delete(m, deleted{key: 1})
}

func Keep() int {
	k := append([]kept{}, kept{value: 3})
	return len(k)
}
