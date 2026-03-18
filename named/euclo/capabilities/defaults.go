package capabilities

import (
	"github.com/lexcodex/relurpify/framework/agentenv"
)

// NewDefaultCapabilityRegistry creates an EucloCapabilityRegistry
// populated with all built-in coding capabilities.
func NewDefaultCapabilityRegistry(env agentenv.AgentEnvironment) *EucloCapabilityRegistry {
	reg := NewEucloCapabilityRegistry()
	_ = reg.Register(&investigateRegressionCapability{env: env})
	_ = reg.Register(&traceAnalyzeCapability{env: env})
	_ = reg.Register(&designAlternativesCapability{env: env})
	_ = reg.Register(&executionProfileSelectCapability{env: env})
	_ = reg.Register(&diffSummaryCapability{env: env})
	_ = reg.Register(&traceToRootCauseCapability{env: env})
	_ = reg.Register(&verificationSummaryCapability{env: env})
	_ = reg.Register(&migrationExecuteCapability{env: env})
	_ = reg.Register(&reviewFindingsCapability{env: env})
	_ = reg.Register(&reviewCompatibilityCapability{env: env})
	_ = reg.Register(&reviewImplementIfSafeCapability{env: env})
	_ = reg.Register(&refactorAPICompatibleCapability{env: env})
	_ = reg.Register(&editVerifyRepairCapability{env: env})
	_ = reg.Register(&reproduceLocalizePatchCapability{env: env})
	_ = reg.Register(&tddGenerateCapability{env: env})
	_ = reg.Register(&plannerPlanCapability{env: env})
	_ = reg.Register(&verifyChangeCapability{env: env})
	_ = reg.Register(&reportFinalCodingCapability{env: env})
	return reg
}
