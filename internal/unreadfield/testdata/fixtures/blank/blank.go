package blank

type padded struct {
	_    int
	used int
}

func Used() int {
	p := padded{0, 1}
	return p.used
}
