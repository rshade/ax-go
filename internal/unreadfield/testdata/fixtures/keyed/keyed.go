package keyed

type row struct {
	name   string
	unread bool // want `struct field row\.unread is assigned but never read`
}

func Use() string {
	r := row{name: "a", unread: true}
	return r.name
}
