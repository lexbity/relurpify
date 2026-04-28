package contracts

// CompatibilitySurface represents the extracted compatibility surface for a workspace.
type CompatibilitySurface struct {
	Functions []map[string]any
	Types     []map[string]any
	Metadata  map[string]any
}

// CompatibilitySurfaceRequest captures the inputs for a surface extraction operation.
type CompatibilitySurfaceRequest struct {
	TaskInstruction string
	Workspace       string
	Files           []string
	FileContents    []map[string]any
}

// VerificationPlanRequest captures the inputs for a verification planning operation.
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

// VerificationCommand represents a single verification command to execute.
type VerificationCommand struct {
	Command          []string
	Dir              string
	Name             string
	Args             []string
	WorkingDirectory string
}

// VerificationPlan represents a complete verification plan with rationale.
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
