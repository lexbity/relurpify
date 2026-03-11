// Package git provides agent-accessible git tools for common version-control
// operations within the agent's workspace.
//
// Exposed operations include: status, diff, log, show, clone, add, commit,
// push, pull, branch, checkout, and stash. Each operation is implemented as
// a registered capability tool that runs git via the sandbox runner and
// returns structured output for the agent to reason over.
package git
