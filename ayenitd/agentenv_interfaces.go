package ayenitd

import "codeburg.org/lexbit/relurpify/framework/agentenv"

// Type aliases so that callers can reference ayenitd.VerificationPlanner etc.
// without importing framework/agentenv directly. These are the same types.
type (
	VerificationPlanner           = agentenv.VerificationPlanner
	VerificationPlan              = agentenv.VerificationPlan
	VerificationPlanRequest       = agentenv.VerificationPlanRequest
	VerificationCommand           = agentenv.VerificationCommand
	CompatibilitySurfaceExtractor = agentenv.CompatibilitySurfaceExtractor
	CompatibilitySurface          = agentenv.CompatibilitySurface
	CompatibilitySurfaceRequest   = agentenv.CompatibilitySurfaceRequest
)
