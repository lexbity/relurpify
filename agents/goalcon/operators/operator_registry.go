package operators

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

// types.OperatorRegistry indexes operators by effects.
type types.OperatorRegistry struct {
	operators   []*types.Operator
	effectIndex map[types.Predicate][]*types.Operator
}

// Register adds an operator to the registry.
func (r *types.OperatorRegistry) Register(op types.Operator) {
	if r.effectIndex == nil {
		r.effectIndex = make(map[types.Predicate][]*types.Operator)
	}
	copyOp := op
	r.operators = append(r.operators, &copyOp)
	for _, effect := range copyOp.Effects {
		r.effectIndex[effect] = append(r.effectIndex[effect], &copyOp)
	}
}

// OperatorsSatisfying returns operators that can produce pred.
func (r *types.OperatorRegistry) OperatorsSatisfying(pred types.Predicate) []*types.Operator {
	if r == nil {
		return nil
	}
	ops := r.effectIndex[pred]
	return append([]*types.Operator(nil), ops...)
}

// All returns all registered operators.
func (r *types.OperatorRegistry) All() []*types.Operator {
	if r == nil {
		return nil
	}
	return append([]*types.Operator(nil), r.operators...)
}

// DefaultOperatorRegistry builds the default deterministic operator set.
func DefaultOperatorRegistry() *types.OperatorRegistry {
	registry := &types.OperatorRegistry{}
	registry.Register(types.Operator{Name: "ReadFile", Description: "Read file content", Effects: []types.Predicate{"file_content_known"}})
	registry.Register(types.Operator{Name: "SearchCode", Description: "Search for relevant code", Effects: []types.Predicate{"relevant_symbols_known"}})
	registry.Register(types.Operator{Name: "AnalyzeCode", Description: "Analyze code and derive edit plan", Preconditions: []types.Predicate{"file_content_known"}, Effects: []types.Predicate{"edit_plan_known"}})
	registry.Register(types.Operator{Name: "WriteFile", Description: "Write planned file modifications", Preconditions: []types.Predicate{"file_content_known", "edit_plan_known"}, Effects: []types.Predicate{"file_modified"}})
	registry.Register(types.Operator{Name: "RunTests", Description: "Run tests after modifications", Preconditions: []types.Predicate{"file_modified"}, Effects: []types.Predicate{"test_result_known"}})
	return registry
}
