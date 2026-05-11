package thoughtrecipe

import (
	"fmt"
	"strings"
)

// LowerDocument converts a source-level Euclo thoughtrecipe document into an
// execution plan that can be consumed by the existing thoughtrecipe runtime.
func LowerDocument(doc *ThoughtRecipeDocument) (*ExecutionPlan, error) {
	if doc == nil {
		return nil, fmt.Errorf("thoughtrecipe document is nil")
	}

	plan := &ExecutionPlan{
		ThoughtRecipe: &ThoughtRecipe{
			ID:   doc.Name,
			Name: doc.Name,
			Metadata: ThoughtRecipeMetadata{
				Name: doc.Name,
			},
		},
		Agents: make(map[string]AgentBinding),
	}

	if doc.Header.Name.Value != "" {
		plan.ThoughtRecipe.Name = doc.Header.Name.Value
		plan.ThoughtRecipe.Metadata.Name = doc.Header.Name.Value
		plan.ThoughtRecipe.ID = doc.Header.Name.Value
	}
	if doc.Header.Description != nil {
		plan.ThoughtRecipe.Description = strings.TrimSpace(doc.Header.Description.Value)
	}
	if trigger := firstTriggerDecl(doc); trigger != nil {
		plan.ThoughtRecipe.RouteKind = TriggerRouteKindFromDecl(trigger)
		plan.RouteKind = plan.ThoughtRecipe.RouteKind
		meta, err := TriggerAssociationsFromDecl(trigger)
		if err != nil {
			return nil, err
		}
		plan.ThoughtRecipe.Metadata.Families = meta.Families
		plan.ThoughtRecipe.Metadata.Keywords = meta.Keywords
		plan.ThoughtRecipe.Metadata.HandoffTargets = meta.HandoffTargets
		plan.ThoughtRecipe.Metadata.Tags = meta.Tags
	}

	for _, decl := range doc.Declarations {
		switch node := decl.(type) {
		case *AgentDecl:
			paradigm := strings.TrimSpace(node.AgentType.Value)
			if !isSupportedAgentParadigm(paradigm) {
				return nil, fmt.Errorf("%s:%d:%d: unsupported agent paradigm %q", node.GetSpan().Start.File, node.GetSpan().Start.Line, node.GetSpan().Start.Column, paradigm)
			}
			plan.Agents[strings.TrimSpace(node.Name.Value)] = AgentBinding{
				Name:     strings.TrimSpace(node.Name.Value),
				Paradigm: paradigm,
				Span:     node.GetSpan(),
			}
		}
	}

	var runIndex int
	for _, decl := range doc.Declarations {
		if err := gatherLoweredFromDeclaration(decl, plan, &runIndex); err != nil {
			return nil, err
		}
	}

	return plan, nil
}

func firstTriggerDecl(doc *ThoughtRecipeDocument) *TriggerDecl {
	if doc == nil {
		return nil
	}
	for _, decl := range doc.Declarations {
		if trigger, ok := decl.(*TriggerDecl); ok {
			return trigger
		}
	}
	return nil
}

func lowerRunItems(items []ExecutionItem) (sources []string, goals []string, directives []string, captures []CaptureBinding, promptID string, config map[string]any) {
	var directiveConfigs []map[string]any
	for _, item := range items {
		switch node := item.(type) {
		case *FromClause:
			if raw := strings.TrimSpace(valueExprRaw(node.Source)); raw != "" {
				sources = append(sources, raw)
			}
		case *GoalClause:
			if node.PromptID != nil {
				if ref := strings.TrimSpace(node.PromptID.ResolvedID); ref != "" {
					promptID = ref
				} else if ref := strings.TrimSpace(node.PromptID.Name.Value); ref != "" {
					promptID = ref
				}
				continue
			}
			if text := strings.TrimSpace(node.Text.Value); text != "" {
				goals = append(goals, text)
			}
		case *DirectiveClause:
			if raw := strings.TrimSpace(node.Raw); raw != "" {
				directives = append(directives, raw)
			}
			directiveConfigs = append(directiveConfigs, map[string]any{
				"name":      node.Name.Value,
				"arguments": rawValueExprList(node.Arguments),
				"raw":       node.Raw,
			})
		case *DirectiveBlock:
			if raw := strings.TrimSpace(node.Raw); raw != "" {
				directives = append(directives, raw)
			}
			directiveConfigs = append(directiveConfigs, map[string]any{
				"name":      node.Name.Value,
				"arguments": rawValueExprList(node.Arguments),
				"predicate": predicateRaw(*node.Predicate),
				"body":      summarizeExecutionItems(node.Body),
				"raw":       node.Raw,
			})
		case *CaptureBlock:
			captures = append(captures, LowerCaptureBindings(node)...)
			directiveConfigs = append(directiveConfigs, map[string]any{
				"type":     "capture",
				"bindings": summarizeCaptureBindings(node.Bindings),
			})
		case *CapabilityInvocation:
			directiveConfigs = append(directiveConfigs, map[string]any{
				"type":       "capability",
				"namespace":  node.Namespace.Value,
				"capability": node.Capability.Value,
				"target":     valueExprRaw(node.Target),
				"input":      valueExprRaw(node.Input),
			})
		}
	}
	if len(sources) > 0 || len(goals) > 0 || len(directives) > 0 || len(captures) > 0 || len(directiveConfigs) > 0 || strings.TrimSpace(promptID) != "" {
		config = map[string]any{}
	}
	if len(sources) > 0 {
		config["from_sources"] = append([]string(nil), sources...)
	}
	if len(goals) > 0 {
		config["goals"] = append([]string(nil), goals...)
	}
	if strings.TrimSpace(promptID) != "" {
		config["prompt_id"] = strings.TrimSpace(promptID)
	}
	if len(directives) > 0 {
		config["directives"] = append([]string(nil), directives...)
	}
	if len(directiveConfigs) > 0 {
		config["execution_items"] = directiveConfigs
	}
	if len(captures) > 0 {
		config["capture_bindings"] = summarizeCaptureBindings(captures)
	}
	if len(config) == 0 {
		config = nil
	}
	return sources, goals, directives, captures, promptID, config
}

func lowerAskDecl(decl *AskDecl, runIndex *int) (ExecutionStep, error) {
	if decl == nil {
		return ExecutionStep{}, fmt.Errorf("ask declaration is nil")
	}
	question, choices, choiceSource, captures, promptID, config, err := lowerAskItems(decl.Items)
	if err != nil {
		return ExecutionStep{}, err
	}
	stepID := fmt.Sprintf("ask.%d.%d.%d", decl.GetSpan().Start.Line, decl.GetSpan().Start.Column, *runIndex)
	step := ExecutionStep{
		ID:              stepID,
		Type:            "ask",
		Paradigm:        "euclo",
		Question:        question,
		Choices:         choices,
		ChoiceSource:    choiceSource,
		CaptureBindings: captures,
		Prompt:          question,
		PromptID:        promptID,
		Step:            ThoughtRecipeStep{ID: stepID, Type: "ask"},
	}
	step.Step.Parent.Paradigm = "euclo"
	step.Step.Prompt = question
	step.Step.PromptID = promptID
	step.Step.Type = "ask"
	step.Step.Config = config
	*runIndex = *runIndex + 1
	return step, nil
}

func lowerAskItems(items []AskItem) (question string, choices []string, choiceSource string, captures []CaptureBinding, promptID string, config map[string]any, err error) {
	var staticChoices []string
	for _, item := range items {
		switch node := item.(type) {
		case *QuestionClause:
			if strings.TrimSpace(question) != "" {
				return "", nil, "", nil, "", nil, fmt.Errorf("%s:%d:%d: duplicate ask question", node.GetSpan().Start.File, node.GetSpan().Start.Line, node.GetSpan().Start.Column)
			}
			if node.PromptID != nil {
				if ref := strings.TrimSpace(node.PromptID.ResolvedID); ref != "" {
					promptID = ref
				} else if ref := strings.TrimSpace(node.PromptID.Name.Value); ref != "" {
					promptID = ref
				}
				continue
			}
			question = strings.TrimSpace(node.Text.Value)
		case *ChoicesListClause:
			if len(staticChoices) > 0 || strings.TrimSpace(choiceSource) != "" {
				return "", nil, "", nil, "", nil, fmt.Errorf("%s:%d:%d: duplicate ask choices", node.GetSpan().Start.File, node.GetSpan().Start.Line, node.GetSpan().Start.Column)
			}
			for _, entry := range node.Items {
				if choice := strings.TrimSpace(askChoiceText(entry)); choice != "" {
					staticChoices = append(staticChoices, choice)
				}
			}
		case *ChoicesReferenceClause:
			if len(staticChoices) > 0 || strings.TrimSpace(choiceSource) != "" {
				return "", nil, "", nil, "", nil, fmt.Errorf("%s:%d:%d: duplicate ask choices", node.GetSpan().Start.File, node.GetSpan().Start.Line, node.GetSpan().Start.Column)
			}
			choiceSource = strings.TrimSpace(valueExprRaw(node.Source))
		case *CaptureBlock:
			captures = append(captures, LowerCaptureBindings(node)...)
		}
	}
	if strings.TrimSpace(question) == "" && strings.TrimSpace(promptID) == "" {
		return "", nil, "", nil, "", nil, fmt.Errorf("ask user requires a question")
	}
	if len(staticChoices) > 0 {
		choices = append([]string(nil), staticChoices...)
	}
	if len(captures) > 0 || strings.TrimSpace(question) != "" || len(choices) > 0 || strings.TrimSpace(choiceSource) != "" || strings.TrimSpace(promptID) != "" {
		config = map[string]any{
			"question": question,
		}
	}
	if strings.TrimSpace(promptID) != "" {
		config["prompt_id"] = strings.TrimSpace(promptID)
	}
	if len(choices) > 0 {
		config["choices"] = append([]string(nil), choices...)
	}
	if strings.TrimSpace(choiceSource) != "" {
		config["choice_source"] = choiceSource
	}
	if len(captures) > 0 {
		config["capture_bindings"] = summarizeCaptureBindings(captures)
	}
	return question, choices, choiceSource, captures, promptID, config, nil
}

func askChoiceText(expr ValueExpr) string {
	switch v := expr.(type) {
	case StringLiteral:
		return v.Value
	case *StringLiteral:
		return v.Value
	case Identifier:
		return v.Value
	case *Identifier:
		return v.Value
	default:
		return valueExprRaw(expr)
	}
}

func lowerPipelineDecl(decl *PipelineDecl, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan) (ExecutionStep, error) {
	if decl == nil {
		return ExecutionStep{}, fmt.Errorf("pipeline declaration is nil")
	}
	if plan == nil {
		return ExecutionStep{}, fmt.Errorf("execution plan is nil")
	}

	pipelineID := pipelineGroupID(decl)
	stages := make([]PipelineStageSpec, 0, len(decl.Stages))
	var totalSteps int
	for idx, stage := range decl.Stages {
		stageSteps, err := lowerPipelineExecutionItems(stage.Body, agents, runIndex, plan)
		if err != nil {
			return ExecutionStep{}, err
		}
		stages = append(stages, PipelineStageSpec{
			Name:  strings.TrimSpace(stage.Name.Value),
			Span:  stage.GetSpan(),
			Steps: stageSteps,
		})
		totalSteps += len(stageSteps)
		if strings.TrimSpace(stage.Name.Value) == "" {
			plan.Warnings = append(plan.Warnings, SemanticWarning{
				Span:    stage.GetSpan(),
				Message: fmt.Sprintf("pipeline stage %d has no name", idx+1),
			})
		}
		if len(stageSteps) == 0 {
			plan.Warnings = append(plan.Warnings, SemanticWarning{
				Span:    stage.GetSpan(),
				Message: fmt.Sprintf("pipeline stage %q has no executable steps", strings.TrimSpace(stage.Name.Value)),
			})
		}
	}
	if len(stages) == 0 {
		plan.Warnings = append(plan.Warnings, SemanticWarning{
			Span:    decl.GetSpan(),
			Message: "pipeline has no stages",
		})
	}
	step := ExecutionStep{
		ID:             pipelineID,
		Type:           "pipeline",
		Paradigm:       "euclo",
		PipelineStages: append([]PipelineStageSpec(nil), stages...),
		Step:           ThoughtRecipeStep{ID: pipelineID, Type: "pipeline"},
	}
	step.Step.Parent.Paradigm = "euclo"
	step.Step.Type = "pipeline"
	step.Step.Prompt = "pipeline " + pipelineID
	step.Step.Config = map[string]any{
		"pipeline_id": pipelineID,
		"stage_count": len(stages),
		"step_count":  totalSteps,
		"stages":      summarizePipelineStageSpecs(stages),
	}
	plan.Pipelines = append(plan.Pipelines, CompiledPipelineGroup{
		Group: &PipelineGroup{
			positioned: positioned{Span: decl.GetSpan()},
			ID:         pipelineID,
		},
		Stages: compiledPipelineStagesFromSpecs(stages),
	})
	return step, nil
}

func lowerPipelineExecutionItems(items []ExecutionItem, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan) ([]ExecutionStep, error) {
	steps := make([]ExecutionStep, 0)
	for _, item := range items {
		switch node := item.(type) {
		case *RunDecl:
			step, err := lowerAgentExecutionDecl("run", node.Agent, node.Items, agents, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DelegateDecl:
			step, err := lowerAgentExecutionDecl("delegate", node.Agent, node.Items, agents, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *RouteDecl:
			group, err := lowerRouteDecl(node, agents, runIndex, plan)
			if err != nil {
				return nil, err
			}
			plan.Routes = append(plan.Routes, *group)
		case *AskDecl:
			step, err := lowerAskDecl(node, runIndex)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *PipelineDecl:
			step, err := lowerPipelineDecl(node, agents, runIndex, plan)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DirectiveBlock:
			nested, err := lowerPipelineExecutionItems(node.Body, agents, runIndex, plan)
			if err != nil {
				return nil, err
			}
			steps = append(steps, nested...)
		}
	}
	return steps, nil
}

func summarizePipelineStageSpecs(stages []PipelineStageSpec) []string {
	if len(stages) == 0 {
		return nil
	}
	out := make([]string, 0, len(stages))
	for _, stage := range stages {
		name := strings.TrimSpace(stage.Name)
		if name == "" {
			name = "stage"
		}
		out = append(out, name)
	}
	return out
}

func compiledPipelineStagesFromSpecs(stages []PipelineStageSpec) []CompiledPipelineStage {
	if len(stages) == 0 {
		return nil
	}
	out := make([]CompiledPipelineStage, 0, len(stages))
	for _, stage := range stages {
		stageCopy := stage
		steps := make([]ExecutionStep, len(stageCopy.Steps))
		copy(steps, stageCopy.Steps)
		out = append(out, CompiledPipelineStage{
			Stage: &PipelineStage{
				positioned: positioned{Span: stageCopy.Span},
				Name:       Identifier{positioned: positioned{Span: stageCopy.Span}, Value: stageCopy.Name},
			},
			Steps: steps,
		})
	}
	return out
}

func pipelineGroupID(decl *PipelineDecl) string {
	span := decl.GetSpan()
	return fmt.Sprintf("pipeline.%d.%d", span.Start.Line, span.Start.Column)
}

func gatherLoweredFromDeclaration(node Declaration, plan *ExecutionPlan, runIndex *int) error {
	switch v := node.(type) {
	case *RunDecl:
		step, err := lowerAgentExecutionDecl("run", v.Agent, v.Items, plan.Agents, runIndex)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *DelegateDecl:
		step, err := lowerAgentExecutionDecl("delegate", v.Agent, v.Items, plan.Agents, runIndex)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *RouteDecl:
		group, err := lowerRouteDecl(v, plan.Agents, runIndex, plan)
		if err != nil {
			return err
		}
		plan.Routes = append(plan.Routes, *group)
	case *AskDecl:
		step, err := lowerAskDecl(v, runIndex)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *PipelineDecl:
		step, err := lowerPipelineDecl(v, plan.Agents, runIndex, plan)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	}
	return nil
}

func gatherLoweredFromExecutionItem(item ExecutionItem, plan *ExecutionPlan, runIndex *int) error {
	switch v := item.(type) {
	case *RunDecl:
		step, err := lowerAgentExecutionDecl("run", v.Agent, v.Items, plan.Agents, runIndex)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *DelegateDecl:
		step, err := lowerAgentExecutionDecl("delegate", v.Agent, v.Items, plan.Agents, runIndex)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *RouteDecl:
		group, err := lowerRouteDecl(v, plan.Agents, runIndex, plan)
		if err != nil {
			return err
		}
		plan.Routes = append(plan.Routes, *group)
	case *AskDecl:
		step, err := lowerAskDecl(v, runIndex)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *DirectiveBlock:
		for _, child := range v.Body {
			if err := gatherLoweredFromExecutionItem(child, plan, runIndex); err != nil {
				return err
			}
		}
	case *PipelineDecl:
		step, err := lowerPipelineDecl(v, plan.Agents, runIndex, plan)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	}
	return nil
}

func isSupportedAgentParadigm(paradigm string) bool {
	switch strings.TrimSpace(paradigm) {
	case "react", "planner", "htn", "reflection", "blackboard", "chainer", "pipeline", "rewoo", "goalcon":
		return true
	default:
		return false
	}
}

func rawValueExprList(values []ValueExpr) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if raw := strings.TrimSpace(valueExprRaw(value)); raw != "" {
			out = append(out, raw)
		}
	}
	return out
}

func predicateRaw(pred PredicateExpr) string {
	return strings.TrimSpace(pred.Raw)
}

func summarizeExecutionItems(items []ExecutionItem) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		span := item.GetSpan()
		out = append(out, fmt.Sprintf("%T@%d:%d", item, span.Start.Line, span.Start.Column))
	}
	return out
}

func summarizeCaptureBindings(bindings []CaptureBinding) []string {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if raw := strings.TrimSpace(valueExprRaw(binding.Source)); raw != "" {
			out = append(out, raw)
		}
	}
	return out
}
