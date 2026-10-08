package compound

type counter struct {
	hits  int
	total int
}

func Bump() {
	c := counter{hits: 1, total: 2}
	c.hits++
	c.total += 5
}
