package anontable

func Check() int {
	tests := []struct {
		name        string
		expectError bool // want `struct field struct\{\.\.\.\}@anontable\.go:4:13\.expectError is assigned but never read`
	}{
		{name: "a", expectError: true},
	}
	n := 0
	for _, tc := range tests {
		n += len(tc.name)
	}
	return n
}

func Local() {
	type row struct {
		hidden int // want `struct field row@anontable\.go:18:7\.hidden is assigned but never read`
	}
	r := row{hidden: 1}
	_ = r
}
