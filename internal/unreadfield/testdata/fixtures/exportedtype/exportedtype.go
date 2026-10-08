package exportedtype

// Options is exported, so its exported fields may be read downstream.
type Options struct {
	Verbose bool
	retries int // want `struct field Options\.retries is assigned but never read`
}

func defaults() {
	o := Options{Verbose: true, retries: 1}
	_ = o
}
