package ingestion

// IngestionSpec defines what to ingest and how.
type IngestionSpec struct {
	// Mode controls scope: "files_only", "incremental", "full"
	Mode IngestionMode

	// ExplicitFiles are user-selected files (from TaskEnvelope.UserFiles + SessionPins).
	ExplicitFiles []string

	// WorkspaceRoot is required for incremental/full modes.
	WorkspaceRoot string

	// IncludeGlobs and ExcludeGlobs filter workspace scans.
	IncludeGlobs []string
	ExcludeGlobs []string

	// SinceRef is the git ref used as base for incremental scans.
	// Empty means "since last known scan ref stored in envelope".
	SinceRef string
}

// BuildIngestionSpec constructs an IngestionSpec from task envelope and config.
func BuildIngestionSpec(userFiles, sessionPins []string, workspaceRoot, defaultMode string, includeGlobs, excludeGlobs []string) *IngestionSpec {
	spec := &IngestionSpec{
		Mode:          IngestionMode(defaultMode),
		ExplicitFiles: append(userFiles, sessionPins...),
		WorkspaceRoot: workspaceRoot,
		IncludeGlobs:  includeGlobs,
		ExcludeGlobs:  excludeGlobs,
	}

	// If no explicit files and mode is files_only, we may need to emit a scope confirmation frame
	// This is handled at a higher level (interaction frame emission)
	return spec
}
