package capability

import "github.com/lexcodex/relurpify/framework/core"

type Registry = CapabilityRegistry
type Descriptor = core.CapabilityDescriptor
type Kind = core.CapabilityKind
type RuntimeFamily = core.CapabilityRuntimeFamily
type Source = core.CapabilitySource
type Exposure = core.CapabilityExposure
type Selector = core.CapabilitySelector
type Policy = core.CapabilityPolicy
type ExposurePolicy = core.CapabilityExposurePolicy
type ExecutionResult = core.CapabilityExecutionResult
type PromptRenderResult = core.PromptRenderResult
type ResourceReadResult = core.ResourceReadResult
type Registrar = core.CapabilityRegistrar

const (
	KindTool     = core.CapabilityKindTool
	KindPrompt   = core.CapabilityKindPrompt
	KindResource = core.CapabilityKindResource
)

const (
	RuntimeFamilyLocalTool = core.CapabilityRuntimeFamilyLocalTool
	RuntimeFamilyProvider  = core.CapabilityRuntimeFamilyProvider
	RuntimeFamilyRelurpic  = core.CapabilityRuntimeFamilyRelurpic
)

const (
	ExposureHidden      = core.CapabilityExposureHidden
	ExposureInspectable = core.CapabilityExposureInspectable
	ExposureCallable    = core.CapabilityExposureCallable
)

func NewRegistry() *Registry {
	return NewCapabilityRegistry()
}
