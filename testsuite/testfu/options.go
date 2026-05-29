package testfu

type Option func(*Agent)

func WithWorkspace(workspace string) Option {
	return func(agent *Agent) {
		agent.Workspace = workspace
	}
}

func WithRunner(runner suiteRunner) Option {
	return func(agent *Agent) {
		agent.Runner = runner
	}
}
