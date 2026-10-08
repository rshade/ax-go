package unexportedtype

type result struct {
	Status string // want `struct field result\.Status is assigned but never read`
}

func build() {
	r := result{Status: "ok"}
	_ = r
}
