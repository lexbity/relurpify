package agenttest

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
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

func seedWorkflowRetrievalStateForCase(state *contextdata.Envelope, task *execution.Task, c CaseSpec) {
	if state == nil || task == nil || task.Context == nil {
		return
	}
	workflowID, ok := task.Context["workflow_id"]
	if !ok || strings.TrimSpace(fmt.Sprint(workflowID)) == "" {
		return
	}
	var summary string
	var seededPlan map[string]any
	for _, workflow := range c.Setup.Workflows {
		if workflow.Workflow.WorkflowID != fmt.Sprint(workflowID) {
			continue
		}
		for _, record := range workflow.Knowledge {
			if text := strings.TrimSpace(record.Content); text != "" {
				if summary != "" {
					summary += "\n"
				}
				summary += text
			}
			if seededPlan == nil {
				seededPlan = seededWorkflowPlan(record)
			}
		}
		break
	}
	if summary == "" {
		return
	}
	payload := map[string]any{
		"query":   task.Instruction,
		"summary": summary,
		"scope":   fmt.Sprintf("workflow:%s", fmt.Sprint(workflowID)),
	}
	task.Context["workflow_retrieval"] = payload
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(task.Context["mode"])))
	switch mode {
	case "architect":
		state.SetWorkingValue("planner.workflow_retrieval", payload, contextdata.MemoryClassTask)
	default:
		state.SetWorkingValue("pipeline.workflow_retrieval", payload, contextdata.MemoryClassTask)
	}
	if seededPlan != nil {
		state.SetWorkingValue("pipeline.plan", seededPlan, contextdata.MemoryClassTask)
	}
}

func seededWorkflowPlan(record WorkflowKnowledgeSeedSpec) map[string]any {
	title := strings.TrimSpace(record.Title)
	content := strings.TrimSpace(record.Content)
	lowerTitle := strings.ToLower(title)
	lowerContent := strings.ToLower(content)
	if !strings.Contains(lowerTitle, "compiled plan") && !strings.HasPrefix(lowerContent, "plan:") {
		return nil
	}
	step := map[string]any{
		"id":          "seeded-plan-step-1",
		"title":       firstNonEmpty(title, "Compiled plan"),
		"description": content,
	}
	if scope := seededWorkflowPlanScope(content); len(scope) > 0 {
		step["scope"] = scope
	}
	return map[string]any{
		"source":  "agenttest.workflow_knowledge",
		"summary": content,
		"steps":   []map[string]any{step},
	}
}

func seededWorkflowPlanScope(content string) []string {
	re := regexp.MustCompile(`[\w./-]+\.(?:go|md|yaml|yml|json|toml|txt)`)
	matches := re.FindAllString(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if _, ok := seen[match]; ok {
			continue
		}
		seen[match] = struct{}{}
		out = append(out, match)
	}
	return out
}

func shouldRestrictAllowedCapabilitiesForCase(c CaseSpec) bool {
	mode := ""
	if c.Context != nil {
		if raw, ok := c.Context["mode"]; ok {
			mode = strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
		}
	}
	switch mode {
	case "ask", "debug", "architect":
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.TaskType), "analysis")
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

func applySetup(workspace, targetWorkspace string, setup SetupSpec, sandbox bool, logger *log.Logger) (cleanup func(), err error) {
	type original struct {
		path    string
		existed bool
		data    []byte
	}
	var originals []original
	for _, f := range setup.Files {
		if f.Path == "" {
			continue
		}
		target, err := resolvePathWithin(workspace, f.Path)
		if err != nil {
			return nil, err
		}
		mode, err := parseSetupFileMode(f.Mode)
		if err != nil {
			return nil, err
		}
		if data, readErr := os.ReadFile(target); readErr == nil {
			originals = append(originals, original{path: target, existed: true, data: data})
		} else {
			originals = append(originals, original{path: target, existed: false})
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}

		var content []byte
		if f.Content != "" {
			content = []byte(f.Content)
		} else {
			// Copy from source fixtures
			srcPath := filepath.Join(targetWorkspace, f.Path)
			if _, err := os.Stat(srcPath); err != nil {
				return nil, fmt.Errorf("fixture file not found at %s (targetWorkspace=%s): %w", srcPath, targetWorkspace, err)
			}
			content, err = os.ReadFile(srcPath)
			if err != nil {
				return nil, fmt.Errorf("read fixture from %s (targetWorkspace=%s): %w", srcPath, targetWorkspace, err)
			}
		}

		if err := os.WriteFile(target, content, mode); err != nil {
			return nil, err
		}
	}
	cleanup = func() {
		for _, orig := range originals {
			if orig.existed {
				_ = os.WriteFile(orig.path, orig.data, 0o644)
			} else {
				_ = os.Remove(orig.path)
			}
		}
	}
	if setup.GitInit {
		gitDir, err := resolvePathWithin(workspace, ".git")
		if err != nil {
			return nil, err
		}
		_ = os.RemoveAll(gitDir)
		if !sandbox {
			for _, args := range [][]string{
				{"init"},
				{"config", "user.name", "agenttest"},
				{"config", "user.email", "agenttest@example.invalid"},
				{"add", "."},
				{"commit", "-m", "agenttest baseline"},
			} {
				cmd := exec.Command("git", args...)
				cmd.Dir = workspace
				_ = cmd.Run()
			}
		}
	}
	if logger != nil {
		logger.Printf("setup complete for %s", workspace)
	}
	return cleanup, nil
}

func modelProvenanceDigest(provenance *BackendModelProvenance) string {
	if provenance == nil {
		return ""
	}
	// Digest may not be available for all providers (e.g., LM Studio)
	// Try to extract from details if not in top-level field
	if provenance.Digest != "" {
		return provenance.Digest
	}
	if provenance.Details != nil {
		if digest, ok := provenance.Details["digest"].(string); ok {
			return digest
		}
	}
	return ""
}

func modelProvenanceName(provenance *BackendModelProvenance) string {
	if provenance == nil {
		return ""
	}
	// Use LoadedName/LoadedModel which are populated from generic ModelInfo
	return firstNonEmpty(provenance.LoadedName, provenance.LoadedModel)
}

type BackendProviderProvenance struct {
	Provider      string `json:"provider,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	ResetStrategy string `json:"reset_strategy,omitempty"`
	ResetBetween  bool   `json:"reset_between,omitempty"`
}

func providerProvenanceForExecution(execution resolvedCaseExecution) *BackendProviderProvenance {
	if strings.TrimSpace(execution.Provider) == "" && strings.TrimSpace(execution.Endpoint) == "" {
		return nil
	}
	return &BackendProviderProvenance{
		Provider:      execution.Provider,
		Endpoint:      execution.Endpoint,
		ResetStrategy: execution.ProviderResetStrategy,
		ResetBetween:  execution.ProviderResetBetween,
	}
}
