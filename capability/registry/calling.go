package registry

import (
	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/model"
)

// CapabilityCallingMode determines whether a model invocation should use a
// provider-native tool-call API or the framework-owned render/parse fallback.
type CapabilityCallingMode string

const (
	CapabilityCallingNative   CapabilityCallingMode = "native"
	CapabilityCallingFallback CapabilityCallingMode = "fallback"
)

// ResolveCallingMode selects the capability-calling path for a request.
//
// Native tool calling is only used when the agent spec enables it and the
// backend reports native tool-call support. Everything else falls back to the
// framework-rendered tool prompt and parser.
func ResolveCallingMode(spec *agentspec.AgentRuntimeSpec, caps model.BackendCapabilities) CapabilityCallingMode {
	if spec != nil && spec.NativeToolCallingEnabled() && caps.NativeToolCalling {
		return CapabilityCallingNative
	}
	return CapabilityCallingFallback
}
