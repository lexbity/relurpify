package descriptor

import (
	"math"

	"codeburg.org/lexbit/relurpify/governance/classification"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// DescriptorViewAdapter wraps CapabilityDescriptor to implement
// governanceports.DescriptorView without field-name conflicts.
type DescriptorViewAdapter struct {
	D CapabilityDescriptor
}

var _ governanceports.DescriptorView = (*DescriptorViewAdapter)(nil)

func CapabilityDescriptorView(d CapabilityDescriptor) governanceports.DescriptorView {
	return &DescriptorViewAdapter{D: d}
}

func (a *DescriptorViewAdapter) CapabilityID() string          { return a.D.ID }
func (a *DescriptorViewAdapter) CapabilityName() string        { return a.D.Name }
func (a *DescriptorViewAdapter) CapabilityKind() string        { return string(a.D.Kind) }
func (a *DescriptorViewAdapter) RuntimeFamily() string         { return string(a.D.RuntimeFamily) }
func (a *DescriptorViewAdapter) Description() string           { return a.D.Description }
func (a *DescriptorViewAdapter) Version() string               { return a.D.Version }
func (a *DescriptorViewAdapter) Category() string              { return a.D.Category }
func (a *DescriptorViewAdapter) Tags() []string                { return a.D.Tags }
func (a *DescriptorViewAdapter) TrustClass() string            { return string(a.D.TrustClass) }
func (a *DescriptorViewAdapter) RiskClasses() []risk.RiskClass { return nil }
func (a *DescriptorViewAdapter) EffectClasses() []classification.EffectClass {
	return a.D.EffectClasses
}
func (a *DescriptorViewAdapter) SourceProviderID() string { return a.D.Source.ProviderID }
func (a *DescriptorViewAdapter) SourceScope() string      { return string(a.D.Source.Scope) }
func (a *DescriptorViewAdapter) SourceSessionID() string  { return a.D.Source.SessionID }

func (a *DescriptorViewAdapter) CoordinationRole() string {
	if a.D.Coordination != nil {
		return string(a.D.Coordination.Role)
	}
	return ""
}
func (a *DescriptorViewAdapter) CoordinationTarget() bool {
	return a.D.Coordination != nil && a.D.Coordination.Target
}
func (a *DescriptorViewAdapter) CoordinationTaskTypes() []string {
	if a.D.Coordination != nil {
		return a.D.Coordination.TaskTypes
	}
	return nil
}
func (a *DescriptorViewAdapter) CoordinationExecutionModes() []string {
	if a.D.Coordination != nil {
		out := make([]string, len(a.D.Coordination.ExecutionModes))
		for i, m := range a.D.Coordination.ExecutionModes {
			out[i] = string(m)
		}
		return out
	}
	return nil
}
func (a *DescriptorViewAdapter) CoordinationLongRunning() int32 {
	if a.D.Coordination != nil {
		return safeInt32View(int(a.D.Coordination.LongRunning))
	}
	return 0
}
func (a *DescriptorViewAdapter) CoordinationDirectInsertionAllowed() int32 {
	if a.D.Coordination != nil {
		return safeInt32View(int(a.D.Coordination.DirectInsertionAllowed))
	}
	return 0
}

func safeInt32View(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}
func (a *DescriptorViewAdapter) CoordinationMaxDepth() int {
	if a.D.Coordination != nil {
		return a.D.Coordination.MaxDepth
	}
	return 0
}
func (a *DescriptorViewAdapter) CoordinationMaxRuntimeSeconds() int {
	if a.D.Coordination != nil {
		return a.D.Coordination.MaxRuntimeSeconds
	}
	return 0
}
