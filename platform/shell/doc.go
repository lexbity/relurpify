// Package shell provides shell execution tools that agents may invoke to run
// bash commands within the sandbox.
//
// execution.go implements the shell execution tool, which runs a command
// string via bash -c inside the active sandbox runner and returns stdout,
// stderr, and exit code as a structured observation.
//
// cli_registry.go maintains a registry of known CLI tools discovered on the
// host, used by agents to check availability before invoking a binary.
// process_metadata.go records metadata about spawned processes for audit and
// telemetry purposes.
package shell
