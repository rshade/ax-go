package escapenested

import "fmt"

type inner struct {
	code int
}

type outer struct {
	in inner
}

func Show() string {
	o := outer{in: inner{code: 1}}
	return fmt.Sprint(o)
}
