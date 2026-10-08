package storeforms

type indexed struct {
	items []int
}

type box struct {
	v int
}

type nested struct {
	inner box
}

type ranged struct {
	key int // want `struct field ranged\.key is assigned but never read`
}

func Run() {
	i := indexed{items: []int{1}}
	i.items[0] = 2
	n := nested{inner: box{}}
	n.inner.v = 1
	r := ranged{key: 0}
	for r.key = range 3 {
	}
}
