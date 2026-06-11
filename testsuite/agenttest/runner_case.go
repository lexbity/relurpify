package agenttest

import (
	"strings"
	"time"
)

func failedCaseReport(startedAt time.Time, name, model, modelSource, manifestModel, endpoint, recordingMode, tapePath, workspace, artifactsDir, errMsg, failureKind string, attempts int) CaseReport {
	finishedAt := time.Now().UTC()
	return CaseReport{
		Name:          name,
		Model:         model,
		ModelSource:   modelSource,
		ManifestModel: manifestModel,
		Endpoint:      endpoint,
		RecordingMode: recordingMode,
		TapePath:      tapePath,
		Workspace:     workspace,
		ArtifactsDir:  artifactsDir,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		Success:       false,
		Error:         errMsg,
		FailureKind:   failureKind,
		Attempts:      attempts,
	}
}

// getLatencyMapOrEmpty returns the ToolLatencies map or an empty map if nil
func resolveCaseMaxRetries(opts RunOptions) int {
	switch {
	case opts.MaxRetries == 0:
		return 3
	case opts.MaxRetries < 0:
		return 0
	default:
		return opts.MaxRetries
	}
}

func resolveTemplateProfile(suite *Suite, c CaseSpec) string {
	templateProfile := suite.Spec.Workspace.TemplateProfile
	if c.Overrides.Workspace != nil && c.Overrides.Workspace.TemplateProfile != "" {
		templateProfile = c.Overrides.Workspace.TemplateProfile
	}
	if templateProfile == "" {
		return "default"
	}
	return templateProfile
}

func resolveWorkspaceExclude(suite *Suite, c CaseSpec) []string {
	exclude := append([]string{}, suite.Spec.Workspace.Exclude...)
	if c.Overrides.Workspace != nil && len(c.Overrides.Workspace.Exclude) > 0 {
		exclude = append([]string{}, c.Overrides.Workspace.Exclude...)
	}
	if len(exclude) == 0 {
		return []string{
			".git/**",
			".gocache/**",
			".gomodcache/**",
			"relurpify_cfg/test_run/**",
		}
	}
	normalized := make([]string, 0, len(exclude))
	for _, pattern := range exclude {
		pattern = strings.ReplaceAll(pattern, "testsetup", "test_run")
		pattern = strings.ReplaceAll(pattern, "test_runs", "test_run")
		normalized = append(normalized, pattern)
	}
	return normalized
}

func resolveWorkspaceFiles(suite *Suite, c CaseSpec) []SetupFileSpec {
	files := append([]SetupFileSpec{}, suite.Spec.Workspace.Files...)
	files = append(files, c.Setup.Files...)
	if c.Overrides.Workspace != nil && len(c.Overrides.Workspace.Files) > 0 {
		files = append(files, c.Overrides.Workspace.Files...)
	}
	return files
}

type BackendProviderProvenance struct {
	Provider      string `json:"provider,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	ResetStrategy string `json:"reset_strategy,omitempty"`
	ResetBetween  bool   `json:"reset_between,omitempty"`
}
