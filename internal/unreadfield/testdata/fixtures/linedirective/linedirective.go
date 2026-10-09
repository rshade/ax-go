package linedirective

//line linedirective.go:1000
type long struct {
	unread int // want `struct field long\.unread is assigned but never read`
}

//line other.go:3
type renamed struct {
	unread int // want `struct field renamed\.unread is assigned but never read`
}

func mk() {
	l := long{unread: 1}
	r := renamed{unread: 2}
	_, _ = l, r
}
