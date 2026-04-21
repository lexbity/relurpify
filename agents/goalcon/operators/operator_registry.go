package operators

import (
	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
)

// DefaultOperatorRegistry builds the default deterministic operator set.
func DefaultOperatorRegistry() *types.OperatorRegistry {
	registry := types.NewOperatorRegistry()
	registry.Register(types.Operator{Name: "ReadFile", Description: "Read file content", Effects: []types.Predicate{"file_content_known"}})
	registry.Register(types.Operator{Name: "SearchCode", Description: "Search for relevant code", Effects: []types.Predicate{"relevant_symbols_known"}})
	registry.Register(types.Operator{Name: "AnalyzeCode", Description: "Analyze code and derive edit plan", Preconditions: []types.Predicate{"file_content_known"}, Effects: []types.Predicate{"edit_plan_known"}})
	registry.Register(types.Operator{Name: "WriteFile", Description: "Write planned file modifications", Preconditions: []types.Predicate{"file_content_known", "edit_plan_known"}, Effects: []types.Predicate{"file_modified"}})
	registry.Register(types.Operator{Name: "RunTests", Description: "Run tests after modifications", Preconditions: []types.Predicate{"file_modified"}, Effects: []types.Predicate{"test_result_known"}})
	return registry
}
