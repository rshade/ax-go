package positional

type pair struct {
	left  int // want `struct field pair\.left is assigned but never read`
	right int // want `struct field pair\.right is assigned but never read`
}

func Make() {
	p := pair{1, 2}
	_ = p
}
