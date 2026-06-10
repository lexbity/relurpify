// Package ports defines governance-owned consumer interfaces and types.
// AccessRequest, Decision, Principal, Action, and Resource are governance's
// own request vocabulary — built by callers (execution, toolcapabilities, TUI)
// from their own types and consumed by governance/authorization.Enforcer.
package ports

import "context"

// Principal represents the identity making a request as resolved by governance.
// It is set by execution/agentlifecycle before any Check call and carried in
// context. No code outside execution/agentlifecycle may write it.
type Principal struct {
	AgentID    string
	Delegation []DelegationLink
}

// DelegationLink represents one step in a resolved delegation chain.
type DelegationLink struct {
	FromAgentID string
	ToAgentID   string
}

type contextKey string

const principalKey contextKey = "governance:principal"

// ContextWithPrincipal returns a new context carrying the given Principal.
// Only execution/agentlifecycle should call this.
func ContextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFromContext recovers the Principal from context.
// Returns zero-value Principal if not set.
func PrincipalFromContext(ctx context.Context) Principal {
	p, _ := ctx.Value(principalKey).(Principal)
	return p
}

// Action is the governance-owned type for "what" is being requested.
type Action string

const (
	ActionToolInvoke  Action = "tool.invoke"
	ActionFileRead    Action = "file.read"
	ActionFileWrite   Action = "file.write"
	ActionFileEdit    Action = "file.edit"
	ActionFileExecute Action = "file.execute"
	ActionExecBinary  Action = "exec.binary"
	ActionNetEgress   Action = "net.egress"
	ActionCapability  Action = "capability.invoke"
	ActionIPC         Action = "ipc.communicate"
)

// Resource is the governance-owned type for "on what" is being requested.
type Resource struct {
	Kind string // "path", "host", "tool", "binary", "capability"
	ID   string
}

// AccessRequest is the governance-owned input for every authorization check.
// It is built by callers (execution, toolcapabilities, TUI) from their own
// types. There is deliberately no shared adapters package.
type AccessRequest struct {
	Principal Principal
	Action    Action
	Resource  Resource
}

// Decision is the governance-owned output of every authorization check.
// It is pure over its input and performs no I/O.
type Decision struct {
	Allow       bool
	Reason      string
	Obligations []Obligation
}

// Obligation is a governance-imposed side-effect that must be fulfilled
// before or after the action executes (e.g. audit, require-approval,
// egress-proxy).
type Obligation struct {
	Kind string // "audit", "require-approval", "egress-proxy"
	ID   string
}
