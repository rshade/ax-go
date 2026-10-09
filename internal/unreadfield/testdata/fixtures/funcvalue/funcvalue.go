package funcvalue

type made struct {
	Value int
}

// Factory hands callers outside the package a function that returns made.
func Factory() func() made {
	return func() made { return made{Value: 1} }
}

type local struct {
	value int // want `struct field local\.value is assigned but never read`
}

// UseLocal calls a function value that never leaves the package.
func UseLocal() {
	mk := func() local { return local{value: 2} }
	l := mk()
	_ = l
}
