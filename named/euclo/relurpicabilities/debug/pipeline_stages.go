package debug

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	frameworkpipeline "github.com/lexcodex/relurpify/framework/pipeline"
)

type investigationSummaryStage struct {
	task *core.Task
}

type repairReadinessStage struct {
	task *core.Task
}

func (s *investigationSummaryStage) Name() string { return "debug_investigation_summary" }

func (s *investigationSummaryStage) Contract() frameworkpipeline.ContractDescriptor {
	return frameworkpipeline.ContractDescriptor{
		Name: "debug-investigation-summary",
		Metadata: frameworkpipeline.ContractMetadata{
			InputKey:      "pipeline.analyze",
			OutputKey:     "euclo.debug_investigation_summary",
			SchemaVersion: "v1",
		},
	}
}

func (s *investigationSummaryStage) BuildPrompt(ctx *core.Context) (string, error) {
	var analyze any
	var verify any
	if ctx != nil {
		analyze, _ = ctx.Get("pipeline.analyze")
		verify, _ = ctx.Get("pipeline.verify")
	}
	return fmt.Sprintf(
		"Synthesize this debug investigation into a concise engineering summary.\nInstruction: %s\nAnalyze: %v\nVerification: %v\nReturn a short plain-text summary covering root cause, affected surface, and most important next action.",
		taskInstruction(s.task),
		analyze,
		verify,
	), nil
}

func (s *investigationSummaryStage) Decode(resp *core.LLMResponse) (any, error) {
	return strings.TrimSpace(resp.Text), nil
}

func (s *investigationSummaryStage) Validate(output any) error {
	if strings.TrimSpace(fmt.Sprint(output)) == "" {
		return fmt.Errorf("summary required")
	}
	return nil
}

func (s *investigationSummaryStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("euclo.debug_investigation_summary", strings.TrimSpace(fmt.Sprint(output)))
	return nil
}

func (s *repairReadinessStage) Name() string { return "debug_repair_readiness" }

func (s *repairReadinessStage) Contract() frameworkpipeline.ContractDescriptor {
	return frameworkpipeline.ContractDescriptor{
		Name: "debug-repair-readiness",
		Metadata: frameworkpipeline.ContractMetadata{
			InputKey:      "euclo.debug_investigation_summary",
			OutputKey:     "euclo.debug_repair_readiness",
			SchemaVersion: "v1",
		},
	}
}

func (s *repairReadinessStage) BuildPrompt(ctx *core.Context) (string, error) {
	var summary any
	var verify any
	if ctx != nil {
		summary, _ = ctx.Get("euclo.debug_investigation_summary")
		verify, _ = ctx.Get("pipeline.verify")
	}
	return fmt.Sprintf(
		"Assess whether the current debug investigation is ready to transition into implementation repair.\nInstruction: %s\nInvestigation summary: %v\nVerification: %v\nReturn a short plain-text readiness assessment with the strongest blocker or confidence signal.",
		taskInstruction(s.task),
		summary,
		verify,
	), nil
}

func (s *repairReadinessStage) Decode(resp *core.LLMResponse) (any, error) {
	return strings.TrimSpace(resp.Text), nil
}

func (s *repairReadinessStage) Validate(output any) error {
	if strings.TrimSpace(fmt.Sprint(output)) == "" {
		return fmt.Errorf("repair readiness summary required")
	}
	return nil
}

func (s *repairReadinessStage) Apply(ctx *core.Context, output any) error {
	ctx.Set("euclo.debug_repair_readiness", strings.TrimSpace(fmt.Sprint(output)))
	return nil
}
