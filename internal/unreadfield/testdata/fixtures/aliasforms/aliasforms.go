package aliasforms

type pointed struct {
	Value int
}

// Ptr exposes pointed through a pointer.
type Ptr = *pointed

type anon = struct {
	Count int
}

// Anon exposes an anonymous struct through an unexported alias.
type Anon = anon

type settings struct {
	Name string
}

// Default exposes settings through an exported variable.
var Default = settings{Name: "d"}

type inner struct {
	Depth int
}

type promoted struct {
	Level int
}

// Outer exposes inner through an exported field, and promoted, with the
// embedded field itself, through promotion.
type Outer struct {
	Inner inner
	promoted
}

type private struct {
	Hidden int // want `struct field private\.Hidden is assigned but never read`
}

func mk() {
	p := pointed{Value: 1}
	a := anon{Count: 2}
	o := Outer{Inner: inner{Depth: 3}, promoted: promoted{Level: 4}}
	h := private{Hidden: 5}
	_, _, _, _ = p, a, o, h
}
