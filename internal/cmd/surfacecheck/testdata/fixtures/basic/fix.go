// Package fix is a synthetic scanner fixture covering the basic exported
// declaration shapes: consts, vars, funcs, structs with fields, value and
// pointer methods, interfaces with embedding, and a type alias.
package fix

import "io"

const Answer = 42

const Named string = "x"

var Count int

var Default = Config{Name: "d"}

type Config struct {
	Name  string
	Count int
}

func (Config) Get() string { return "" }

func (c *Config) Set(s string) { c.Name = s }

type Stringer interface{ String() string }

type ReadWriter interface {
	io.Reader
	io.Writer
}

type Alias = Config

func Do(v int) string { return "" }

func Pick[T any](v T) T { return v }
