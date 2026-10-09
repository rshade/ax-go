package anoncopy

// Each function copies a literal into a distinct but identical struct type
// and reads the field only through the copy.

func Assign() int {
	x := struct{ value int }{value: 1}
	var y struct{ value int }
	y = x
	return y.value
}

func Declare() int {
	x := struct{ total int }{total: 2}
	var y struct{ total int } = x
	return y.total
}

func helper(p struct{ count int }) int { return p.count }

func Arg() int {
	z := struct{ count int }{count: 3}
	return helper(z)
}

func made() struct{ n int } {
	v := struct{ n int }{n: 4}
	return v
}

func Return() int { return made().n }

func Element() int {
	x := struct{ e int }{e: 5}
	s := []struct{ e int }{x}
	return s[0].e
}

func Range() int {
	var r struct{ k int }
	for _, r = range []struct{ k int }{{k: 8}} {
	}
	return r.k
}

type named struct {
	size int
}

func Unnamed() int {
	var u struct{ size int } = named{size: 6}
	return u.size
}

type control struct {
	kept int // want `struct field control\.kept is assigned but never read`
}

// Control copies within one named type, so the field objects are shared.
func Control() {
	c := control{kept: 7}
	d := c
	_ = d
}
