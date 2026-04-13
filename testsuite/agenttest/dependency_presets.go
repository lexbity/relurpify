package agenttest

// Preset dependency rules for common tool call patterns.
// These can be used directly in test suites or as templates for custom rules.

// PresetCodeEditDependencies defines dependencies for code editing workflows.
// This ensures safe file operations where reads precede writes,
// and tests are run after modifications.
var PresetCodeEditDependencies = []ToolDependency{
	// File operations: must read before writing
	{Tool: "file_write", Requires: []string{"file_read"}},
	{Tool: "file_edit", Requires: []string{"file_read"}},

	// Compilation: must have written changes first
	{Tool: "go_build", Requires: []string{"file_write", "file_edit"}},

	// Testing: must have built or written changes
	{Tool: "go_test", Requires: []string{"file_write", "file_edit", "go_build"}},

	// Version control: avoid conflicts
	{Tool: "file_write", Excludes: []string{"git_commit"}},
	{Tool: "git_commit", Excludes: []string{"file_write", "file_edit"}},
}

// PresetAnalysisDependencies defines dependencies for code analysis workflows.
// This ensures files are listed/searched before detailed analysis.
var PresetAnalysisDependencies = []ToolDependency{
	// Search operations: list files first to understand structure
	{Tool: "file_search", Requires: []string{"file_list"}},

	// Semantic search: read files before analyzing content
	{Tool: "search_semantic", Requires: []string{"file_read"}},

	// AST operations: search for relevant files first
	{Tool: "ast_analyze", Requires: []string{"file_search"}},

	// Index operations: analyze before indexing
	{Tool: "index_update", Requires: []string{"ast_analyze"}},
}

// PresetWorkflowDependencies defines dependencies for workflow/state management.
var PresetWorkflowDependencies = []ToolDependency{
	// Checkpoint operations: validate before saving state
	{Tool: "checkpoint_save", Requires: []string{"validate_state"}},

	// Memory operations: commit before persisting
	{Tool: "memory_persist", Requires: []string{"memory_commit"}},

	// State transitions: analyze current state first
	{Tool: "state_transition", Requires: []string{"validate_state"}},
}

// PresetSafetyDependencies defines dependencies for safe operation enforcement.
// These prevent dangerous or irreversible operations without proper checks.
var PresetSafetyDependencies = []ToolDependency{
	// Dangerous operations: require explicit confirmation
	{Tool: "git_reset", Requires: []string{"git_status"}},
	{Tool: "git_clean", Requires: []string{"git_status"}},
	{Tool: "file_delete", Requires: []string{"file_read"}},

	// Network operations: validate before executing
	{Tool: "http_request", Requires: []string{"validate_url"}},
	{Tool: "api_call", Requires: []string{"validate_endpoint"}},
}

// PresetTestingDependencies defines dependencies for test execution workflows.
var PresetTestingDependencies = []ToolDependency{
	// Test discovery: build first
	{Tool: "test_discover", Requires: []string{"go_build"}},

	// Test execution: discover first
	{Tool: "test_run", Requires: []string{"test_discover"}},

	// Coverage: run tests first
	{Tool: "coverage_report", Requires: []string{"test_run"}},

	// Benchmark: build optimized first
	{Tool: "benchmark_run", Requires: []string{"go_build"}},
}

// PresetShellDependencies defines dependencies for shell command execution.
var PresetShellDependencies = []ToolDependency{
	// Shell commands: validate environment first
	{Tool: "shell_exec", Requires: []string{"env_check"}},

	// Package management: check before installing
	{Tool: "package_install", Requires: []string{"package_check"}},
}

// AllPresets combines all preset dependency rules.
// This can be used when comprehensive validation is needed.
var AllPresets = combinePresets(
	PresetCodeEditDependencies,
	PresetAnalysisDependencies,
	PresetWorkflowDependencies,
	PresetSafetyDependencies,
	PresetTestingDependencies,
	PresetShellDependencies,
)

// combinePresets merges multiple preset slices into one.
func combinePresets(presets ...[]ToolDependency) []ToolDependency {
	var combined []ToolDependency
	seen := make(map[string]bool)

	for _, preset := range presets {
		for _, dep := range preset {
			// Create a key to deduplicate
			key := dep.Tool + "->"
			for _, r := range dep.Requires {
				key += r + ","
			}
			if !seen[key] {
				seen[key] = true
				combined = append(combined, dep)
			}
		}
	}

	return combined
}

// GetPresetByName returns a preset by its name.
// Supported names: "code_edit", "analysis", "workflow", "safety", "testing", "shell", "all"
func GetPresetByName(name string) []ToolDependency {
	switch name {
	case "code_edit":
		return PresetCodeEditDependencies
	case "analysis":
		return PresetAnalysisDependencies
	case "workflow":
		return PresetWorkflowDependencies
	case "safety":
		return PresetSafetyDependencies
	case "testing":
		return PresetTestingDependencies
	case "shell":
		return PresetShellDependencies
	case "all":
		return AllPresets
	default:
		return nil
	}
}

// ListPresetNames returns the list of available preset names.
func ListPresetNames() []string {
	return []string{
		"code_edit",
		"analysis",
		"workflow",
		"safety",
		"testing",
		"shell",
		"all",
	}
}
