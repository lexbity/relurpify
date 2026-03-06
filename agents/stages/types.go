package stages

// FileRef describes a file selected for deeper work.
type FileRef struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// FileSelection captures the exploration outcome for a coding task.
type FileSelection struct {
	RelevantFiles   []FileRef `json:"relevant_files"`
	ToolSuggestions []string  `json:"tool_suggestions,omitempty"`
	Summary         string    `json:"summary"`
}

// Issue describes one concrete problem discovered during analysis.
type Issue struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
}

// IssueList captures structured analysis output.
type IssueList struct {
	Issues  []Issue `json:"issues"`
	Summary string  `json:"summary"`
}

// FixStep is one explicit plan item for resolving issues.
type FixStep struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Files       []string `json:"files,omitempty"`
}

// FixPlan describes the intended strategy and execution steps.
type FixPlan struct {
	Strategy string    `json:"strategy"`
	Steps    []FixStep `json:"steps"`
	Risks    []string  `json:"risks,omitempty"`
}

// FileEdit proposes one concrete file mutation.
type FileEdit struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Content string `json:"content,omitempty"`
	Summary string `json:"summary"`
}

// EditPlan captures the code generation stage output.
type EditPlan struct {
	Edits   []FileEdit `json:"edits"`
	Summary string     `json:"summary"`
}

// VerificationCheck records one verification activity or recommendation.
type VerificationCheck struct {
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

// VerificationReport captures the verification stage output.
type VerificationReport struct {
	Status          string              `json:"status"`
	Summary         string              `json:"summary"`
	Checks          []VerificationCheck `json:"checks,omitempty"`
	RemainingIssues []string            `json:"remaining_issues,omitempty"`
}
