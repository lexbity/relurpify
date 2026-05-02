package capability

import (
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type Registry = CapabilityRegistry
type Descriptor = core.CapabilityDescriptor
type Kind = core.CapabilityKind
type RuntimeFamily = core.CapabilityRuntimeFamily
type Source = core.CapabilitySource
type Exposure = agentspec.CapabilityExposure
type Selector = agentspec.CapabilitySelector
type Policy = agentspec.CapabilityPolicy
type ExposurePolicy = agentspec.CapabilityExposurePolicy
type ExecutionResult = contracts.CapabilityExecutionResult
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
	ExposureHidden      = agentspec.CapabilityExposureHidden
	ExposureInspectable = agentspec.CapabilityExposureInspectable
	ExposureCallable    = agentspec.CapabilityExposureCallable
)

func NewRegistry() *Registry {
	return NewCapabilityRegistry()
}
