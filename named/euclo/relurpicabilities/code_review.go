package relurpicabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	reflectionagent "codeburg.org/lexbit/relurpify/agents/reflection"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// CodeReviewHandler implements the code review capability via an LLM sub-agent.
type CodeReviewHandler struct {
	env agentenv.WorkspaceEnvironment
}

// NewCodeReviewHandler creates a new code review handler.
func NewCodeReviewHandler(env agentenv.WorkspaceEnvironment) *CodeReviewHandler {
	return &CodeReviewHandler{env: env}
}

// Descriptor returns the capability descriptor for the code review handler.
func (h *CodeReviewHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return core.CapabilityDescriptor{
		ID:            "euclo:cap.code_review",
		Kind:          core.CapabilityKindTool,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          "Code Review",
		Version:       "1.0.0",
		Description:   "Reviews code for correctness, security, style, and architecture issues",
		Category:      "review_synthesis",
		Tags:          []string{"review", "llm", "relurpic"},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   []core.RiskClass{core.RiskClassReadOnly},
		EffectClasses: []core.EffectClass{},
		InputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"focus": {
					Type:        "string",
					Description: `Review focus: "correctness" | "security" | "style" | "architecture" | "all" (default: "all")`,
				},
			},
		},
		OutputSchema: &core.Schema{
			Type: "object",
			Properties: map[string]*core.Schema{
				"success": {
					Type:        "boolean",
					Description: "True if review completed",
				},
				"findings": {
					Type:        "array",
					Description: "Review findings",
					Items:       &core.Schema{Type: "object"},
				},
				"summary": {
					Type:        "string",
					Description: "Overall review summary",
				},
				"focus": {
					Type:        "string",
					Description: "The focus dimension used",
				},
				"file_count": {
					Type:        "integer",
					Description: "Number of files reviewed",
				},
			},
		},
	}
}

// Invoke reviews code from the envelope's user files or retrieval context.
func (h *CodeReviewHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	focus, _ := stringArg(args, "focus")
	if focus == "" {
		focus = "all"
	}

	if !hasReviewContext(env) {
		return &core.CapabilityExecutionResult{
			Success: true,
			Data: map[string]interface{}{
				"success":    true,
				"findings":   []interface{}{},
				"summary":    "no context to review",
				"focus":      focus,
				"file_count": 0,
			},
		}, nil
	}

	contextText, fileCount := buildReviewContext(env)
	if fileCount == 0 && strings.TrimSpace(contextText) == "" {
		return &core.CapabilityExecutionResult{
			Success: true,
			Data: map[string]interface{}{
				"success":    true,
				"findings":   []interface{}{},
				"summary":    "no context to review",
				"focus":      focus,
				"file_count": 0,
			},
		}, nil
	}

	findings, summary, err := h.runReflectionReview(ctx, env, focus, contextText)
	if err != nil || len(findings) == 0 && summary == "" {
		if err != nil {
			summary = strings.TrimSpace(err.Error())
		}
		if len(findings) == 0 {
			findings = heuristicReviewFindings(contextText, focus)
		}
		if summary == "" {
			if len(findings) > 0 {
				summary = "heuristic review completed"
			} else {
				summary = "code review completed"
			}
		}
	}

	return &core.CapabilityExecutionResult{
		Success: true,
		Data: map[string]interface{}{
			"success":    true,
			"findings":   findingsToInterfaces(findings),
			"summary":    summary,
			"focus":      focus,
			"file_count": fileCount,
		},
	}, nil
}

type codeReviewFinding struct {
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

type codeReviewPayload struct {
	Findings []codeReviewFinding `json:"findings"`
	Summary  string              `json:"summary"`
}

type reflectionReviewPayload struct {
	Issues []struct {
		Severity    string `json:"severity"`
		Description string `json:"description"`
		Suggestion  string `json:"suggestion"`
	} `json:"issues"`
	Approve bool `json:"approve"`
}

func hasReviewContext(env *contextdata.Envelope) bool {
	if env == nil {
		return false
	}
	return len(env.References.Retrieval) > 0 || len(env.WorkingData) > 0
}

func buildReviewContext(env *contextdata.Envelope) (string, int) {
	if env == nil {
		return "", 0
	}

	var parts []string
	fileCount := 0

	if len(env.References.Retrieval) > 0 {
		fileCount += len(env.References.Retrieval)
		for i, ref := range env.References.Retrieval {
			parts = append(parts, fmt.Sprintf("retrieval[%d]: query=%q scope=%q chunks=%d total_found=%d", i, ref.QueryText, ref.Scope, len(ref.ChunkIDs), ref.TotalFound))
		}
	}

	if len(env.WorkingData) > 0 {
		fileCount += len(env.WorkingData)
		keys := make([]string, 0, len(env.WorkingData))
		for key := range env.WorkingData {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("working[%s]: %s", key, truncate(fmt.Sprintf("%#v", env.WorkingData[key]), 2000)))
		}
	}

	return strings.Join(parts, "\n"), fileCount
}

func buildCodeReviewPrompt(focus, contextText string) string {
	var b strings.Builder
	b.WriteString("Review the provided workspace context.\n")
	b.WriteString("Focus: ")
	b.WriteString(focus)
	b.WriteString("\n")
	b.WriteString("Return JSON with keys: summary, findings.\n")
	b.WriteString("Each finding should include file, line, severity, category, description, suggestion.\n")
	b.WriteString("Severity must be one of error, warning, info.\n")
	b.WriteString("Context:\n")
	b.WriteString(contextText)
	return b.String()
}

func buildReflectionReviewTask(focus, contextText string) *core.Task {
	return &core.Task{
		ID:   "euclo:cap.code_review",
		Type: string(core.TaskTypeReview),
		Instruction: fmt.Sprintf(
			"Perform a code review with focus %q. Analyze the provided workspace context and return concise, actionable findings as JSON. Context:\n%s",
			focus,
			contextText,
		),
		Data: map[string]interface{}{
			"focus":       focus,
			"context":     contextText,
			"capability":  "euclo:cap.code_review",
			"paradigm":    "reflection",
			"review_mode": true,
		},
		Metadata: map[string]interface{}{
			"capability_id": "euclo:cap.code_review",
			"focus":         focus,
		},
	}
}

func (h *CodeReviewHandler) runReflectionReview(ctx context.Context, env *contextdata.Envelope, focus, contextText string) ([]map[string]interface{}, string, error) {
	if h.env.Model == nil {
		return nil, "", fmt.Errorf("LLM model not available in environment")
	}

	scopedEnv := env
	if scopedEnv != nil {
		scopedEnv = scopedEnv.Clone()
	}
	if scopedEnv == nil {
		scopedEnv = contextdata.NewEnvelope("euclo.code_review", "session")
	}
	scopedEnv.SetWorkingValue("code_review.focus", focus, contextdata.MemoryClassTask)
	scopedEnv.SetWorkingValue("code_review.context", contextText, contextdata.MemoryClassTask)

	task := buildReflectionReviewTask(focus, contextText)
	runtimeEnv := h.env
	if runtimeEnv.Config == nil {
		runtimeEnv.Config = &core.Config{}
	}
	agent := reflectionagent.New(&runtimeEnv, reactpkg.New(&runtimeEnv))
	if agent == nil {
		return nil, "", fmt.Errorf("reflection agent could not be constructed")
	}

	if _, err := agent.Execute(ctx, task, scopedEnv); err != nil {
		return nil, "", err
	}

	findings, summary, ok := extractReflectionReview(scopedEnv)
	if !ok {
		return nil, "", fmt.Errorf("reflection agent did not produce a review payload")
	}
	if summary == "" {
		if len(findings) > 0 {
			summary = "reflection review completed"
		} else {
			summary = "reflection review approved"
		}
	}

	if env != nil {
		writeCodeReviewReferences(env, findings)
	}

	return findings, summary, nil
}

func parseCodeReviewResponse(raw string) ([]map[string]interface{}, string, bool) {
	var payload codeReviewPayload
	if err := json.Unmarshal([]byte(reactpkg.ExtractJSON(raw)), &payload); err != nil {
		return nil, "", false
	}

	findings := make([]map[string]interface{}, 0, len(payload.Findings))
	for _, finding := range payload.Findings {
		findings = append(findings, map[string]interface{}{
			"file":        finding.File,
			"line":        finding.Line,
			"severity":    normalizeReviewSeverity(finding.Severity),
			"category":    finding.Category,
			"description": finding.Description,
			"suggestion":  finding.Suggestion,
		})
	}
	return findings, strings.TrimSpace(payload.Summary), true
}

func extractReflectionReview(env *contextdata.Envelope) ([]map[string]interface{}, string, bool) {
	if env == nil {
		return nil, "", false
	}
	if review, ok := env.GetWorkingValue("reflection.review"); ok {
		if findings, summary, ok := decodeReflectionReview(review); ok {
			return findings, summary, true
		}
	}
	if result, ok := env.GetWorkingValue("reflection.last_result"); ok {
		if coreResult, ok := result.(*core.Result); ok && coreResult != nil {
			if review, ok := coreResult.Data["review"]; ok {
				if findings, summary, ok := decodeReflectionReview(review); ok {
					return findings, summary, true
				}
			}
		}
	}
	return nil, "", false
}

func decodeReflectionReview(review any) ([]map[string]interface{}, string, bool) {
	if review == nil {
		return nil, "", false
	}
	raw, err := json.Marshal(review)
	if err != nil {
		return nil, "", false
	}
	var payload reflectionReviewPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", false
	}
	findings := make([]map[string]interface{}, 0, len(payload.Issues))
	for _, issue := range payload.Issues {
		findings = append(findings, map[string]interface{}{
			"file":        "",
			"line":        0,
			"severity":    normalizeReviewSeverity(issue.Severity),
			"category":    focusCategory("all"),
			"description": issue.Description,
			"suggestion":  issue.Suggestion,
		})
	}
	summary := "reflection review completed"
	if payload.Approve && len(findings) == 0 {
		summary = "reflection review approved"
	}
	return findings, summary, true
}

func heuristicReviewFindings(contextText, focus string) []map[string]interface{} {
	lower := strings.ToLower(contextText)
	var findings []map[string]interface{}

	addFinding := func(severity, category, description, suggestion string) {
		findings = append(findings, map[string]interface{}{
			"file":        "",
			"line":        0,
			"severity":    severity,
			"category":    category,
			"description": description,
			"suggestion":  suggestion,
		})
	}

	if strings.Contains(lower, "todo") || strings.Contains(lower, "fixme") || strings.Contains(lower, "stub") {
		addFinding("warning", focusCategory(focus), "workspace context contains placeholder or unfinished implementation markers", "replace placeholders with the production implementation before merging")
	}
	if strings.Contains(lower, "panic(") || strings.Contains(lower, "panic ") {
		addFinding("error", "correctness", "workspace context references a panic path", "guard the failure path or return a typed error instead of panicking")
	}
	if strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "token") {
		addFinding("warning", "security", "workspace context references a sensitive credential-like value", "avoid logging or embedding secrets in prompts and results")
	}

	return findings
}

func writeCodeReviewReferences(env *contextdata.Envelope, findings []map[string]interface{}) {
	if env == nil {
		return
	}
	for i, finding := range findings {
		file, _ := finding["file"].(string)
		if strings.TrimSpace(file) == "" {
			continue
		}
		line := intValue(finding["line"])
		if line <= 0 {
			line = 1
		}
		env.AddRetrievalReference(contextdata.RetrievalReference{
			QueryID:     fmt.Sprintf("code_review_%d", i),
			QueryText:   fmt.Sprintf("%s:%d", file, line),
			Scope:       file,
			ChunkIDs:    []contextdata.ChunkID{contextdata.ChunkID(fmt.Sprintf("%s:%d", file, line))},
			TotalFound:  1,
			RetrievedAt: time.Now().UTC(),
		})
	}
}

func intValue(v any) int {
	switch typed := v.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func findingsToInterfaces(findings []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding)
	}
	return out
}

func normalizeReviewSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "error", "high":
		return "error"
	case "info", "low":
		return "info"
	default:
		return "warning"
	}
}

func focusCategory(focus string) string {
	switch strings.ToLower(strings.TrimSpace(focus)) {
	case "correctness", "security", "style", "architecture":
		return strings.ToLower(strings.TrimSpace(focus))
	default:
		return "all"
	}
}

func configuredModelName(cfg *core.Config) string {
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Model); name != "" {
			return name
		}
		return strings.TrimSpace(cfg.InferenceModel)
	}
	return ""
}
