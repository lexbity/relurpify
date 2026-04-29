package ingestion

// WorkspaceIngestionPolicy controls workspace scanning behavior.
type WorkspaceIngestionPolicy struct {
	// Mode determines the scanning scope.
	Mode IngestionMode

	// IncludeGlobs specifies glob patterns for files to include.
	IncludeGlobs []string

	// ExcludeGlobs specifies glob patterns for files to exclude.
	ExcludeGlobs []string
}

// DefaultWorkspacePolicy returns the default workspace ingestion policy.
func DefaultWorkspacePolicy() *WorkspaceIngestionPolicy {
	return &WorkspaceIngestionPolicy{
		Mode:          IngestionModeFilesOnly,
		IncludeGlobs:  []string{},
		ExcludeGlobs:  []string{"vendor/**", ".git/**", "node_modules/**"},
	}
}

// UpgradeMode upgrades the ingestion mode from files_only to incremental or full.
// This is called when the user grants blanket workspace read permission.
func (p *WorkspaceIngestionPolicy) UpgradeMode(newMode IngestionMode) {
	p.Mode = newMode
}
