package goalcon

// OperatorRegistry indexes operators by effects.
type OperatorRegistry struct {
	operators   []*Operator
	effectIndex map[Predicate][]*Operator
}

// Register adds an operator to the registry.
func (r *OperatorRegistry) Register(op Operator) {
	if r.effectIndex == nil {
		r.effectIndex = make(map[Predicate][]*Operator)
	}
	copyOp := op
	r.operators = append(r.operators, &copyOp)
	for _, effect := range copyOp.Effects {
		r.effectIndex[effect] = append(r.effectIndex[effect], &copyOp)
	}
}

// OperatorsSatisfying returns operators that can produce pred.
func (r *OperatorRegistry) OperatorsSatisfying(pred Predicate) []*Operator {
	if r == nil {
		return nil
	}
	ops := r.effectIndex[pred]
	return append([]*Operator(nil), ops...)
}

// All returns all registered operators.
func (r *OperatorRegistry) All() []*Operator {
	if r == nil {
		return nil
	}
	return append([]*Operator(nil), r.operators...)
}

// DefaultOperatorRegistry builds the default deterministic operator set.
func DefaultOperatorRegistry() *OperatorRegistry {
	registry := &OperatorRegistry{}
	registry.Register(Operator{Name: "ReadFile", Description: "Read file content", Effects: []Predicate{"file_content_known"}})
	registry.Register(Operator{Name: "SearchCode", Description: "Search for relevant code", Effects: []Predicate{"relevant_symbols_known"}})
	registry.Register(Operator{Name: "AnalyzeCode", Description: "Analyze code and derive edit plan", Preconditions: []Predicate{"file_content_known"}, Effects: []Predicate{"edit_plan_known"}})
	registry.Register(Operator{Name: "WriteFile", Description: "Write planned file modifications", Preconditions: []Predicate{"file_content_known", "edit_plan_known"}, Effects: []Predicate{"file_modified"}})
	registry.Register(Operator{Name: "RunTests", Description: "Run tests after modifications", Preconditions: []Predicate{"file_modified"}, Effects: []Predicate{"test_result_known"}})
	return registry
}
