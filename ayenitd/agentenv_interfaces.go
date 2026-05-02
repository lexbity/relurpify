package ayenitd

import (
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// Type aliases so that callers can reference ayenitd.VerificationPlanner etc.
// without importing framework/agentenv or platform/contracts directly.
type (
	VerificationPlanner           = agentenv.VerificationPlanner
	VerificationPlan              = contracts.VerificationPlan
	VerificationPlanRequest       = contracts.VerificationPlanRequest
	VerificationCommand           = contracts.VerificationCommand
	CompatibilitySurfaceExtractor = agentenv.CompatibilitySurfaceExtractor
	CompatibilitySurface          = contracts.CompatibilitySurface
	CompatibilitySurfaceRequest   = contracts.CompatibilitySurfaceRequest
	WorkspaceEnvironment          = agentenv.WorkspaceEnvironment
)
