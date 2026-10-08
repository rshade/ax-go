package promoted

type base struct {
	id int
}

type wrapper struct {
	base
	label string
}

func ID() int {
	w := wrapper{base: base{id: 7}, label: "x"}
	return w.id + len(w.label)
}
