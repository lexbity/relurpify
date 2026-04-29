package relurpic

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

type reviewerReviewCapabilityHandler struct {
	model  core.LanguageModel
	config *core.Config
}

func (h reviewerReviewCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:reviewer.review",
		"reviewer.review",
		"Review a provided change summary or artifact bundle and return structured findings.",
		core.CapabilityKindTool,
		core.CoordinationRoleReviewer,
		[]string{"review"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredTaskSchema("instruction", "artifact_summary", "acceptance_criteria"),
		structuredObjectSchema(map[string]*core.Schema{
			"summary": {Type: "string"},
			"approve": {Type: "boolean"},
			"findings": {
				Type: "array",
				Items: &core.Schema{
					Type: "object",
					Properties: map[string]*core.Schema{
						"severity":    {Type: "string"},
						"description": {Type: "string"},
						"suggestion":  {Type: "string"},
					},
					Required: []string{"severity", "description"},
				},
			},
		}, "summary", "approve", "findings"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "review",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h reviewerReviewCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	instruction := strings.TrimSpace(fmt.Sprint(args["instruction"]))
	if instruction == "" {
		return nil, fmt.Errorf("instruction required")
	}
	prompt := fmt.Sprintf(`You are a code and artifact reviewer.
Task: %s
Artifact summary: %s
Acceptance criteria: %s
Return valid JSON: {"summary":string,"approve":bool,"findings":[{"severity":"high|medium|low","description":string,"suggestion":string}]}.`,
		instruction,
		strings.TrimSpace(fmt.Sprint(args["artifact_summary"])),
		stringifyContextValue(args["acceptance_criteria"]),
	)
	result, err := invokeStructuredReasoner(ctx, h.model, modelName(h.config), prompt)
	if err != nil {
		return nil, err
	}
	env.SetWorkingValue("review_result", result.Data, contextdata.MemoryClassTask)
	return result, nil
}

type verifierVerifyCapabilityHandler struct {
	model  core.LanguageModel
	config *core.Config
}

func (h verifierVerifyCapabilityHandler) Descriptor(ctx context.Context, env *contextdata.Envelope) core.CapabilityDescriptor {
	return coordinatedRelurpicDescriptor(
		"relurpic:verifier.verify",
		"verifier.verify",
		"Verify that a proposed result satisfies explicit criteria and identify remaining gaps.",
		core.CapabilityKindTool,
		core.CoordinationRoleVerifier,
		[]string{"verify"},
		[]core.CoordinationExecutionMode{core.CoordinationExecutionModeSync},
		structuredTaskSchema("instruction", "artifact_summary", "verification_criteria"),
		structuredObjectSchema(map[string]*core.Schema{
			"summary":       {Type: "string"},
			"verified":      {Type: "boolean"},
			"evidence":      {Type: "array", Items: &core.Schema{Type: "string"}},
			"missing_items": {Type: "array", Items: &core.Schema{Type: "string"}},
		}, "summary", "verified", "evidence", "missing_items"),
		map[string]any{
			"relurpic_capability": true,
			"workflow":            "verify",
		},
		[]core.RiskClass{core.RiskClassReadOnly},
		[]core.EffectClass{core.EffectClassContextInsertion},
	)
}

func (h verifierVerifyCapabilityHandler) Invoke(ctx context.Context, env *contextdata.Envelope, args map[string]interface{}) (*core.CapabilityExecutionResult, error) {
	instruction := strings.TrimSpace(fmt.Sprint(args["instruction"]))
	if instruction == "" {
		return nil, fmt.Errorf("instruction required")
	}
	prompt := fmt.Sprintf(`You are a verification specialist.
Task: %s
Artifact summary: %s
Verification criteria: %s
Return valid JSON: {"summary":string,"verified":bool,"evidence":[string],"missing_items":[string]}.`,
		instruction,
		strings.TrimSpace(fmt.Sprint(args["artifact_summary"])),
		stringifyContextValue(args["verification_criteria"]),
	)
	result, err := invokeStructuredReasoner(ctx, h.model, modelName(h.config), prompt)
	if err != nil {
		return nil, err
	}
	env.SetWorkingValue("verification_result", result.Data, contextdata.MemoryClassTask)
	return result, nil
}
