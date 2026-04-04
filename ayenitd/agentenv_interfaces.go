package ayenitd

// VerificationPlanner is an interface for agents that verify plans.
// This is a copy of the interface from framework/agentenv to avoid import cycles.
type VerificationPlanner interface {
	// PlanVerification requests verification of a plan.
	PlanVerification(req VerificationPlanRequest) (*VerificationPlan, error)
}

// VerificationPlanRequest contains the details needed to request a verification.
type VerificationPlanRequest struct {
	// Fields placeholder
}

// VerificationPlan represents a verification plan.
type VerificationPlan struct {
	// Fields placeholder
}

// CompatibilitySurfaceExtractor is an interface for agents that extract compatibility surfaces.
// This is a copy of the interface from framework/agentenv to avoid import cycles.
type CompatibilitySurfaceExtractor interface {
	// ExtractCompatibilitySurface extracts compatibility surface from a request.
	ExtractCompatibilitySurface(req CompatibilitySurfaceRequest) (*CompatibilitySurface, error)
}

// CompatibilitySurfaceRequest contains the details needed to request a compatibility surface.
type CompatibilitySurfaceRequest struct {
	// Fields placeholder
}

// CompatibilitySurface represents a compatibility surface.
type CompatibilitySurface struct {
	// Fields placeholder
}
