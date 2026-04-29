package capabilities

// Built-in capability families for Euclo.
const (
	FamilyCodeUnderstanding      = "code_understanding"
	FamilyRefactorPatch          = "refactor_patch"
	FamilyVerification           = "verification"
	FamilyRegressionLocalization = "regression_localization"
	FamilyMigrationCompat        = "migration_compat"
	FamilyReviewSynthesis        = "review_synthesis"
	FamilyArchitecture           = "architecture"
)

// GetBuiltinFamilies returns the built-in capability families.
func GetBuiltinFamilies() []CapabilityFamily {
	return []CapabilityFamily{
		{
			ID:                 FamilyCodeUnderstanding,
			Name:               "Code Understanding",
			Description:        "Capabilities for investigating and understanding code",
			FallbackCapability: "euclo:cap.ast_query",
			CapabilityIDs: []string{
				"euclo:cap.ast_query",
				"euclo:cap.symbol_trace",
				"euclo:cap.call_graph",
			},
		},
		{
			ID:                 FamilyRefactorPatch,
			Name:               "Refactor & Patch",
			Description:        "Capabilities for refactoring and patching code",
			FallbackCapability: "euclo:cap.targeted_refactor",
			CapabilityIDs: []string{
				"euclo:cap.targeted_refactor",
				"euclo:cap.rename_symbol",
				"euclo:cap.extract_func",
			},
		},
		{
			ID:                 FamilyVerification,
			Name:               "Verification",
			Description:        "Capabilities for testing and verification",
			FallbackCapability: "euclo:cap.test_run",
			CapabilityIDs: []string{
				"euclo:cap.test_synthesis",
				"euclo:cap.test_run",
				"euclo:cap.coverage_check",
			},
		},
		{
			ID:                 FamilyRegressionLocalization,
			Name:               "Regression Localization",
			Description:        "Capabilities for localizing regressions",
			FallbackCapability: "euclo:cap.bisect",
			CapabilityIDs: []string{
				"euclo:cap.bisect",
				"euclo:cap.blame_trace",
			},
		},
		{
			ID:                 FamilyMigrationCompat,
			Name:               "Migration Compatibility",
			Description:        "Capabilities for migration and compatibility checks",
			FallbackCapability: "euclo:cap.api_compat",
			CapabilityIDs: []string{
				"euclo:cap.api_compat",
				"euclo:cap.dep_upgrade",
			},
		},
		{
			ID:                 FamilyReviewSynthesis,
			Name:               "Review Synthesis",
			Description:        "Capabilities for code review and synthesis",
			FallbackCapability: "euclo:cap.code_review",
			CapabilityIDs: []string{
				"euclo:cap.code_review",
				"euclo:cap.diff_summary",
			},
		},
		{
			ID:                 FamilyArchitecture,
			Name:               "Architecture",
			Description:        "Capabilities for architecture analysis",
			FallbackCapability: "euclo:cap.layer_check",
			CapabilityIDs: []string{
				"euclo:cap.layer_check",
				"euclo:cap.boundary_report",
			},
		},
	}
}
