// Package sandbox abstracts command execution for agent tool invocations and
// owns the backend-neutral policy contract used to validate and apply runtime
// security intent.
//
// LocalCommandRunner executes commands directly on the host; sandbox backends
// report capabilities, validate policy, and store the effective policy before
// command execution proceeds.
//
// EnforcingCommandRunner composes a CommandPolicy around any runner so the
// sandbox layer can refuse execution before the backend process is launched.
//
// The package also carries filesystem-scope helpers used by sandbox-aware host
// tools so protected roots are denied before any host I/O occurs.
//
// command_runner.go selects the appropriate implementation based on the
// workspace configuration.
package sandbox
