package families

// Built-in keyword families for Euclo.
// These are registered by default when BuiltinFamilies is enabled.

const (
	FamilyDebug          = "debug"
	FamilyRepair         = "repair"
	FamilyReview         = "review"
	FamilyPlanning       = "planning"
	FamilyImplementation = "implementation"
	FamilyRefactor       = "refactor"
	FamilyMigration      = "migration"
	FamilyInvestigation  = "investigation"
	FamilyArchitecture   = "architecture"
)

// RegisterBuiltins registers all built-in keyword families into the registry.
func RegisterBuiltins(registry *KeywordFamilyRegistry) error {
	families := []KeywordFamily{
		{
			ID:                  FamilyDebug,
			DisplayName:         "Debug",
			Keywords:            []string{"fix", "diagnose", "trace", "panic", "error", "broken", "crash", "exception"},
			IntentKeywords:      []string{"failing test", "panic", "error", "broken", "crash"},
			DefaultHITLPolicy:   HITLPolicyAsk,
			DefaultVerification: VerificationRequired,
			RetrievalTemplate:   "error context and related code for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.bisect",
			CapabilitySequence:  []string{"euclo:cap.bisect", "euclo:cap.symbol_trace"},
		},
		{
			ID:                  FamilyRepair,
			DisplayName:         "Repair",
			Keywords:            []string{"patch", "correct", "resolve", "hotfix", "fix bug"},
			IntentKeywords:      []string{"hotfix", "patch", "quick fix"},
			DefaultHITLPolicy:   HITLPolicyAsk,
			DefaultVerification: VerificationRequired,
			RetrievalTemplate:   "bug context and related code for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.targeted_refactor",
			CapabilitySequence:  []string{"euclo:cap.targeted_refactor"},
		},
		{
			ID:                  FamilyReview,
			DisplayName:         "Review",
			Keywords:            []string{"review", "audit", "check", "assess", "analyze", "inspect"},
			IntentKeywords:      []string{"code review", "audit", "assessment"},
			DefaultHITLPolicy:   HITLPolicyNever,
			DefaultVerification: VerificationNotRequired,
			RetrievalTemplate:   "code to review for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.code_review",
			CapabilitySequence:  []string{"euclo:cap.code_review"},
		},
		{
			ID:                  FamilyPlanning,
			DisplayName:         "Planning",
			Keywords:            []string{"plan", "design", "scaffold", "outline", "architect"},
			IntentKeywords:      []string{"design document", "architecture", "scaffold"},
			DefaultHITLPolicy:   HITLPolicyAsk,
			DefaultVerification: VerificationNotRequired,
			RetrievalTemplate:   "project context for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.layer_check",
			CapabilitySequence:  []string{"euclo:cap.layer_check"},
		},
		{
			ID:                  FamilyImplementation,
			DisplayName:         "Implementation",
			Keywords:            []string{"implement", "add", "build", "create", "write", "develop"},
			IntentKeywords:      []string{"implement", "build", "create", "develop"},
			DefaultHITLPolicy:   HITLPolicyAsk,
			DefaultVerification: VerificationConfigurable,
			RetrievalTemplate:   "implementation context for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.targeted_refactor",
			CapabilitySequence:  []string{"euclo:cap.targeted_refactor"},
		},
		{
			ID:                  FamilyRefactor,
			DisplayName:         "Refactor",
			Keywords:            []string{"refactor", "restructure", "extract", "rename", "move", "reorganize"},
			IntentKeywords:      []string{"refactor", "restructure", "extract", "rename"},
			DefaultHITLPolicy:   HITLPolicyAsk,
			DefaultVerification: VerificationConfigurable,
			RetrievalTemplate:   "code to refactor for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.extract_func",
			CapabilitySequence:  []string{"euclo:cap.extract_func", "euclo:cap.rename"},
		},
		{
			ID:                  FamilyMigration,
			DisplayName:         "Migration",
			Keywords:            []string{"migrate", "upgrade", "port", "update dependency", "upgrade dep"},
			IntentKeywords:      []string{"migration", "upgrade", "port", "dependency upgrade"},
			DefaultHITLPolicy:   HITLPolicyAsk,
			DefaultVerification: VerificationRequired,
			RetrievalTemplate:   "migration context for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.api_compat",
			CapabilitySequence:  []string{"euclo:cap.api_compat"},
		},
		{
			ID:                  FamilyInvestigation,
			DisplayName:         "Investigation",
			Keywords:            []string{"investigate", "explore", "understand", "trace", "analyze"},
			IntentKeywords:      []string{"investigate", "explore", "understand", "trace"},
			DefaultHITLPolicy:   HITLPolicyNever,
			DefaultVerification: VerificationNotRequired,
			RetrievalTemplate:   "investigation context for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.symbol_trace",
			CapabilitySequence:  []string{"euclo:cap.symbol_trace"},
		},
		{
			ID:                  FamilyArchitecture,
			DisplayName:         "Architecture",
			Keywords:            []string{"architecture", "decompose", "layer", "boundary", "structure"},
			IntentKeywords:      []string{"architecture", "decompose", "layer", "boundary"},
			DefaultHITLPolicy:   HITLPolicyAsk,
			DefaultVerification: VerificationNotRequired,
			RetrievalTemplate:   "architecture context for: {{.Instruction}}",
			FallbackCapability:  "euclo:cap.boundary_report",
			CapabilitySequence:  []string{"euclo:cap.boundary_report"},
		},
	}

	for _, family := range families {
		if err := registry.Register(family); err != nil {
			return err
		}
	}

	return nil
}
