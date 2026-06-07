package capability

import agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"

func (d CapabilityDescriptor) CapabilityID() string                       { return d.ID }
func (d CapabilityDescriptor) CapabilityName() string                     { return d.Name }
func (d CapabilityDescriptor) CapabilityTrustClass() agentspec.TrustClass { return d.TrustClass }
func (d CapabilityDescriptor) CoordinationRole() agentspec.CoordinationRole {
	return coordinationRole(d)
}
func (d CapabilityDescriptor) CoordinationTarget() bool            { return coordinationTarget(d) }
func (d CapabilityDescriptor) LongRunning() agentspec.EnabledState { return longRunning(d) }
func (d CapabilityDescriptor) CapabilityRuntimeFamily() agentspec.CapabilityRuntimeFamily {
	return d.RuntimeFamily
}
func (d CapabilityDescriptor) SourceScope() agentspec.CapabilityScope { return scope(d) }
func (d CapabilityDescriptor) SourceProviderID() string               { return sourceProviderID(d) }
func (d CapabilityDescriptor) SourceSessionID() string                { return sourceSessionID(d) }

func (d CapabilityDescriptor) CoordinationTaskTypes() []string { return coordinationTaskTypes(d) }
func (d CapabilityDescriptor) CoordinationExecutionModes() []agentspec.CoordinationExecutionMode {
	return coordinationExecutionModes(d)
}
func (d CapabilityDescriptor) DirectInsertionAllowed() agentspec.EnabledState {
	return directInsertionAllowed(d)
}

func coordinationRole(d CapabilityDescriptor) agentspec.CoordinationRole {
	if d.Coordination != nil {
		return d.Coordination.Role
	}
	return ""
}

func coordinationTarget(d CapabilityDescriptor) bool {
	return d.Coordination != nil && d.Coordination.Target
}

func longRunning(d CapabilityDescriptor) agentspec.EnabledState {
	if d.Coordination != nil {
		return agentspec.EnabledState(d.Coordination.LongRunning)
	}
	return agentspec.EnabledStateUnset
}

func scope(d CapabilityDescriptor) agentspec.CapabilityScope { return d.Source.Scope }
func sourceProviderID(d CapabilityDescriptor) string         { return d.Source.ProviderID }
func sourceSessionID(d CapabilityDescriptor) string          { return d.Source.SessionID }

func coordinationTaskTypes(d CapabilityDescriptor) []string {
	if d.Coordination != nil {
		return d.Coordination.TaskTypes
	}
	return nil
}

func coordinationExecutionModes(d CapabilityDescriptor) []agentspec.CoordinationExecutionMode {
	if d.Coordination != nil {
		return d.Coordination.ExecutionModes
	}
	return nil
}

func directInsertionAllowed(d CapabilityDescriptor) agentspec.EnabledState {
	if d.Coordination != nil {
		return agentspec.EnabledState(d.Coordination.DirectInsertionAllowed)
	}
	return agentspec.EnabledStateUnset
}
