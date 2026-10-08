package dynamiccall

type task struct {
	prio int
}

func run(t task) {}

func Dispatch() {
	f := run
	f(task{prio: 1})
}
