package selfassign

type state struct {
	count int // want `struct field state\.count is assigned but never read`
}

func Reset() {
	s := state{count: 1}
	s.count = s.count
}
