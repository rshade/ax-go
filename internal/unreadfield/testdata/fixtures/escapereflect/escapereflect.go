package escapereflect

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type compared struct {
	v int
}

type formatted struct {
	v int
}

type encoded struct {
	V int
}

func All() (bool, string, error) {
	a := compared{v: 1}
	ok := reflect.DeepEqual(a, compared{v: 1})
	s := fmt.Sprintf("%v", formatted{v: 2})
	_, err := json.Marshal(encoded{V: 3})
	return ok, s, err
}
