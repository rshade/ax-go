package assignrhs

type settings struct {
	port int // want `struct field settings\.port is assigned but never read`
}

func newSettings() settings { return settings{port: 80} }

func Build() {
	x := settings{port: 1}
	y := x
	for _, tc := range []settings{{port: 2}} {
		tc := tc
		_ = tc
	}
	z := newSettings()
	_ = y
	_ = z
}
