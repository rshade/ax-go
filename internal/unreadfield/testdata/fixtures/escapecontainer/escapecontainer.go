package escapecontainer

import "fmt"

type sliced struct {
	v int
}

type mapped struct {
	v int
}

type keyed struct {
	v int
}

type channeled struct {
	v int
}

type row struct {
	skip bool // want `struct field row\.skip is assigned but never read`
	name string
}

func Containers() (string, string, string) {
	as := []sliced{{v: 1}}
	s1 := fmt.Sprintf("%v", as)
	bm := map[string]mapped{"k": {v: 2}}
	out := make(chan any, 1)
	out <- bm
	cm := map[keyed]int{{v: 3}: 1}
	s2 := fmt.Sprint(cm)
	ch := make(chan channeled, 1)
	ch <- channeled{v: 4}
	s3 := fmt.Sprint(ch)
	rows := []row{{skip: true, name: "x"}}
	for _, r := range rows {
		_ = r.name
	}
	return s1, s2, s3
}
