package taggedread

type flags struct {
	grpc bool
}

func load() flags { return flags{grpc: true} }
