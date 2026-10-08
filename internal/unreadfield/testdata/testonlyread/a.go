package testonlyread

type opts struct {
	debug bool
}

type hidden struct {
	Level int
}

func newOpts() opts { return opts{debug: true} }

func makeHidden() hidden { return hidden{Level: 2} }
