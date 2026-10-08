package subprocess

import "codeburg.org/lexbit/relurpify/capability/ports"

type builder struct {
	approver                 PrivateEgressApprover
	networkIsolationDisabled bool
}

func (b builder) BuildTool(manifest ports.ToolManifest, runner ports.CommandRunner) (ports.Tool, error) {
	return NewToolWithEgress(manifest, runner, b.approver, b.networkIsolationDisabled), nil
}

// BackendBuilder builds subprocess tools without an egress approver. Private
// egress is denied (fail closed) when no approver is wired.
func BackendBuilder() ports.ToolBackendBuilder { return builder{} }

// BackendBuilderWithEgress builds subprocess tools with the private-egress
// approval port and the effective container isolation. The composition root
// supplies both.
func BackendBuilderWithEgress(approver PrivateEgressApprover, networkIsolationDisabled bool) ports.ToolBackendBuilder {
	return builder{approver: approver, networkIsolationDisabled: networkIsolationDisabled}
}
