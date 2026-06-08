package capability

import (
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
)

// descriptorViewAdapter wraps CapabilityDescriptor to implement
// governanceports.DescriptorView without field-name conflicts.
type descriptorViewAdapter struct {
	d CapabilityDescriptor
}

var _ governanceports.DescriptorView = (*descriptorViewAdapter)(nil)

func CapabilityDescriptorView(d CapabilityDescriptor) governanceports.DescriptorView {
	return &descriptorViewAdapter{d: d}
}

func (a *descriptorViewAdapter) CapabilityID() string    { return a.d.ID }
func (a *descriptorViewAdapter) CapabilityName() string  { return a.d.Name }
func (a *descriptorViewAdapter) CapabilityKind() string  { return string(a.d.Kind) }
func (a *descriptorViewAdapter) RuntimeFamily() string   { return string(a.d.RuntimeFamily) }
func (a *descriptorViewAdapter) Description() string     { return a.d.Description }
func (a *descriptorViewAdapter) Version() string         { return a.d.Version }
func (a *descriptorViewAdapter) Category() string        { return a.d.Category }
func (a *descriptorViewAdapter) Tags() []string          { return a.d.Tags }
func (a *descriptorViewAdapter) TrustClass() string      { return string(a.d.TrustClass) }
func (a *descriptorViewAdapter) RiskClasses() []taxonomy.RiskClass   { return a.d.RiskClasses }
func (a *descriptorViewAdapter) EffectClasses() []taxonomy.EffectClass { return a.d.EffectClasses }
func (a *descriptorViewAdapter) SourceProviderID() string { return a.d.Source.ProviderID }
func (a *descriptorViewAdapter) SourceScope() string     { return string(a.d.Source.Scope) }
func (a *descriptorViewAdapter) SourceSessionID() string { return a.d.Source.SessionID }

func (a *descriptorViewAdapter) CoordinationRole() string {
	if a.d.Coordination != nil {
		return string(a.d.Coordination.Role)
	}
	return ""
}
func (a *descriptorViewAdapter) CoordinationTarget() bool {
	return a.d.Coordination != nil && a.d.Coordination.Target
}
func (a *descriptorViewAdapter) CoordinationTaskTypes() []string {
	if a.d.Coordination != nil {
		return a.d.Coordination.TaskTypes
	}
	return nil
}
func (a *descriptorViewAdapter) CoordinationExecutionModes() []string {
	if a.d.Coordination != nil {
		out := make([]string, len(a.d.Coordination.ExecutionModes))
		for i, m := range a.d.Coordination.ExecutionModes {
			out[i] = string(m)
		}
		return out
	}
	return nil
}
func (a *descriptorViewAdapter) CoordinationLongRunning() int32 {
	if a.d.Coordination != nil {
		return int32(a.d.Coordination.LongRunning)
	}
	return 0
}
func (a *descriptorViewAdapter) CoordinationDirectInsertionAllowed() int32 {
	if a.d.Coordination != nil {
		return int32(a.d.Coordination.DirectInsertionAllowed)
	}
	return 0
}
func (a *descriptorViewAdapter) CoordinationMaxDepth() int {
	if a.d.Coordination != nil {
		return a.d.Coordination.MaxDepth
	}
	return 0
}
func (a *descriptorViewAdapter) CoordinationMaxRuntimeSeconds() int {
	if a.d.Coordination != nil {
		return a.d.Coordination.MaxRuntimeSeconds
	}
	return 0
}
