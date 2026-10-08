package escapeeq

type point struct {
	x, y int
}

func Same() bool {
	a := point{x: 1, y: 2}
	return a == point{x: 1, y: 2}
}
