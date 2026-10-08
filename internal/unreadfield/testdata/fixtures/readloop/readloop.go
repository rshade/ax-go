package readloop

func Count() int {
	tests := []struct {
		name string
		want bool
	}{
		{name: "a", want: true},
	}
	n := 0
	for _, tc := range tests {
		if tc.want {
			n++
		}
		_ = tc.name
	}
	return n
}
