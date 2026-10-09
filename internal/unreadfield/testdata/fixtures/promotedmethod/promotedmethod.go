package promotedmethod

type base struct {
	id int
}

func (b base) ID() int { return b.id }

type wrapper struct {
	base
	label string
}

func Use() (int, string) {
	w := wrapper{base: base{id: 1}, label: "x"}
	return w.ID(), w.label
}
