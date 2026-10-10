package thoughtrecipe

import (
	"fmt"
	"strings"
)

// revise_lowering.go lowers a reflection `revise when <predicate>:` block into
// a typed Predicate and executable body steps (Wave 3 D5). The body runs
// in-process through ExecuteDelegateCore; its steps inherit the reflection
// step's tool scope so a revise body can never exceed what the step declared.

// lowerReviseDirective finds the reflection `revise` block in a run block's
// items and lowers its predicate and body. Absent a revise block it returns
// nil, nil, nil.
func lowerReviseDirective(items []ExecutionItem, agents map[string]AgentBinding, index *int, inheritedToolScopes []ToolScopeFrame) ([]ExecutionStep, *Predicate, error) {
	for _, item := range items {
		block, ok := item.(*DirectiveBlock)
		if !ok || strings.TrimSpace(block.Name.Value) != "revise" {
			continue
		}
		if block.Predicate == nil {
			return nil, nil, reviseSpanError(block, "revise requires a when predicate")
		}
		pred, err := NormalizeRoutePredicate(*block.Predicate)
		if err != nil {
			return nil, nil, err
		}
		steps, err := lowerReviseBodyItems(block.Body, agents, index, inheritedToolScopes)
		if err != nil {
			return nil, nil, err
		}
		if len(steps) == 0 {
			return nil, nil, reviseSpanError(block, "revise requires a body of run/delegate items")
		}
		return steps, pred, nil
	}
	return nil, nil, nil
}

// lowerReviseBodyItems lowers the allowed revise-body items (run/delegate/ask/
// capability, plus flattened nested directive blocks) into executable steps.
// The contract admits run/delegate; ask/capability are defensively accepted
// and any other clause is a load error naming its type and span.
func lowerReviseBodyItems(body []ExecutionItem, agents map[string]AgentBinding, index *int, inheritedToolScopes []ToolScopeFrame) ([]ExecutionStep, error) {
	var steps []ExecutionStep
	for _, item := range body {
		switch node := item.(type) {
		case *RunDecl:
			step, err := lowerAgentExecutionDecl(StepKindRun, node.Agent, node.Items, agents, index, inheritedToolScopes)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DelegateDecl:
			step, err := lowerAgentExecutionDecl(StepKindDelegate, node.Agent, node.Items, agents, index, inheritedToolScopes)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *AskDecl:
			step, err := lowerAskDecl(node, index)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *CapabilityInvocation:
			step, err := lowerCapabilityExecutionDecl(node, index, nil)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
		case *DirectiveBlock:
			// Flatten allowed nested items; a nested revise is not expressible
			// (the contract order rule admits at most one revise per block).
			nested, err := lowerReviseBodyItems(node.Body, agents, index, inheritedToolScopes)
			if err != nil {
				return nil, err
			}
			steps = append(steps, nested...)
		default:
			return nil, reviseSpanError(node, fmt.Sprintf("unsupported item in revise body: %T", item))
		}
	}
	return steps, nil
}

func reviseSpanError(node Node, message string) error {
	span := node.GetSpan()
	return fmt.Errorf("%s:%d:%d: %s", span.Start.File, span.Start.Line, span.Start.Column, message)
}
