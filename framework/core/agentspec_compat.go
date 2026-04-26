package core

import "codeburg.org/lexbit/relurpify/framework/agentspec"

type AgentRuntimeSpec = agentspec.AgentRuntimeSpec
type AgentModelConfig = agentspec.AgentModelConfig
type AgentModelConfigOverlay = agentspec.AgentModelConfigOverlay
type AgentSpecOverlay = agentspec.AgentSpecOverlay
type AgentBashPermissions = agentspec.AgentBashPermissions
type AgentFileMatrix = agentspec.AgentFileMatrix
type AgentInvocationSpec = agentspec.AgentInvocationSpec
type AgentCoordinationSpec = agentspec.AgentCoordinationSpec
type AgentProjectionPolicy = agentspec.AgentProjectionPolicy
type AgentProjectionTier = agentspec.AgentProjectionTier
type AgentScaleOutPolicy = agentspec.AgentScaleOutPolicy
type AgentArtifactWindowSpec = agentspec.AgentArtifactWindowSpec
type AgentBrowserSpec = agentspec.AgentBrowserSpec
type AgentBrowserExtractionSpec = agentspec.AgentBrowserExtractionSpec
type AgentBrowserDownloadSpec = agentspec.AgentBrowserDownloadSpec
type AgentBrowserCredentialsSpec = agentspec.AgentBrowserCredentialsSpec
type AgentSkillConfig = agentspec.AgentSkillConfig
type AgentVerificationPolicy = agentspec.AgentVerificationPolicy
type AgentRecoveryPolicy = agentspec.AgentRecoveryPolicy
type AgentPlanningPolicy = agentspec.AgentPlanningPolicy
type AgentReviewPolicy = agentspec.AgentReviewPolicy
type AgentSkillContextHints = agentspec.AgentSkillContextHints
type AgentMetadata = agentspec.AgentMetadata
type AgentLoggingSpec = agentspec.AgentLoggingSpec

// EffectiveCoordination exposes the agentspec coordination resolution helper at
// the legacy core boundary.
func EffectiveCoordination(spec *AgentRuntimeSpec) AgentCoordinationSpec {
	return agentspec.EffectiveCoordination(spec)
}

func AgentSpecOverlayFromSpec(spec *AgentRuntimeSpec) AgentSpecOverlay {
	return agentspec.AgentSpecOverlayFromSpec(spec)
}

func MergeAgentSpecs(base *AgentRuntimeSpec, overlays ...AgentSpecOverlay) *AgentRuntimeSpec {
	return agentspec.MergeAgentSpecs(base, overlays...)
}

func MergeAgentModelConfig(base AgentModelConfig, overlays ...AgentModelConfigOverlay) AgentModelConfig {
	return agentspec.MergeAgentModelConfig(base, overlays...)
}
