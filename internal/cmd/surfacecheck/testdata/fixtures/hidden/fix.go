// Package fix is a synthetic scanner fixture covering reachable hidden
// concrete types: exported declarations that expose unexported types through
// their signatures, and interface results that expose only the interface
// method set.
package fix

type result struct{ V int }

func (r *result) Next() bool { return true }

func New() *result { return &result{} }

type It interface{ Step() }

func NewIt() It { return nil }

type it2 interface{ X() }

func NewIt2() it2 { return nil }
