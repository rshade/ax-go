package clean

type row struct {
	name string
	want bool
}

func names() []string {
	rows := []row{{name: "a", want: true}}
	var out []string
	for _, r := range rows {
		if r.want {
			out = append(out, r.name)
		}
	}
	return out
}
