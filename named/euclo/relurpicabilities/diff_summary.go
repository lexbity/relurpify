package relurpicabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/sandbox"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// DiffSummaryHandler implements the diff summary capability.
type DiffSummaryHandler struct {
	env agentenv.WorkspaceEnvironment
	frameworkPolicyContext
}

// NewDiffSummaryHandler creates a new diff summary handler.
func NewDiffSummaryHandler(env agentenv.WorkspaceEnvironment) *DiffSummaryHandler {
	return &DiffSummaryHandler{env: env}
}

// Descriptor returns the capability descriptor for the diff summary handler.
func (h *DiffSummaryHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.diff_summary",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Diff Summary",
		Version:       "1.0.0",
		Description:   "Summarizes git diff output and identifies risk areas",
		Category:      "review_synthesis",
		Tags:          []string{"git", "diff", "review", "relurpic"},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassReadOnly},
		EffectClasses: []core.EffectClass{},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"base_ref": {
					Type:        "string",
					Description: "Base git ref (default: HEAD~1)",
				},
				"head_ref": {
					Type:        "string",
					Description: "Head git ref (default: HEAD)",
				},
				"scope": {
					Type:        "string",
					Description: "Path scope for git diff -- <scope>",
				},
			},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if diff completed",
				},
				"summary": {
					Type:        "string",
					Description: "Summary of the diff",
				},
				"changed_files": {
					Type:        "array",
					Description: "List of changed files",
					Items:       &core.Schema{Type: "string"},
				},
				"additions": {
					Type:        "integer",
					Description: "Total lines added",
				},
				"deletions": {
					Type:        "integer",
					Description: "Total lines deleted",
				},
				"risk_areas": {
					Type:        "array",
					Description: "Identified risk areas",
					Items:       &core.Schema{Type: "object"},
				},
			},
		},
	}
}

// Invoke runs git diff and returns a structured summary.
func (h *DiffSummaryHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*contracts.CapabilityExecutionResult, error) {
	if h.env.CommandRunner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}

	baseRef, _ := stringArg(args, "base_ref")
	if baseRef == "" {
		baseRef = "HEAD~1"
	}
	headRef, _ := stringArg(args, "head_ref")
	if headRef == "" {
		headRef = "HEAD"
	}
	scope, _ := stringArg(args, "scope")
	if normalized, err := h.normalizedScope(scope); err != nil {
		return failResult(fmt.Sprintf("scope resolution failed: %v", err)), err
	} else {
		scope = normalized
	}

	workdir := ""
	if h.env.IndexManager != nil {
		workdir = h.env.IndexManager.WorkspacePath()
	}

	// Run git diff --stat to get file list and line counts
	statArgs := []string{"git", "diff", "--stat", baseRef, headRef}
	if scope != "" {
		statArgs = append(statArgs, "--", scope)
	}
	statReq := sandbox.CommandRequest{
		Args:    statArgs,
		Workdir: workdir,
		Timeout: 30 * time.Second,
	}
	if err := h.authorizeCommand(ctx, h.env, statReq, "euclo diff summary"); err != nil {
		return failResult(fmt.Sprintf("diff command denied: %v", err)), err
	}
	statOut, _, err := h.env.CommandRunner.Run(ctx, statReq)
	if err != nil {
		return &contracts.CapabilityExecutionResult{
			Success: false,
			Data: map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("git diff --stat failed: %v", err),
			},
		}, nil
	}

	// Run git diff --name-only to get changed files
	nameArgs := []string{"git", "diff", "--name-only", baseRef, headRef}
	if scope != "" {
		nameArgs = append(nameArgs, "--", scope)
	}
	nameReq := sandbox.CommandRequest{
		Args:    nameArgs,
		Workdir: workdir,
		Timeout: 30 * time.Second,
	}
	if err := h.authorizeCommand(ctx, h.env, nameReq, "euclo diff summary"); err != nil {
		return failResult(fmt.Sprintf("diff command denied: %v", err)), err
	}
	nameOut, _, _ := h.env.CommandRunner.Run(ctx, nameReq)

	changedFiles := []string{}
	for _, line := range strings.Split(strings.TrimSpace(nameOut), "\n") {
		if line != "" {
			changedFiles = append(changedFiles, line)
		}
	}

	additions, deletions := parseStatSummary(statOut)
	riskAreas := identifyRiskAreas(changedFiles)
	summary := truncate(statOut, 4096)

	if h.env.Model != nil {
		if agentSummary, err := h.runReactSummary(ctx, baseRef, headRef, scope, statOut, changedFiles, additions, deletions, riskAreas); err == nil && strings.TrimSpace(agentSummary) != "" {
			summary = agentSummary
		}
	}

	return &contracts.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"success":       true,
			"summary":       summary,
			"changed_files": changedFiles,
			"additions":     additions,
			"deletions":     deletions,
			"risk_areas":    riskAreas,
		},
	}, nil
}

func (h *DiffSummaryHandler) runReactSummary(ctx context.Context, baseRef, headRef, scope, statOut string, changedFiles []string, additions, deletions int, riskAreas []map[string]interface{}) (string, error) {
	runtimeEnv := h.env
	if runtimeEnv.Config == nil {
		runtimeEnv.Config = &core.Config{}
	}
	agent := reactpkg.New(&runtimeEnv)
	if agent == nil {
		return "", fmt.Errorf("react agent could not be constructed")
	}

	scopedEnv := contextdata.NewEnvelope("euclo.diff_summary", "session")
	scopedEnv.SetWorkingValue("diff.base_ref", baseRef, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("diff.head_ref", headRef, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("diff.scope", scope, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("diff.stat", statOut, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("diff.changed_files", changedFiles, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("diff.additions", additions, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("diff.deletions", deletions, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("diff.risk_areas", riskAreas, contextdata.MemoryClassTask)

	task := &core.Task{
		ID:          "euclo:cap.diff_summary",
		Type:        string(core.TaskTypeExplain),
		Instruction: buildDiffSummaryInstruction(baseRef, headRef, scope, statOut, changedFiles, additions, deletions, riskAreas),
		Data: map[string]interface{}{
			"base_ref":      baseRef,
			"head_ref":      headRef,
			"scope":         scope,
			"changed_files": changedFiles,
			"additions":     additions,
			"deletions":     deletions,
			"risk_areas":    riskAreas,
			"stat":          statOut,
			"summary_mode":  true,
		},
	}

	result, err := agent.Execute(ctx, task, scopedEnv)
	if err != nil {
		return "", err
	}
	if summary := summaryFromReActResult(scopedEnv, result); summary != "" {
		return summary, nil
	}
	if result != nil && result.Data != nil {
		if summary, ok := result.Data["summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return summary, nil
		}
		if text, ok := result.Data["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text, nil
		}
	}
	return "", nil
}

func (h *DiffSummaryHandler) normalizedScope(scope string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "", nil
	}
	resolved, err := h.resolveWorkspacePath(h.env, scope)
	if err != nil {
		return "", err
	}
	root := workspacePath(h.env)
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Clean(rel)), nil
}

func buildDiffSummaryInstruction(baseRef, headRef, scope, statOut string, changedFiles []string, additions, deletions int, riskAreas []map[string]interface{}) string {
	payload, _ := json.Marshal(map[string]interface{}{
		"base_ref":      baseRef,
		"head_ref":      headRef,
		"scope":         scope,
		"changed_files": changedFiles,
		"additions":     additions,
		"deletions":     deletions,
		"risk_areas":    riskAreas,
		"stat":          statOut,
	})
	return fmt.Sprintf(`Summarize the git diff payload below for a review workflow.
Return a short plain-language summary and keep the risk areas explicit.
Payload:
%s`, string(payload))
}

func summaryFromReActResult(env *contextdata.Envelope, result *core.Result) string {
	if env != nil {
		if summary, ok := env.GetWorkingValue("react.synthetic_summary"); ok {
			if s, ok := summary.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		if output, ok := env.GetWorkingValue("react.final_output"); ok {
			if s := reactSummaryFromValue(output); s != "" {
				return s
			}
		}
	}
	if result != nil && result.Data != nil {
		if s := strings.TrimSpace(fmt.Sprint(result.Data["summary"])); s != "" && s != "<nil>" {
			return s
		}
		if s := strings.TrimSpace(fmt.Sprint(result.Data["text"])); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func reactSummaryFromValue(value any) string {
	data, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	if summary, ok := data["summary"].(string); ok {
		return strings.TrimSpace(summary)
	}
	return ""
}

// parseStatSummary extracts total additions and deletions from git diff --stat output.
func parseStatSummary(stat string) (additions, deletions int) {
	lines := strings.Split(stat, "\n")
	for _, line := range lines {
		if strings.Contains(line, "insertion") || strings.Contains(line, "deletion") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if strings.HasPrefix(f, "insertion") && i > 0 {
					fmt.Sscanf(fields[i-1], "%d", &additions)
				}
				if strings.HasPrefix(f, "deletion") && i > 0 {
					fmt.Sscanf(fields[i-1], "%d", &deletions)
				}
			}
		}
	}
	return
}

// identifyRiskAreas flags files in sensitive paths.
func identifyRiskAreas(files []string) []map[string]interface{} {
	riskPatterns := []struct {
		pattern  string
		reason   string
		severity string
	}{
		{"auth", "authentication-sensitive path", "high"},
		{"security", "security-sensitive path", "high"},
		{"crypto", "cryptography-sensitive path", "high"},
		{"permission", "permission-sensitive path", "high"},
		{"migration", "database migration", "medium"},
		{"schema", "schema change", "medium"},
		{"api", "public API change", "medium"},
		{"config", "configuration change", "low"},
	}

	var areas []map[string]interface{}
	for _, file := range files {
		lower := strings.ToLower(file)
		for _, p := range riskPatterns {
			if strings.Contains(lower, p.pattern) {
				areas = append(areas, map[string]interface{}{
					"file":     file,
					"reason":   p.reason,
					"severity": p.severity,
				})
				break
			}
		}
	}
	return areas
}
