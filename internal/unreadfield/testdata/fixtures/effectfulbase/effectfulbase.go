package effectfulbase

type slot struct {
	f int
}

type queued struct {
	f int
}

var (
	pool = []*slot{{f: 1}, {f: 2}}
	turn int
)

func next() *slot {
	turn++
	return pool[turn%len(pool)]
}

// Rotate copies one slot's f into another: each call returns a different
// slot, so the right-hand side is a real read.
func Rotate() { next().f = next().f }

// Drain does the same through two channel receives.
func Drain(c chan *queued) {
	c <- &queued{f: 3}
	(<-c).f = (<-c).f
}
