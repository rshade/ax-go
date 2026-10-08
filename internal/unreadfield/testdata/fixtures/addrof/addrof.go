package addrof

type cfg struct {
	level int
}

func setLevel(p *int) { *p = 3 }

func Apply() {
	c := cfg{level: 1}
	setLevel(&c.level)
}
