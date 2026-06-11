package descriptor

import (
	"math"

	"codeburg.org/lexbit/relurpify/governance/classification"
)

func (d CapabilityDescriptor) CapabilityID() string         { return d.ID }
func (d CapabilityDescriptor) CapabilityName() string       { return d.Name }
func (d CapabilityDescriptor) CapabilityTrustClass() string { return string(d.TrustClass) }
func (d CapabilityDescriptor) CoordinationRole() string {
	return coordinationRole(d)
}
func (d CapabilityDescriptor) CoordinationTarget() bool { return coordinationTarget(d) }
func (d CapabilityDescriptor) LongRunning() int32       { return longRunning(d) }
func (d CapabilityDescriptor) CapabilityRuntimeFamily() string {
	return string(d.RuntimeFamily)
}
func (d CapabilityDescriptor) SourceScope() classification.CapabilityScope { return scope(d) }
func (d CapabilityDescriptor) SourceProviderID() string                    { return sourceProviderID(d) }
func (d CapabilityDescriptor) SourceSessionID() string                     { return sourceSessionID(d) }

func (d CapabilityDescriptor) CoordinationTaskTypes() []string { return coordinationTaskTypes(d) }
func (d CapabilityDescriptor) CoordinationExecutionModes() []string {
	return coordinationExecutionModes(d)
}
func (d CapabilityDescriptor) DirectInsertionAllowed() int32 {
	return directInsertionAllowed(d)
}

func safeInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

func coordinationRole(d CapabilityDescriptor) string {
	if d.Coordination != nil {
		return string(d.Coordination.Role)
	}
	return ""
}

func coordinationTarget(d CapabilityDescriptor) bool {
	return d.Coordination != nil && d.Coordination.Target
}

func longRunning(d CapabilityDescriptor) int32 {
	if d.Coordination != nil {
		return safeInt32(int(d.Coordination.LongRunning))
	}
	return 0
}

func scope(d CapabilityDescriptor) classification.CapabilityScope { return d.Source.Scope }
func sourceProviderID(d CapabilityDescriptor) string              { return d.Source.ProviderID }
func sourceSessionID(d CapabilityDescriptor) string               { return d.Source.SessionID }

func coordinationTaskTypes(d CapabilityDescriptor) []string {
	if d.Coordination != nil {
		return d.Coordination.TaskTypes
	}
	return nil
}

func coordinationExecutionModes(d CapabilityDescriptor) []string {
	if d.Coordination != nil {
		out := make([]string, len(d.Coordination.ExecutionModes))
		for i, m := range d.Coordination.ExecutionModes {
			out[i] = string(m)
		}
		return out
	}
	return nil
}

func directInsertionAllowed(d CapabilityDescriptor) int32 {
	if d.Coordination != nil {
		return safeInt32(int(d.Coordination.DirectInsertionAllowed))
	}
	return 0
}
