package generic

type box[T any] struct {
	val   T
	label string // want `struct field box\.label is assigned but never read`
}

func Sum() int {
	b := box[int]{val: 1, label: "x"}
	c := box[string]{val: "s", label: "y"}
	return b.val + len(c.val)
}
