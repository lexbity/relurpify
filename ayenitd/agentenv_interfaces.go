package ayenitd

import (
	"context"
)

// VerificationPlanner is an interface for agents that verify plans.
// This matches the interface from framework/agentenv.
type VerificationPlanner interface {
	SelectVerificationPlan(ctx context.Context, req VerificationPlanRequest) (VerificationPlan, bool, error)
}

// VerificationPlanRequest contains the details needed to request a verification.
type VerificationPlanRequest struct {
	TaskInstruction                 string
	ModeID                          string
	ProfileID                       string
	Workspace                       string
	Files                           []string
	TestFiles                       []string
	PublicSurfaceChanged            bool
	PreferredVerifyCapabilities     []string
	VerificationSuccessCapabilities []string
	RequireVerificationStep         bool
}

// VerificationPlan represents a verification plan.
type VerificationPlan struct {
	ScopeKind              string
	Files                  []string
	TestFiles              []string
	Commands               []VerificationCommand
	Source                 string
	PlannerID              string
	Rationale              string
	AuditTrail             []string
	CompatibilitySensitive bool
	Metadata               map[string]any
}

// VerificationCommand represents a command to run for verification.
type VerificationCommand struct {
	Name             string
	Command          string
	Args             []string
	WorkingDirectory string
}

// CompatibilitySurfaceExtractor is an interface for agents that extract compatibility surfaces.
// This matches the interface from framework/agentenv.
type CompatibilitySurfaceExtractor interface {
	ExtractSurface(ctx context.Context, req CompatibilitySurfaceRequest) (CompatibilitySurface, bool, error)
}

// CompatibilitySurfaceRequest contains the details needed to request a compatibility surface.
type CompatibilitySurfaceRequest struct {
	TaskInstruction string
	Workspace       string
	Files           []string
	FileContents    []map[string]any
}

// CompatibilitySurface represents a compatibility surface.
type CompatibilitySurface struct {
	Functions []map[string]any
	Types     []map[string]any
	Metadata  map[string]any
}
