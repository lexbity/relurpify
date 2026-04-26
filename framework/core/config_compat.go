package core

import "codeburg.org/lexbit/relurpify/framework/agentspec"

// Capability enumerates the agent capability surface used throughout the
// framework.
type Capability string

const (
	CapabilityPlan        Capability = "plan"
	CapabilityExecute     Capability = "execute"
	CapabilityCode        Capability = "code"
	CapabilityExplain     Capability = "explain"
	CapabilityReview      Capability = "review"
	CapabilityHumanInLoop Capability = "human-in-loop"
)

// Config is the agent runtime configuration surface used by the framework.
type Config struct {
	Name              string                      `json:"name,omitempty" yaml:"name,omitempty"`
	Model             string                      `json:"model,omitempty" yaml:"model,omitempty"`
	MaxIterations     int                         `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
	NativeToolCalling bool                        `json:"native_tool_calling,omitempty" yaml:"native_tool_calling,omitempty"`
	Telemetry         Telemetry                   `json:"-" yaml:"-"`
	AgentSpec         *agentspec.AgentRuntimeSpec `json:"agent_spec,omitempty" yaml:"agent_spec,omitempty"`
	Workspace         string                      `json:"workspace,omitempty" yaml:"workspace,omitempty"`
	AgentsDir         string                      `json:"agents_dir,omitempty" yaml:"agents_dir,omitempty"`
	ManifestPath      string                      `json:"manifest_path,omitempty" yaml:"manifest_path,omitempty"`
	ConfigPath        string                      `json:"config_path,omitempty" yaml:"config_path,omitempty"`
	InferenceModel    string                      `json:"inference_model,omitempty" yaml:"inference_model,omitempty"`
	InferenceProvider string                      `json:"inference_provider,omitempty" yaml:"inference_provider,omitempty"`
	DebugLLM          bool                        `json:"debug_llm,omitempty" yaml:"debug_llm,omitempty"`
	DebugAgent        bool                        `json:"debug_agent,omitempty" yaml:"debug_agent,omitempty"`
	RecordingMode     string                      `json:"recording_mode,omitempty" yaml:"recording_mode,omitempty"`
	SandboxBackend    string                      `json:"sandbox_backend,omitempty" yaml:"sandbox_backend,omitempty"`
	Extensions        map[string]any              `json:"extensions,omitempty" yaml:"extensions,omitempty"`
	Metadata          map[string]any              `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}
