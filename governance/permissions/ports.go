// Package permissions owns governance-side permission *behavior*: the denial
// error and the file, network, and capability checker interfaces. The
// declarative permission vocabulary (permission types, levels, sets,
// descriptors) is owned by userconfig/permissions, which decodes it from
// configuration; governance imports it directly as ucperms — the two-contract
// reality is spelled honestly, with no aliases.
package permissions

import (
	"context"

	"codeburg.org/lexbit/relurpify/governance/netpolicy"
	ucperms "codeburg.org/lexbit/relurpify/userconfig/permissions"
)

// PermissionDeniedError records a denied permission with context.
type PermissionDeniedError struct {
	Descriptor ucperms.PermissionDescriptor `json:"descriptor"`
	Message    string                       `json:"message"`
}

func (e *PermissionDeniedError) Error() string { return e.Message }

// FilePermissionChecker checks filesystem access.
type FilePermissionChecker interface {
	CheckFilePermission(ctx context.Context, agentID, basePath, action, absPath string, matrix any) error
}

// NetworkPermissionChecker checks network access against an already-resolved
// target. Callers resolve names first (ResolveTarget) so the pure decision
// layer never performs I/O; an unresolved name is a denial, not an allow.
type NetworkPermissionChecker interface {
	CheckNetwork(ctx context.Context, agentID, direction, protocol string, target netpolicy.Target, port int) error
	ResolveTarget(ctx context.Context, token string) (netpolicy.Target, error)
}

// CapabilityChecker checks capability access.
type CapabilityChecker interface {
	CheckCapability(ctx context.Context, agentID, capability string) error
}
