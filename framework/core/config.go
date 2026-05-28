package core

import "codeburg.org/lexbit/relurpify/framework/agentspec"

// Config is the agent runtime configuration surface used by the framework.
type Config struct {
	Name              string                      `json:"name,omitempty"`
	Model             string                      `json:"model,omitempty"`
	MaxIterations     int                         `json:"max_iterations,omitempty"`
	NativeToolCalling bool                        `json:"native_tool_calling,omitempty"`
	Telemetry         Telemetry                   `json:"-"`
	AgentSpec         *agentspec.AgentRuntimeSpec `json:"agent_spec,omitempty"`
	Workspace         string                      `json:"workspace,omitempty"`
	AgentsDir         string                      `json:"agents_dir,omitempty"`
	ManifestPath      string                      `json:"manifest_path,omitempty"`
	ConfigPath        string                      `json:"config_path,omitempty"`
	InferenceModel    string                      `json:"inference_model,omitempty"`
	InferenceProvider string                      `json:"inference_provider,omitempty"`
	DebugLLM          bool                        `json:"debug_llm,omitempty"`
	DebugAgent        bool                        `json:"debug_agent,omitempty"`
	RecordingMode     string                      `json:"recording_mode,omitempty"`
	SandboxBackend    string                      `json:"sandbox_backend,omitempty"`
	Extensions        map[string]any              `json:"extensions,omitempty"`
	Metadata          map[string]any              `json:"metadata,omitempty"`
}
