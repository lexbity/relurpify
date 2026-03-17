package euclo

import (
	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
)

// RegisterDefaultCodingCapabilities creates an EucloCapabilityRegistry
// populated with all built-in coding capabilities.
func RegisterDefaultCodingCapabilities(env agentenv.AgentEnvironment) *EucloCapabilityRegistry {
	reg := NewEucloCapabilityRegistry()
	_ = reg.Register(&editVerifyRepairCapability{env: env})
	_ = reg.Register(&reproduceLocalizePatchCapability{env: env})
	_ = reg.Register(&tddGenerateCapability{env: env})
	_ = reg.Register(&plannerPlanCapability{env: env})
	_ = reg.Register(&verifyChangeCapability{env: env})
	_ = reg.Register(&reportFinalCodingCapability{env: env})
	return reg
}

// BuildExecutionEnvelope constructs an ExecutionEnvelope from the agent's
// runtime state. This is used by the profile controller to pass context
// to coding capabilities.
func BuildExecutionEnvelope(
	task *core.Task,
	state *core.Context,
	mode ModeResolution,
	profile ExecutionProfileSelection,
	env agentenv.AgentEnvironment,
	workflowStore WorkflowArtifactWriter,
	workflowID, runID string,
	telemetry core.Telemetry,
) ExecutionEnvelope {
	return ExecutionEnvelope{
		Task:          task,
		Mode:          mode,
		Profile:       profile,
		Registry:      env.Registry,
		State:         state,
		Memory:        env.Memory,
		Environment:   env,
		WorkflowStore: workflowStore,
		WorkflowID:    workflowID,
		RunID:         runID,
		Telemetry:     telemetry,
	}
}
