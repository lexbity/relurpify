package policy

import (
	"testing"

	"codeburg.org/lexbit/relurpify/governance/classification"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// fakeDescriptor implements governanceports.DescriptorView for testing.
type fakeDescriptor struct {
	id         string
	name       string
	kind       string
	runtimeFam string
	trustClass string
}

func (f *fakeDescriptor) CapabilityID() string                        { return f.id }
func (f *fakeDescriptor) CapabilityName() string                      { return f.name }
func (f *fakeDescriptor) CapabilityKind() string                      { return f.kind }
func (f *fakeDescriptor) RuntimeFamily() string                       { return f.runtimeFam }
func (f *fakeDescriptor) Description() string                         { return "" }
func (f *fakeDescriptor) Version() string                             { return "" }
func (f *fakeDescriptor) Category() string                            { return "" }
func (f *fakeDescriptor) Tags() []string                              { return nil }
func (f *fakeDescriptor) TrustClass() string                          { return f.trustClass }
func (f *fakeDescriptor) RiskClasses() []risk.RiskClass               { return nil }
func (f *fakeDescriptor) EffectClasses() []classification.EffectClass { return nil }
func (f *fakeDescriptor) SourceProviderID() string                    { return "" }
func (f *fakeDescriptor) SourceScope() string                         { return "" }
func (f *fakeDescriptor) SourceSessionID() string                     { return "" }
func (f *fakeDescriptor) CoordinationRole() string                    { return "" }
func (f *fakeDescriptor) CoordinationTarget() bool                    { return false }
func (f *fakeDescriptor) CoordinationTaskTypes() []string             { return nil }
func (f *fakeDescriptor) CoordinationExecutionModes() []string        { return nil }
func (f *fakeDescriptor) CoordinationLongRunning() int32              { return 0 }
func (f *fakeDescriptor) CoordinationDirectInsertionAllowed() int32   { return 0 }
func (f *fakeDescriptor) CoordinationMaxDepth() int                   { return 0 }
func (f *fakeDescriptor) CoordinationMaxRuntimeSeconds() int          { return 0 }

func TestDescriptorView_InterfaceSatisfied(t *testing.T) {
	var _ governanceports.DescriptorView = (*fakeDescriptor)(nil)
}

func TestDescriptorView_FieldAccess(t *testing.T) {
	d := &fakeDescriptor{
		id:         "cap-1",
		name:       "test-cap",
		kind:       "tool",
		runtimeFam: "local-tool",
		trustClass: "builtin-trusted",
	}

	if d.CapabilityID() != "cap-1" {
		t.Errorf("CapabilityID() = %q, want %q", d.CapabilityID(), "cap-1")
	}
	if d.CapabilityKind() != "tool" {
		t.Errorf("CapabilityKind() = %q, want %q", d.CapabilityKind(), "tool")
	}
	if d.RuntimeFamily() != "local-tool" {
		t.Errorf("RuntimeFamily() = %q, want %q", d.RuntimeFamily(), "local-tool")
	}
	if d.TrustClass() != "builtin-trusted" {
		t.Errorf("TrustClass() = %q, want %q", d.TrustClass(), "builtin-trusted")
	}
}
