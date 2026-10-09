package thoughtrecipe

import (
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// LowerDocument converts a source-level Euclo thoughtrecipe document into an
// execution plan that can be consumed by the existing thoughtrecipe runtime.
// capabilityScopeLookup resolves coordination metadata during lowering.
// Nil means the step scope is exactly the named capability.
type capabilityScopeLookup = CapabilityRegistryLookup

// LowerDocument lowers a document without a capability registry: standalone
// capability steps scope to exactly the named capability.
func LowerDocument(doc *ThoughtRecipeDocument) (*ExecutionPlan, error) {
	return LowerDocumentWithRegistry(doc, nil)
}

// LowerDocumentWithRegistry lowers a document with a capability lookup so
// manifest-declared coordination targets union into capability step scopes.
func LowerDocumentWithRegistry(doc *ThoughtRecipeDocument, lookup CapabilityRegistryLookup) (*ExecutionPlan, error) {
	if doc == nil {
		return nil, fmt.Errorf("thoughtrecipe document is nil")
	}

	plan := &ExecutionPlan{
		ThoughtRecipe: &surface.ThoughtRecipe{
			ID:   doc.Name,
			Name: doc.Name,
			Metadata: surface.ThoughtRecipeMetadata{
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
		plan.ToolScopes = lowerToolScopeFrames(trigger.ToolPolicies, "trigger")
	}

	for _, decl := range doc.Declarations {
		if node, ok := decl.(*AgentDecl); ok {
			paradigm := strings.TrimSpace(node.AgentType.Value)
			if !surface.IsSupported(surface.Paradigm(paradigm)) {
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
		if err := gatherLoweredFromDeclaration(decl, plan, &runIndex, lookup); err != nil {
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

func lowerRunItems(items []ExecutionItem) (sources []string, goals []string, directives []TypedDirective, captures []CaptureBinding, toolScopes []ToolScopeFrame, promptID string, capabilityPlan *CapabilityInvocationPlan, streamSpec *surface.ThoughtRecipeStreamSpec, config map[string]any, err error) {
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
		case *DirectiveClause, *DirectiveBlock:
			// Directives lower to typed values (D2). Nested directive children
			// of a block are carried in the typed Body; non-directive children
			// (do/capture/goal/from/run/delegate) are not part of the typed
			// directive tree — the route flattening that processes block
			// bodies lowers them to steps.
			directives = append(directives, lowerTypedDirective(item))
		case *CaptureBlock:
			captures = append(captures, LowerCaptureBindings(node)...)
		case *CapabilityInvocation:
			plan, err := LowerCapabilityInvocation(node)
			if err != nil {
				return nil, nil, nil, nil, nil, "", nil, nil, nil, err
			}
			if capabilityPlan != nil {
				return nil, nil, nil, nil, nil, "", nil, nil, nil, fmt.Errorf("%s:%d:%d: multiple direct capability invocations in one run block are not supported", node.GetSpan().Start.File, node.GetSpan().Start.Line, node.GetSpan().Start.Column)
			}
			capabilityPlan = plan
		case *ToolInvokePolicyDecl:
			toolScopes = append(toolScopes, lowerToolScopeFrame(node, "run"))
		case *StreamClause:
			spec := &surface.ThoughtRecipeStreamSpec{Mode: strings.TrimSpace(node.Mode)}
			if node.Query != nil {
				spec.QueryTemplate = node.Query.Value
			}
			if node.MaxTokens != nil {
				if n, convErr := strconv.Atoi(strings.TrimSpace(node.MaxTokens.Value)); convErr == nil {
					spec.MaxTokens = n
				}
			}
			streamSpec = spec
		}
	}
	// Config carries only the pre-existing explicit non-directive options
	// (clause-borne from/goal/prompt and tool-scope data). Directive payloads
	// travel in the typed Directives field, never in Config (D2).
	if len(sources) > 0 || len(goals) > 0 || len(toolScopes) > 0 || strings.TrimSpace(promptID) != "" {
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
	if len(toolScopes) > 0 {
		config["effective_tool_names"] = effectiveToolNames(toolScopes)
	}
	if len(config) == 0 {
		config = nil
	}
	return sources, goals, directives, captures, toolScopes, promptID, capabilityPlan, streamSpec, config, nil
}

// lowerTypedDirective lowers one directive clause or block into its typed
// form. Block children lower into Body as clause-tagged typed directives:
// nested directive clauses keep their own names, while structural children
// (from, goal, do, capture, run, delegate, stream) are tagged by their clause
// name so block payloads such as chainer `link from`/`capture` survive
// lowering. The typed Body is a faithful mirror of the block content.
func lowerTypedDirective(item ExecutionItem) TypedDirective {
	switch node := item.(type) {
	case *DirectiveClause:
		return TypedDirective{
			Name:     strings.TrimSpace(node.Name.Value),
			TextArgs: textArgsFromValueExprs(node.Arguments),
			Span:     node.GetSpan(),
		}
	case *DirectiveBlock:
		typed := TypedDirective{
			Name:      strings.TrimSpace(node.Name.Value),
			TextArgs:  textArgsFromValueExprs(node.Arguments),
			Predicate: node.Predicate,
			Span:      node.GetSpan(),
		}
		for _, child := range node.Body {
			typed.Body = append(typed.Body, lowerTypedBlockItem(child))
		}
		return typed
	default:
		return TypedDirective{}
	}
}

// lowerTypedBlockItem lowers one child execution item of a directive block
// into a clause-tagged TypedDirective inside the parent's Body.
func lowerTypedBlockItem(item ExecutionItem) TypedDirective {
	switch node := item.(type) {
	case *DirectiveClause, *DirectiveBlock:
		return lowerTypedDirective(item)
	case *FromClause:
		return TypedDirective{
			Name:     "from",
			TextArgs: []string{strings.TrimSpace(valueExprRaw(node.Source))},
			Span:     node.GetSpan(),
		}
	case *GoalClause:
		return TypedDirective{
			Name:     "goal",
			TextArgs: []string{strings.TrimSpace(node.Text.Value)},
			Span:     node.GetSpan(),
		}
	case *StreamClause:
		text := ""
		if node.Query != nil {
			text = strings.TrimSpace(node.Query.Value)
		}
		return TypedDirective{
			Name:     "stream",
			TextArgs: []string{text},
			Span:     node.GetSpan(),
		}
	case *CapabilityInvocation:
		return TypedDirective{
			Name:     "do",
			TextArgs: []string{strings.TrimSpace(node.Namespace.Value) + ":" + strings.TrimSpace(node.Capability.Value)},
			Span:     node.GetSpan(),
		}
	case *CaptureBlock:
		var args []string
		for _, binding := range node.Bindings {
			args = append(args, captureBindingRaw(binding))
		}
		return TypedDirective{
			Name:     "capture",
			TextArgs: args,
			Span:     node.GetSpan(),
		}
	case *RunDecl:
		return TypedDirective{
			Name:     "run",
			TextArgs: []string{strings.TrimSpace(node.Agent.Value)},
			Span:     node.GetSpan(),
		}
	case *DelegateDecl:
		return TypedDirective{
			Name:     "delegate",
			TextArgs: []string{strings.TrimSpace(node.Agent.Value)},
			Span:     node.GetSpan(),
		}
	default:
		return TypedDirective{}
	}
}

func captureBindingRaw(binding CaptureBinding) string {
	source := strings.TrimSpace(valueExprRaw(binding.Source))
	dest := strings.TrimSpace(valueExprRaw(binding.Destination))
	if source == "" && dest == "" {
		return ""
	}
	if dest == "" {
		return source
	}
	return source + " -> " + dest
}

func textArgsFromValueExprs(args []ValueExpr) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if raw := strings.TrimSpace(valueExprRaw(arg)); raw != "" {
			out = append(out, raw)
		}
	}
	return out
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
		Kind:            StepKindAsk,
		Paradigm:        "euclo",
		Question:        question,
		Choices:         choices,
		ChoiceSource:    choiceSource,
		CaptureBindings: captures,
		Prompt:          question,
		PromptID:        promptID,
		Config:          config,
	}
	*runIndex++
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

func lowerPipelineDecl(decl *PipelineDecl, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan, lookup capabilityScopeLookup) (ExecutionStep, error) {
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
		stageSteps, err := lowerPipelineExecutionItems(stage.Body, agents, runIndex, plan, lookup)
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
		Kind:           StepKindPipelineStage,
		Paradigm:       "euclo",
		PipelineStages: append([]PipelineStageSpec(nil), stages...),
		Config: map[string]any{
			"pipeline_id": pipelineID,
			"stage_count": len(stages),
			"step_count":  totalSteps,
			"stages":      summarizePipelineStageSpecs(stages),
		},
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

func lowerPipelineExecutionItems(items []ExecutionItem, agents map[string]AgentBinding, runIndex *int, plan *ExecutionPlan, lookup capabilityScopeLookup) ([]ExecutionStep, error) {
	steps := make([]ExecutionStep, 0)
	for _, item := range items {
		switch node := item.(type) {
		case *RunDecl:
			step, err := lowerAgentExecutionDecl(StepKindRun, node.Agent, node.Items, agents, runIndex, plan.ToolScopes)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DelegateDecl:
			step, err := lowerAgentExecutionDecl(StepKindDelegate, node.Agent, node.Items, agents, runIndex, plan.ToolScopes)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *RouteDecl:
			group, err := lowerRouteDecl(node, agents, runIndex, plan, plan.ToolScopes, lookup)
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
			step, err := lowerPipelineDecl(node, agents, runIndex, plan, lookup)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *CapabilityInvocation:
			step, err := lowerCapabilityExecutionDecl(node, runIndex, lookup)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DirectiveBlock:
			nested, err := lowerPipelineExecutionItems(node.Body, agents, runIndex, plan, lookup)
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

func gatherLoweredFromDeclaration(node Declaration, plan *ExecutionPlan, runIndex *int, lookup capabilityScopeLookup) error {
	switch v := node.(type) {
	case *RunDecl:
		step, err := lowerAgentExecutionDecl(StepKindRun, v.Agent, v.Items, plan.Agents, runIndex, plan.ToolScopes)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *DelegateDecl:
		step, err := lowerAgentExecutionDecl(StepKindDelegate, v.Agent, v.Items, plan.Agents, runIndex, plan.ToolScopes)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	case *RouteDecl:
		group, err := lowerRouteDecl(v, plan.Agents, runIndex, plan, plan.ToolScopes, lookup)
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
		step, err := lowerPipelineDecl(v, plan.Agents, runIndex, plan, lookup)
		if err != nil {
			return err
		}
		plan.Steps = append(plan.Steps, step)
	}
	return nil
}

func lowerToolScopeFrames(policies []ToolInvokePolicyDecl, scopeKind string) []ToolScopeFrame {
	if len(policies) == 0 {
		return nil
	}
	frames := make([]ToolScopeFrame, 0, len(policies))
	for i := range policies {
		frames = append(frames, lowerToolScopeFrame(&policies[i], scopeKind))
	}
	return frames
}

func lowerToolScopeFrame(policy *ToolInvokePolicyDecl, scopeKind string) ToolScopeFrame {
	if policy == nil {
		return ToolScopeFrame{ScopeKind: scopeKind}
	}
	names := toolNamesFromPolicy(policy)
	return ToolScopeFrame{
		ScopeKind: scopeKind,
		ToolNames: append([]string(nil), names...),
		Span:      policy.GetSpan(),
	}
}

func toolNamesFromPolicy(policy *ToolInvokePolicyDecl) []string {
	if policy == nil {
		return nil
	}
	if len(policy.ResolvedToolNames) > 0 {
		return append([]string(nil), policy.ResolvedToolNames...)
	}
	if policy.ToolNames == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(policy.ToolNames.Entries))
	out := make([]string, 0, len(policy.ToolNames.Entries))
	for _, entry := range policy.ToolNames.Entries {
		name := canonicalToolNameEntry(entry)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func copyToolScopeFrames(frames []ToolScopeFrame) []ToolScopeFrame {
	if len(frames) == 0 {
		return nil
	}
	out := make([]ToolScopeFrame, len(frames))
	for i := range frames {
		out[i] = ToolScopeFrame{
			ScopeKind: frames[i].ScopeKind,
			ToolNames: append([]string(nil), frames[i].ToolNames...),
			Span:      frames[i].Span,
		}
	}
	return out
}

func appendToolScopeFrames(base []ToolScopeFrame, extra ...ToolScopeFrame) []ToolScopeFrame {
	if len(extra) == 0 {
		return base
	}
	out := copyToolScopeFrames(base)
	for _, frame := range extra {
		out = append(out, ToolScopeFrame{
			ScopeKind: frame.ScopeKind,
			ToolNames: append([]string(nil), frame.ToolNames...),
			Span:      frame.Span,
		})
	}
	return out
}

func effectiveToolNames(frames []ToolScopeFrame) []string {
	if len(frames) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, frame := range frames {
		for _, name := range frame.ToolNames {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}
