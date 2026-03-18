package capabilities

import (
	"github.com/lexcodex/relurpify/framework/agentenv"
)

// NewDefaultCapabilityRegistry creates an EucloCapabilityRegistry
// populated with all built-in coding capabilities.
func NewDefaultCapabilityRegistry(env agentenv.AgentEnvironment) *EucloCapabilityRegistry {
	reg := NewEucloCapabilityRegistry()
	_ = reg.Register(&editVerifyRepairCapability{env: env})
	_ = reg.Register(&reproduceLocalizePatchCapability{env: env})
	_ = reg.Register(&tddGenerateCapability{env: env})
	_ = reg.Register(&plannerPlanCapability{env: env})
	_ = reg.Register(&verifyChangeCapability{env: env})
	_ = reg.Register(&reportFinalCodingCapability{env: env})
	return reg
}
