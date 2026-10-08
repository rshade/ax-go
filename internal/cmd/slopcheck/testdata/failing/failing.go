package failing

type row struct {
	skip  bool
	fixme int
	name  string
}

func names() []string {
	rows := []row{{skip: true, fixme: 1, name: "a"}, {name: "b", skip: false}}
	var out []string
	for _, r := range rows {
		out = append(out, r.name)
	}
	return out
}
