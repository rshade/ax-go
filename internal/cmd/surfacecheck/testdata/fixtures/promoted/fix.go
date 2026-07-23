// Package fix is a synthetic scanner fixture covering promoted fields and
// methods: direct embedding, promotion through an unexported embedded type,
// and ambiguity cancellation for same-depth name collisions.
package fix

type Inner struct {
	A int
	b int
}

func (Inner) M() {}

type Outer struct {
	Inner
	X string
}

type Left struct{ Dup int }

type Right struct{ Dup string }

type Both struct {
	Left
	Right
}

type Repeated struct{ X int }

type ViaLeft struct{ Repeated }

type ViaRight struct{ Repeated }

type Diamond struct {
	ViaLeft
	ViaRight
}

type hidden struct{ Pub int }

type Wrap struct{ hidden }
