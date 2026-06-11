package relurpicabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/capability/sandbox"
	"codeburg.org/lexbit/relurpify/capability/schemacoerce"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	reactpkg "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/classification"
	"codeburg.org/lexbit/relurpify/model"
)

// DiffSummaryHandler implements the diff summary capability.
type DiffSummaryHandler struct {
	cmd   CommandDeps
	model model.LanguageModel
}

// NewDiffSummaryHandler creates a new diff summary handler.
func NewDiffSummaryHandler(cmd CommandDeps, m model.LanguageModel) *DiffSummaryHandler {
	return &DiffSummaryHandler{cmd: cmd, model: m}
}

// Descriptor returns the capability descriptor for the diff summary handler.
func (h *DiffSummaryHandler) Descriptor(ctx context.Context, env ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "euclo:cap.diff_summary",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyRelurpic,
		Name:          "Diff Summary",
		Version:       "1.0.0",
		Description:   "Summarizes git diff output and identifies risk areas",
		Category:      "review_synthesis",
		Tags:          []string{"git", "diff", "review", "relurpic"},
		Source: descriptor.CapabilitySource{
			Scope: classification.CapabilityScopeBuiltin,
		},
		TrustClass:    agentspec.TrustClassBuiltinTrusted,
		EffectClasses: []classification.EffectClass{},
		InputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
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
		OutputSchema: &schemacoerce.Schema{
			Type: "object",
			Properties: map[string]*schemacoerce.Schema{
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
					Items:       &schemacoerce.Schema{Type: "string"},
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
					Items:       &schemacoerce.Schema{Type: "object"},
				},
			},
		},
	}
}

// Invoke runs git diff and returns a structured summary.
func (h *DiffSummaryHandler) Invoke(ctx context.Context, env ports.State, args map[string]any) (*ports.ToolResult, error) {
	baseRef, _ := stringArg(args, "base_ref")
	if baseRef == "" {
		baseRef = "HEAD~1"
	}
	headRef, _ := stringArg(args, "head_ref")
	if headRef == "" {
		headRef = "HEAD"
	}
	scope, _ := stringArg(args, "scope")
	if normalized, err := normalizedDiffScope(scope, h.cmd.Workspace); err != nil {
		return failResult(fmt.Sprintf("scope resolution failed: %v", err)), err
	} else {
		scope = normalized
	}

	workdir := h.cmd.Workspace

	statArgs := []string{"git", "diff", "--stat", baseRef, headRef}
	if scope != "" {
		statArgs = append(statArgs, "--", scope)
	}
	statReq := sandbox.CommandRequest{
		Args:    statArgs,
		Workdir: workdir,
		Timeout: 30 * time.Second,
	}
	if h.cmd.Policy != nil {
		if err := h.cmd.Policy.AllowCommand(ctx, statReq); err != nil {
			return failResult(fmt.Sprintf("diff command denied: %v", err)), err
		}
	}
	if h.cmd.Runner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}
	statRes, err := h.cmd.Runner.Run(ctx, statReq)
	if err != nil {
		return &ports.ToolResult{
			Success: false,
			Data: map[string]any{
				"success": false,
				"error":   fmt.Sprintf("git diff --stat failed: %v", err),
			},
		}, nil
	}
	statOut := statRes.Stdout
	if statRes.ExitCode != 0 {
		statOut = statRes.Stderr
	}

	nameArgs := []string{"git", "diff", "--name-only", baseRef, headRef}
	if scope != "" {
		nameArgs = append(nameArgs, "--", scope)
	}
	nameReq := sandbox.CommandRequest{
		Args:    nameArgs,
		Workdir: workdir,
		Timeout: 30 * time.Second,
	}
	if h.cmd.Policy != nil {
		if err := h.cmd.Policy.AllowCommand(ctx, nameReq); err != nil {
			return failResult(fmt.Sprintf("diff command denied: %v", err)), err
		}
	}
	if h.cmd.Runner == nil {
		return failResult("CommandRunner not available in environment"), nil
	}
	nameRes, err := h.cmd.Runner.Run(ctx, nameReq)
	if err != nil {
		return &ports.ToolResult{
			Success: false,
			Data: map[string]any{
				"success": false,
				"error":   fmt.Sprintf("git diff --name-only failed: %v", err),
			},
		}, nil
	}
	nameOut := nameRes.Stdout

	changedFiles := []string{}
	for _, line := range strings.Split(strings.TrimSpace(nameOut), "\n") {
		if line != "" {
			changedFiles = append(changedFiles, line)
		}
	}

	additions, deletions := parseStatSummary(statOut)
	riskAreas := identifyRiskAreas(changedFiles)
	summary := truncate(statOut, 4096)

	if h.model != nil {
		if agentSummary, err := h.runReactSummary(ctx, baseRef, headRef, scope, statOut, changedFiles, additions, deletions, riskAreas); err == nil && strings.TrimSpace(agentSummary) != "" {
			summary = agentSummary
		}
	}

	return &ports.ToolResult{
		Success: true,
		Data: map[string]any{
			"success":       true,
			"summary":       summary,
			"changed_files": changedFiles,
			"additions":     additions,
			"deletions":     deletions,
			"risk_areas":    riskAreas,
		},
	}, nil
}

func (h *DiffSummaryHandler) runReactSummary(ctx context.Context, baseRef, headRef, scope, statOut string, changedFiles []string, additions, deletions int, riskAreas []map[string]any) (string, error) {
	agent := reactpkg.New(&paradigm.Deps{
		Model: h.model,
	})
	if agent == nil {
		return "", fmt.Errorf("react agent could not be constructed")
	}

	scopedEnv := contextdata.NewEnvelope("euclo.diff_summary", "session")
	contextdata.SetTyped(scopedEnv, "diff.base_ref", baseRef)
	contextdata.SetTyped(scopedEnv, "diff.head_ref", headRef)
	contextdata.SetTyped(scopedEnv, "diff.scope", scope)
	contextdata.SetTyped(scopedEnv, "diff.stat", statOut)
	contextdata.SetTyped(scopedEnv, "diff.changed_files", changedFiles)
	contextdata.SetTyped(scopedEnv, "diff.additions", additions)
	contextdata.SetTyped(scopedEnv, "diff.deletions", deletions)
	contextdata.SetTyped(scopedEnv, "diff.risk_areas", riskAreas)

	task := &execution.Task{
		ID:          "euclo:cap.diff_summary",
		Type:        string(execution.TaskTypeExplain),
		Instruction: buildDiffSummaryInstruction(baseRef, headRef, scope, statOut, changedFiles, additions, deletions, riskAreas),
		Data: map[string]any{
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
	if result != nil {
		if summary, ok := execution.ResultField(result.Data, "summary"); ok && strings.TrimSpace(fmt.Sprint(summary)) != "" {
			return fmt.Sprint(summary), nil
		}
		if text, ok := execution.ResultField(result.Data, "text"); ok && strings.TrimSpace(fmt.Sprint(text)) != "" {
			return fmt.Sprint(text), nil
		}
	}
	return "", nil
}

func normalizedDiffScope(scope, workspace string) (string, error) {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "", nil
	}
	resolved := resolveCandidatePath(scope, workspace)
	if resolved == "" {
		return "", fmt.Errorf("scope resolution failed: %s", scope)
	}
	rel, err := filepath.Rel(workspace, resolved)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Clean(rel)), nil
}

func buildDiffSummaryInstruction(baseRef, headRef, scope, statOut string, changedFiles []string, additions, deletions int, riskAreas []map[string]any) string {
	payload, err := json.Marshal(map[string]any{
		"base_ref":      baseRef,
		"head_ref":      headRef,
		"scope":         scope,
		"changed_files": changedFiles,
		"additions":     additions,
		"deletions":     deletions,
		"risk_areas":    riskAreas,
		"stat":          statOut,
	})
	if err != nil {
		payload = []byte("{}")
	}
	return fmt.Sprintf(`Summarize the git diff payload below for a review workflow.
Return a short plain-language summary and keep the risk areas explicit.
Payload:
%s`, string(payload))
}

func summaryFromReActResult(env *contextdata.Envelope, result *execution.Result) string {
	if env != nil {
		if summary, ok := contextdata.GetTyped[string](env, "react.synthetic_summary"); ok && strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
		if output, ok := contextdata.GetTyped[map[string]any](env, "react.final_output"); ok {
			if s := reactSummaryFromValue(output); s != "" {
				return s
			}
		}
	}
	if result != nil {
		if s, ok := execution.ResultField(result.Data, "summary"); ok {
			if trimmed := strings.TrimSpace(fmt.Sprint(s)); trimmed != "" && trimmed != "<nil>" {
				return trimmed
			}
		}
		if s, ok := execution.ResultField(result.Data, "text"); ok {
			if trimmed := strings.TrimSpace(fmt.Sprint(s)); trimmed != "" && trimmed != "<nil>" {
				return trimmed
			}
		}
	}
	return ""
}

func reactSummaryFromValue(value any) string {
	data, ok := value.(map[string]any)
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
func identifyRiskAreas(files []string) []map[string]any {
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

	var areas []map[string]any
	for _, file := range files {
		lower := strings.ToLower(file)
		for _, p := range riskPatterns {
			if strings.Contains(lower, p.pattern) {
				areas = append(areas, map[string]any{
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
