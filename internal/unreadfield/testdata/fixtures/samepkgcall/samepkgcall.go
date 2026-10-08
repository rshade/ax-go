package samepkgcall

type job struct {
	retries int // want `struct field job\.retries is assigned but never read`
	name    string
}

func describe(j job) string { return j.name }

func Describe() string { return describe(job{retries: 3, name: "x"}) }
