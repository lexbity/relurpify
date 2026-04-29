package types

// Predicate is a satisfied or unsatisfied world-state fact.
type Predicate string

// GoalCondition is a conjunction of desired predicates.
type GoalCondition struct {
	Predicates  []Predicate `json:"predicates"`
	Description string      `json:"description"`
}

// WorldState tracks which predicates currently hold.
type WorldState struct {
	satisfied map[Predicate]bool
}

// Operator transforms world-state predicates.
type Operator struct {
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Preconditions []Predicate    `json:"preconditions"`
	Effects       []Predicate    `json:"effects"`
	DefaultParams map[string]any `json:"default_params"`
}

// NewWorldState creates an empty world state.
func NewWorldState() *WorldState {
	return &WorldState{satisfied: make(map[Predicate]bool)}
}

// Satisfy marks a predicate as true.
func (w *WorldState) Satisfy(p Predicate) {
	if w == nil {
		return
	}
	if w.satisfied == nil {
		w.satisfied = make(map[Predicate]bool)
	}
	w.satisfied[p] = true
}

// IsSatisfied reports whether a predicate is true.
func (w *WorldState) IsSatisfied(p Predicate) bool {
	if w == nil {
		return false
	}
	return w.satisfied[p]
}

// Clone creates an independent copy.
func (w *WorldState) Clone() *WorldState {
	clone := NewWorldState()
	if w == nil {
		return clone
	}
	for p, ok := range w.satisfied {
		if ok {
			clone.satisfied[p] = true
		}
	}
	return clone
}

func BuildPlan(description string, ops []*Operator) *core.Plan {
	steps := make([]core.PlanStep, 0, len(ops))
	deps := make(map[string][]string)
	for i, op := range ops {
		stepID := stepIDFor(i, op)
		params := make(map[string]any, len(op.DefaultParams))
		for k, v := range op.DefaultParams {
			params[k] = v
		}
		steps = append(steps, core.PlanStep{
			ID:          stepID,
			Description: op.Description,
			Tool:        op.Name,
			Params:      params,
		})
	}
	for i, op := range ops {
		if len(op.Preconditions) == 0 {
			continue
		}
		stepID := stepIDFor(i, op)
		for j := 0; j < i; j++ {
			if operatorSatisfiesAny(ops[j], op.Preconditions) {
				deps[stepID] = append(deps[stepID], stepIDFor(j, ops[j]))
			}
		}
	}
	return &core.Plan{
		Goal:         description,
		Steps:        steps,
		Dependencies: deps,
	}
}

func stepIDFor(index int, op *Operator) string {
	name := "op"
	if op != nil && op.Name != "" {
		name = op.Name
	}
	return "goalcon_step_" + twoDigit(index+1) + "_" + name
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+(n/10)%10)) + string(rune('0'+n%10))
}

func operatorSatisfiesAny(op *Operator, preconditions []Predicate) bool {
	if op == nil {
		return false
	}
	for _, effect := range op.Effects {
		for _, pre := range preconditions {
			if effect == pre {
				return true
			}
		}
	}
	return false
}

// OperatorRegistry indexes operators by effects.
type OperatorRegistry struct {
	operators   []*Operator
	effectIndex map[Predicate][]*Operator
}

// NewOperatorRegistry creates a new operator registry.
func NewOperatorRegistry() *OperatorRegistry {
	return &OperatorRegistry{
		operators:   make([]*Operator, 0),
		effectIndex: make(map[Predicate][]*Operator),
	}
}

// Register adds an operator to the registry.
func (r *OperatorRegistry) Register(op Operator) {
	if r == nil {
		return
	}
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
