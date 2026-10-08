package alias

type impl struct {
	Name  string
	cache int // want `struct field impl\.cache is assigned but never read`
}

// Impl exposes impl, so its exported fields are nameable downstream.
type Impl = impl

func mk() {
	v := impl{Name: "a", cache: 1}
	_ = v
}
