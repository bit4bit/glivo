package chain

type CommandChainable struct {
	app string
	args string
}

type RunnerChainable interface{
	Reply() (string, error)
}
