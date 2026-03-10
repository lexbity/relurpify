package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/agents/pattern"
	"github.com/lexcodex/relurpify/framework/core"
)

func coordinatedRelurpicDescriptor(id, name, description string, kind core.CapabilityKind, role core.CoordinationRole, taskTypes []string, executionModes []core.CoordinationExecutionMode, input, output *core.Schema, annotations map[string]any, riskClasses []core.RiskClass, effectClasses []core.EffectClass) core.CapabilityDescriptor {
	return core.NormalizeCapabilityDescriptor(core.CapabilityDescriptor{
		ID:            id,
		Kind:          kind,
		RuntimeFamily: core.CapabilityRuntimeFamilyRelurpic,
		Name:          name,
		Description:   description,
		Category:      "relurpic-orchestration",
		Tags:          []string{"coordination", "relurpic", string(role)},
		Source: core.CapabilitySource{
			Scope: core.CapabilityScopeBuiltin,
		},
		TrustClass:    core.TrustClassBuiltinTrusted,
		RiskClasses:   append([]core.RiskClass{}, riskClasses...),
		EffectClasses: append([]core.EffectClass{}, effectClasses...),
		InputSchema:   input,
		OutputSchema:  output,
		Coordination: &core.CoordinationTargetMetadata{
			Target:                 true,
			Role:                   role,
			TaskTypes:              taskTypes,
			ExecutionModes:         executionModes,
			LongRunning:            false,
			MaxDepth:               1,
			MaxRuntimeSeconds:      600,
			DirectInsertionAllowed: false,
		},
		Availability: core.AvailabilitySpec{Available: true},
		Annotations:  annotations,
	})
}

func structuredTaskSchema(required ...string) *core.Schema {
	properties := map[string]*core.Schema{
		"instruction":           {Type: "string"},
		"task_id":               {Type: "string"},
		"workflow_id":           {Type: "string"},
		"context_summary":       {Type: "string"},
		"artifact_summary":      {Type: "string"},
		"acceptance_criteria":   {Type: "array", Items: &core.Schema{Type: "string"}},
		"verification_criteria": {Type: "array", Items: &core.Schema{Type: "string"}},
	}
	return &core.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func structuredObjectSchema(properties map[string]*core.Schema, required ...string) *core.Schema {
	return &core.Schema{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

func plannerInputSchema() *core.Schema {
	return structuredTaskSchema("instruction")
}

func plannerOutputSchema() *core.Schema {
	return &core.Schema{
		Type: "object",
		Properties: map[string]*core.Schema{
			"goal": {Type: "string"},
			"steps": {
				Type:  "array",
				Items: &core.Schema{Type: "object"},
			},
			"files": {
				Type:  "array",
				Items: &core.Schema{Type: "string"},
			},
			"dependencies": {Type: "object"},
			"summary":      {Type: "string"},
		},
		Required: []string{"goal", "steps", "files", "dependencies"},
	}
}

func invokeStructuredReasoner(ctx context.Context, model core.LanguageModel, modelName, prompt string) (*core.CapabilityExecutionResult, error) {
	resp, err := model.Generate(ctx, prompt, &core.LLMOptions{
		Model:       modelName,
		Temperature: 0.2,
		MaxTokens:   800,
	})
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(pattern.ExtractJSON(resp.Text)), &payload); err != nil {
		return nil, err
	}
	return &core.CapabilityExecutionResult{Success: true, Data: payload}, nil
}

func modelName(cfg *core.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Model
}

func stringifyContextValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []string:
		return strings.Join(typed, ", ")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text == "" {
				continue
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, ", ")
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func planStepsAsAny(steps []core.PlanStep) []any {
	out := make([]any, 0, len(steps))
	for _, step := range steps {
		out = append(out, map[string]any{
			"id":           step.ID,
			"description":  step.Description,
			"tool":         step.Tool,
			"params":       step.Params,
			"expected":     step.Expected,
			"verification": step.Verification,
			"status":       step.Status,
			"files":        append([]string{}, step.Files...),
		})
	}
	return out
}

func planFilesAsAny(files []string) []any {
	out := make([]any, 0, len(files))
	for _, file := range files {
		out = append(out, file)
	}
	return out
}

func planDependenciesAsAny(dependencies map[string][]string) map[string]any {
	out := make(map[string]any, len(dependencies))
	for key, values := range dependencies {
		out[key] = planFilesAsAny(values)
	}
	return out
}

func normalizePlanPayload(plan any) any {
	typed, ok := plan.(core.Plan)
	if !ok {
		return plan
	}
	return map[string]any{
		"goal":         typed.Goal,
		"steps":        planStepsAsAny(typed.Steps),
		"files":        planFilesAsAny(typed.Files),
		"dependencies": planDependenciesAsAny(typed.Dependencies),
	}
}
