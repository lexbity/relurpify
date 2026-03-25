package runtime

import (
	"sort"

	"github.com/lexcodex/relurpify/framework/core"
)

// MethodLibrary stores the set of known decomposition methods and provides
// best-match lookup for incoming tasks.
type MethodLibrary struct {
	methods []Method
}

// NewMethodLibrary returns a library pre-populated with the default built-in
// methods.
func NewMethodLibrary() *MethodLibrary {
	ml := &MethodLibrary{}
	ml.registerDefaults()
	return ml
}

// Register adds a method. If a method with the same Name already exists it is
// replaced, allowing manifest-declared overrides to supersede code defaults.
func (ml *MethodLibrary) Register(m Method) {
	for i, existing := range ml.methods {
		if existing.Name == m.Name {
			ml.methods[i] = m
			return
		}
	}
	ml.methods = append(ml.methods, m)
}

// Find returns the highest-priority method that matches task. Returns nil when
// no method matches.
func (ml *MethodLibrary) Find(task *core.Task) *Method {
	if task == nil {
		return nil
	}
	var candidates []Method
	for _, m := range ml.methods {
		if m.TaskType != task.Type {
			continue
		}
		if m.Precondition != nil && !m.Precondition(task) {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	result := candidates[0]
	return &result
}

// All returns a copy of all registered methods.
func (ml *MethodLibrary) All() []Method {
	out := make([]Method, len(ml.methods))
	copy(out, ml.methods)
	return out
}

// FindAll returns all methods matching the given task, sorted by priority (highest first).
func (ml *MethodLibrary) FindAll(task *core.Task) []Method {
	if task == nil {
		return nil
	}
	var candidates []Method
	for _, m := range ml.methods {
		if m.TaskType != task.Type {
			continue
		}
		if m.Precondition != nil && !m.Precondition(task) {
			continue
		}
		candidates = append(candidates, m)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].Name < candidates[j].Name
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	return candidates
}

// FindByName returns a method by its name, or nil if not found.
func (ml *MethodLibrary) FindByName(name string) *Method {
	for i, m := range ml.methods {
		if m.Name == name {
			return &ml.methods[i]
		}
	}
	return nil
}

// FindResolved returns the best matching method resolved into a ResolvedMethod.
func (ml *MethodLibrary) FindResolved(task *core.Task) *ResolvedMethod {
	method := ml.Find(task)
	if method == nil {
		return nil
	}
	resolved := ResolveMethod(*method)
	return &resolved
}

// registerDefaults installs the built-in method recipes.
func (ml *MethodLibrary) registerDefaults() {
	ml.methods = []Method{
		{
			Name:     "code-new",
			TaskType: core.TaskTypeCodeGeneration,
			Priority: 0,
			Subtasks: []SubtaskSpec{
				{Name: "explore", Type: core.TaskTypeAnalysis, Instruction: "Explore the codebase to understand context for: {{.Instruction}}"},
				{Name: "plan", Type: core.TaskTypePlanning, Instruction: "Plan the implementation for: {{.Instruction}}", DependsOn: []string{"explore"}},
				{Name: "code", Type: core.TaskTypeCodeGeneration, Instruction: "Implement: {{.Instruction}}", DependsOn: []string{"plan"}},
				{Name: "verify", Type: core.TaskTypeAnalysis, Instruction: "Verify the implementation for: {{.Instruction}}", DependsOn: []string{"code"}},
			},
		},
		{
			Name:     "code-fix",
			TaskType: core.TaskTypeCodeModification,
			Priority: 0,
			Subtasks: []SubtaskSpec{
				{Name: "explore", Type: core.TaskTypeAnalysis, Instruction: "Explore the codebase to understand the problem for: {{.Instruction}}"},
				{Name: "diagnose", Type: core.TaskTypeAnalysis, Instruction: "Diagnose the root cause for: {{.Instruction}}", DependsOn: []string{"explore"}},
				{Name: "plan", Type: core.TaskTypePlanning, Instruction: "Plan the fix for: {{.Instruction}}", DependsOn: []string{"diagnose"}},
				{Name: "code", Type: core.TaskTypeCodeModification, Instruction: "Apply the fix for: {{.Instruction}}", DependsOn: []string{"plan"}},
				{Name: "verify", Type: core.TaskTypeAnalysis, Instruction: "Verify the fix for: {{.Instruction}}", DependsOn: []string{"code"}},
			},
		},
		{
			Name:     "code-review",
			TaskType: core.TaskTypeReview,
			Priority: 0,
			Subtasks: []SubtaskSpec{
				{Name: "explore", Type: core.TaskTypeAnalysis, Instruction: "Explore the code to be reviewed for: {{.Instruction}}"},
				{Name: "analyze", Type: core.TaskTypeAnalysis, Instruction: "Analyze for issues in: {{.Instruction}}", DependsOn: []string{"explore"}},
				{Name: "report", Type: core.TaskTypeReview, Instruction: "Produce a review report for: {{.Instruction}}", DependsOn: []string{"analyze"}},
			},
		},
		{
			Name:     "explain",
			TaskType: core.TaskTypeAnalysis,
			Priority: 0,
			Subtasks: []SubtaskSpec{
				{Name: "explore", Type: core.TaskTypeAnalysis, Instruction: "Explore the relevant code for: {{.Instruction}}"},
				{Name: "summarize", Type: core.TaskTypeAnalysis, Instruction: "Summarize and explain: {{.Instruction}}", DependsOn: []string{"explore"}},
			},
		},
		{
			Name:     "plan-task",
			TaskType: core.TaskTypePlanning,
			Priority: 0,
			Subtasks: []SubtaskSpec{
				{Name: "explore", Type: core.TaskTypeAnalysis, Instruction: "Explore context for planning: {{.Instruction}}"},
				{Name: "plan", Type: core.TaskTypePlanning, Instruction: "Produce a detailed plan for: {{.Instruction}}", DependsOn: []string{"explore"}},
			},
		},
	}
}
