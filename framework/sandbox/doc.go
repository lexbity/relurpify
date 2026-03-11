// Package sandbox abstracts command execution for agent tool invocations,
// providing both local and gVisor-sandboxed runners behind a common interface.
//
// Sandbox is the interface all runners implement. LocalCommandRunner executes
// commands directly on the host; the gVisor-backed runner (wired at startup
// when --no-sandbox is not set) runs commands inside an isolated container,
// preventing agent-executed code from affecting the host filesystem or network
// beyond what the agent manifest permits.
//
// command_runner.go selects the appropriate implementation based on the
// workspace configuration and the --no-sandbox flag.
package sandbox
