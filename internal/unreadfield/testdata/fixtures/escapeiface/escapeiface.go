package escapeiface

type sent struct {
	v int
}

type stored struct {
	v int
}

func Leak() (chan any, map[string]any) {
	ch := make(chan any, 1)
	ch <- sent{v: 1}
	m := map[string]any{"k": stored{v: 2}}
	return ch, m
}
