package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
)

func validSubtask(name string) SubtaskSpec {
	return SubtaskSpec{Name: name, Type: execution.TaskTypeExplain, Instruction: "do " + name}
}

func validMethod() Method {
	return Method{
		Name:     "sample",
		TaskType: execution.TaskTypeExplain,
		Subtasks: []SubtaskSpec{
			validSubtask("first"),
			{Name: "second", Type: execution.TaskTypePlan, DependsOn: []string{"first"}},
		},
	}
}

func TestSubtaskSpecValidate(t *testing.T) {
	t.Run("valid subtask passes", func(t *testing.T) {
		require.NoError(t, validSubtask("explore").Validate())
	})
	t.Run("missing name", func(t *testing.T) {
		subtask := validSubtask("  ")
		require.EqualError(t, subtask.Validate(), "htn: subtask name required")
	})
	t.Run("missing type", func(t *testing.T) {
		subtask := validSubtask("explore")
		subtask.Type = ""
		require.EqualError(t, subtask.Validate(), `htn: subtask "explore" task type required`)
	})
	t.Run("interior executor whitespace rejected", func(t *testing.T) {
		for _, executor := range []string{"react pipeline", "react\tn", "react\nx"} {
			subtask := validSubtask("explore")
			subtask.Executor = executor
			require.ErrorContains(t, subtask.Validate(), "must not contain whitespace")
		}
	})
	t.Run("trailing executor whitespace trimmed before the check", func(t *testing.T) {
		subtask := validSubtask("explore")
		subtask.Executor = "react\r"
		require.NoError(t, subtask.Validate())
	})
	t.Run("executor empty or plain accepted", func(t *testing.T) {
		subtask := validSubtask("explore")
		subtask.Executor = ""
		require.NoError(t, subtask.Validate())
		subtask.Executor = "pipeline"
		require.NoError(t, subtask.Validate())
	})
}

func TestMethodValidate(t *testing.T) {
	t.Run("valid method passes", func(t *testing.T) {
		require.NoError(t, validMethod().Validate())
	})
	t.Run("missing name", func(t *testing.T) {
		method := validMethod()
		method.Name = " "
		require.EqualError(t, method.Validate(), "htn: method name required")
	})
	t.Run("missing task type", func(t *testing.T) {
		method := validMethod()
		method.TaskType = ""
		require.EqualError(t, method.Validate(), `htn: method "sample" task type required`)
	})
	t.Run("no subtasks", func(t *testing.T) {
		method := validMethod()
		method.Subtasks = nil
		require.EqualError(t, method.Validate(), `htn: method "sample" must declare at least one subtask`)
	})
	t.Run("invalid subtask surfaces subtask error", func(t *testing.T) {
		method := validMethod()
		method.Subtasks[0].Type = ""
		require.EqualError(t, method.Validate(), `htn: subtask "first" task type required`)
	})
	t.Run("duplicate subtask names", func(t *testing.T) {
		method := validMethod()
		method.Subtasks[1].Name = "first"
		method.Subtasks[1].DependsOn = nil
		require.EqualError(t, method.Validate(), `htn: method "sample" contains duplicate subtask name "first"`)
	})
	t.Run("self dependency", func(t *testing.T) {
		method := validMethod()
		method.Subtasks[0].DependsOn = []string{"first"}
		require.EqualError(t, method.Validate(), `htn: method "sample" subtask "first" cannot depend on itself`)
	})
	t.Run("unknown dependency", func(t *testing.T) {
		method := validMethod()
		method.Subtasks[1].DependsOn = []string{"ghost"}
		require.EqualError(t, method.Validate(), `htn: method "sample" subtask "second" depends on unknown subtask "ghost"`)
	})
}

func TestResolvedMethodValidate(t *testing.T) {
	// Fresh resolved state per subtest: Method is a shared pointer, so broken
	// variants must not leak mutations across cases.
	newResolved := func() ResolvedMethod {
		method := validMethod()
		return ResolveMethod(method)
	}

	t.Run("valid resolved method passes", func(t *testing.T) {
		require.NoError(t, newResolved().Validate())
	})
	t.Run("missing method", func(t *testing.T) {
		broken := newResolved()
		broken.Method = nil
		require.EqualError(t, broken.Validate(), "htn: resolved method requires method")
	})
	t.Run("invalid inner method surfaces error", func(t *testing.T) {
		broken := newResolved()
		broken.Method.Name = ""
		require.ErrorContains(t, broken.Validate(), "method name required")
	})
	t.Run("spec name mismatch", func(t *testing.T) {
		broken := newResolved()
		broken.Spec.Name = "other"
		require.ErrorContains(t, broken.Validate(), `spec name "other" does not match method name "sample"`)
	})
	t.Run("spec name empty", func(t *testing.T) {
		broken := newResolved()
		broken.Spec.Name = ""
		require.EqualError(t, broken.Validate(), "htn: resolved method spec name required")
	})
	t.Run("spec task type empty", func(t *testing.T) {
		broken := newResolved()
		broken.Spec.TaskType = ""
		require.EqualError(t, broken.Validate(), `htn: resolved method "sample" spec task type required`)
	})
	t.Run("spec task type mismatch", func(t *testing.T) {
		broken := newResolved()
		broken.Spec.TaskType = execution.TaskTypePlan
		require.ErrorContains(t, broken.Validate(), `spec task type "plan" does not match method task type "explain"`)
	})
	t.Run("operator count mismatch", func(t *testing.T) {
		broken := newResolved()
		broken.Operators = broken.Operators[:1]
		require.ErrorContains(t, broken.Validate(), "operator count 1 does not match subtask count 2")
	})
	t.Run("spec operator count mismatch", func(t *testing.T) {
		broken := newResolved()
		broken.Spec.OperatorCount = 5
		require.ErrorContains(t, broken.Validate(), "spec operator count 5 does not match operator count 2")
	})
	t.Run("operator name empty", func(t *testing.T) {
		broken := newResolved()
		broken.Operators[0].Name = "  "
		require.ErrorContains(t, broken.Validate(), "operator 0 name required")
	})
	t.Run("operator name mismatch", func(t *testing.T) {
		broken := newResolved()
		broken.Operators[0].Name = "wrong"
		require.ErrorContains(t, broken.Validate(), `operator "wrong" does not match subtask "first"`)
	})
	t.Run("operator task type empty", func(t *testing.T) {
		broken := newResolved()
		broken.Operators[0].TaskType = ""
		require.EqualError(t, broken.Validate(), `htn: resolved method "sample" operator "first" task type required`)
	})
	t.Run("operator task type mismatch", func(t *testing.T) {
		broken := newResolved()
		broken.Operators[0].TaskType = execution.TaskTypePlan
		require.ErrorContains(t, broken.Validate(), `does not match subtask type "explain"`)
	})
	t.Run("operator executor empty", func(t *testing.T) {
		broken := newResolved()
		broken.Operators[0].Executor = ""
		require.ErrorContains(t, broken.Validate(), `operator "first" executor required`)
	})
}

func TestExecutorConstants(t *testing.T) {
	assert.Equal(t, "react", ExecutorReact)
	assert.Equal(t, "pipeline", ExecutorPipeline)
	assert.Equal(t, "htn", ExecutorHTN)
}

func TestClassifyTask(t *testing.T) {
	t.Run("nil task falls back to explain", func(t *testing.T) {
		assert.Equal(t, execution.TaskTypeExplain, ClassifyTask(nil))
	})
	t.Run("existing type returned unchanged", func(t *testing.T) {
		task := &execution.Task{Type: string(execution.TaskTypeAnalysis)}
		assert.Equal(t, execution.TaskTypeAnalysis, ClassifyTask(task))
	})
	t.Run("keyword classification", func(t *testing.T) {
		cases := []struct {
			instruction string
			want        execution.TaskType
		}{
			{"please review this PR", execution.TaskTypeReview},
			{"check the implementation", execution.TaskTypeReview},
			{"inspect module boundaries", execution.TaskTypeReview},
			{"audit the security model", execution.TaskTypeReview},
			{"evaluate correctness", execution.TaskTypeReview},
			{"plan the migration", execution.TaskTypePlan},
			{"design a cache", execution.TaskTypePlan},
			{"outline the approach", execution.TaskTypePlan},
			{"define a strategy", execution.TaskTypePlan},
			{"create a new module", execution.TaskTypeCodeGeneration},
			{"generate fixtures", execution.TaskTypeCodeGeneration},
			{"write docs", execution.TaskTypeCodeGeneration},
			{"implement the parser", execution.TaskTypeCodeGeneration},
			{"add a feature", execution.TaskTypeCodeGeneration},
			{"build the pipeline", execution.TaskTypeCodeGeneration},
			{"new entry point", execution.TaskTypeCodeGeneration},
			{"fix the bug", execution.TaskTypeExecute},
			{"refactor the store", execution.TaskTypeExecute},
			{"update dependencies", execution.TaskTypeExecute},
			{"change the layout", execution.TaskTypeExecute},
			{"modify behavior", execution.TaskTypeExecute},
			{"patch the leak", execution.TaskTypeExecute},
			{"correct the typo", execution.TaskTypeExecute},
			{"improve throughput", execution.TaskTypeExecute},
			{"what does this do", execution.TaskTypeExplain},
			{"", execution.TaskTypeExplain},
		}
		for _, tc := range cases {
			got := ClassifyTask(&execution.Task{Instruction: tc.instruction})
			assert.Equal(t, tc.want, got, "instruction %q", tc.instruction)
		}
	})
	t.Run("case insensitive", func(t *testing.T) {
		assert.Equal(t, execution.TaskTypeReview, ClassifyTask(&execution.Task{Instruction: "REVIEW THIS"}))
	})
	t.Run("review outranks later keyword groups", func(t *testing.T) {
		assert.Equal(t, execution.TaskTypeReview, ClassifyTask(&execution.Task{Instruction: "review and fix"}))
	})
}

func TestResolveMethod(t *testing.T) {
	method := Method{
		Name:     "full",
		TaskType: execution.TaskTypeExecute,
		Priority: 7,
		Subtasks: []SubtaskSpec{
			{Name: "a", Type: execution.TaskTypeExplain, Instruction: "first {{.Instruction}}"},
			{Name: "b", Type: execution.TaskTypePlan, Executor: ExecutorPipeline, DependsOn: []string{"a"}},
			{Name: "c", Type: execution.TaskTypeExecute, Executor: ExecutorReact},
		},
	}
	resolved := ResolveMethod(method)

	assert.Equal(t, "full", resolved.Spec.Name)
	assert.Equal(t, execution.TaskTypeExecute, resolved.Spec.TaskType)
	assert.Equal(t, 7, resolved.Spec.Priority)
	assert.Equal(t, 3, resolved.Spec.OperatorCount)
	assert.Equal(t, 3, resolved.Spec.SubtaskCount)
	require.Len(t, resolved.Operators, 3)

	// Default executor applied.
	assert.Equal(t, ExecutorReact, resolved.Operators[0].Executor)
	assert.Equal(t, ExecutorPipeline, resolved.Operators[1].Executor)
	assert.Equal(t, ExecutorReact, resolved.Operators[2].Executor, "explicit react preserved")

	assert.Equal(t, []string{"a"}, resolved.Operators[1].DependsOn)

	// Each operator requires its dispatch target capability.
	for i, operator := range resolved.Operators {
		require.NotEmpty(t, operator.RequiredCapabilities)
		assert.Equal(t, agentspec.CapabilityKindTool, operator.RequiredCapabilities[0].Kind)
		assert.Equal(t, "agent:"+operator.Executor, operator.RequiredCapabilities[0].Name, "operator %d", i)
	}

	// Method-level selectors deduplicated (agent:react appears twice).
	selectors := resolved.Spec.RequiredCapabilities
	require.Len(t, selectors, 2)
	names := []string{selectors[0].Name, selectors[1].Name}
	assert.ElementsMatch(t, []string{"agent:react", "agent:pipeline"}, names)

	// Resolved method stays consistent with its method.
	require.NoError(t, resolved.Validate())
}

func TestResolveMethodEmpty(t *testing.T) {
	resolved := ResolveMethod(Method{Name: "empty", TaskType: execution.TaskTypeExplain})
	assert.Empty(t, resolved.Operators)
	assert.Empty(t, resolved.Spec.RequiredCapabilities, "no selectors without subtasks")
}

func TestDedupeSelectors(t *testing.T) {
	assert.Nil(t, dedupeSelectors(nil))
	selectors := []agentspec.CapabilitySelector{
		{Kind: agentspec.CapabilityKindTool, Name: "a"},
		{Kind: agentspec.CapabilityKindTool, Name: "a"},
		{Kind: agentspec.CapabilityKindPrompt, Name: "a"},
		{Kind: agentspec.CapabilityKindTool, Name: "b"},
	}
	deduped := dedupeSelectors(selectors)
	require.Len(t, deduped, 3)
	assert.Equal(t, "a", deduped[0].Name)
	assert.Equal(t, agentspec.CapabilityKindPrompt, deduped[1].Kind)
	assert.Equal(t, "b", deduped[2].Name)
}

func TestMethodLibraryDefaultsAreValid(t *testing.T) {
	library := NewMethodLibrary()
	methods := library.All()
	require.NotEmpty(t, methods)
	for _, method := range methods {
		require.NoError(t, method.Validate(), "default method %s", method.Name)
		resolved := ResolveMethod(method)
		require.NoError(t, resolved.Validate())
	}
	names := make([]string, 0, len(methods))
	for _, method := range methods {
		names = append(names, method.Name)
	}
	assert.ElementsMatch(t, []string{"code-new", "code-fix", "code-review", "explain", "plan-task"}, names)
}

func TestMethodLibraryRegisterReplacesByName(t *testing.T) {
	library := NewMethodLibrary()
	before := len(library.All())
	library.Register(Method{
		Name:     "code-new",
		TaskType: execution.TaskTypeCodeGeneration,
		Priority: 9,
		Subtasks: []SubtaskSpec{validSubtask("only")},
	})
	after := library.All()
	assert.Len(t, after, before, "same-name method replaced, not appended")

	found := library.FindByName("code-new")
	require.NotNil(t, found)
	assert.Equal(t, 9, found.Priority)
	assert.Len(t, found.Subtasks, 1)

	library.Register(Method{
		Name:     "brand-new",
		TaskType: execution.TaskTypeAnalysis,
		Subtasks: []SubtaskSpec{validSubtask("solo")},
	})
	assert.Len(t, library.All(), before+1)
	assert.NotNil(t, library.FindByName("brand-new"))
	assert.Nil(t, library.FindByName("missing"))
}

func TestMethodLibraryFind(t *testing.T) {
	library := NewMethodLibrary()

	t.Run("nil task", func(t *testing.T) {
		assert.Nil(t, library.Find(nil))
		assert.Nil(t, library.FindAll(nil))
	})
	t.Run("no match", func(t *testing.T) {
		assert.Nil(t, library.Find(&execution.Task{Type: string(execution.TaskTypeAnalysis)}))
	})
	t.Run("match by type", func(t *testing.T) {
		found := library.Find(&execution.Task{Type: string(execution.TaskTypeReview), Instruction: "review it"})
		require.NotNil(t, found)
		assert.Equal(t, "code-review", found.Name)
	})
	t.Run("highest priority wins", func(t *testing.T) {
		library.Register(Method{
			Name:     "code-review-priority",
			TaskType: execution.TaskTypeReview,
			Priority: 5,
			Subtasks: []SubtaskSpec{validSubtask("fast")},
		})
		found := library.Find(&execution.Task{Type: string(execution.TaskTypeReview)})
		require.NotNil(t, found)
		assert.Equal(t, "code-review-priority", found.Name)
	})
	t.Run("precondition filters", func(t *testing.T) {
		library.Register(Method{
			Name:         "code-review-gated",
			TaskType:     execution.TaskTypeReview,
			Priority:     10,
			Precondition: func(*execution.Task) bool { return false },
			Subtasks:     []SubtaskSpec{validSubtask("gated")},
		})
		found := library.Find(&execution.Task{Type: string(execution.TaskTypeReview)})
		require.NotNil(t, found)
		assert.Equal(t, "code-review-priority", found.Name, "failing precondition excluded")
	})
	t.Run("priority tie broken by name", func(t *testing.T) {
		library.Register(Method{
			Name:     "aaa-review",
			TaskType: execution.TaskTypeReview,
			Priority: 10,
			Subtasks: []SubtaskSpec{validSubtask("tie")},
		})
		found := library.Find(&execution.Task{Type: string(execution.TaskTypeReview)})
		require.NotNil(t, found)
		assert.Equal(t, "aaa-review", found.Name)
	})
	t.Run("FindAll returns sorted candidates", func(t *testing.T) {
		all := library.FindAll(&execution.Task{Type: string(execution.TaskTypeReview)})
		require.GreaterOrEqual(t, len(all), 2)
		for i := 1; i < len(all); i++ {
			prev, curr := all[i-1], all[i]
			if prev.Priority == curr.Priority {
				assert.Less(t, prev.Name, curr.Name)
			} else {
				assert.Greater(t, prev.Priority, curr.Priority)
			}
		}
	})
	t.Run("FindAll empty", func(t *testing.T) {
		assert.Nil(t, library.FindAll(&execution.Task{Type: "nonexistent-type"}))
	})
}

func TestMethodLibraryFindResolved(t *testing.T) {
	library := NewMethodLibrary()
	resolved := library.FindResolved(&execution.Task{Type: string(execution.TaskTypeExplain), Instruction: "explain x"})
	require.NotNil(t, resolved)
	assert.Equal(t, "explain", resolved.Spec.Name)
	require.NoError(t, resolved.Validate())

	assert.Nil(t, library.FindResolved(&execution.Task{Type: "unknown"}))
	assert.Nil(t, library.FindResolved(nil))
}

func testEnvelope(t *testing.T) *contextdata.Envelope {
	t.Helper()
	return contextdata.NewEnvelope("task-htn", "session-htn")
}

func contextdataGetAny(env *contextdata.Envelope, key string) (any, bool) {
	return contextdata.GetTyped[any](env, key)
}

func TestPersistDispatchMetadata(t *testing.T) {
	env := testEnvelope(t)
	persistDispatchMetadata(env, "dispatcher-1", "agent:react", "explicit_coordination_target")
	raw, ok := contextdataGetAny(env, contextKeyLastDispatch)
	require.True(t, ok)
	meta, ok := raw.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "dispatcher-1", meta["dispatcher"])
	assert.Equal(t, "agent:react", meta["target"])
	assert.Equal(t, "explicit_coordination_target", meta["reason"])

	assert.NotPanics(t, func() { persistDispatchMetadata(nil, "d", "t", "r") })
}

func TestDecompose(t *testing.T) {
	t.Run("nil method errors", func(t *testing.T) {
		_, err := Decompose(&execution.Task{Instruction: "x"}, nil)
		require.EqualError(t, err, "htn: no method provided for decomposition")
	})
	t.Run("empty subtasks errors", func(t *testing.T) {
		_, err := Decompose(&execution.Task{}, &Method{Name: "empty"})
		require.EqualError(t, err, `htn: method "empty" has no subtasks`)
	})
	t.Run("subtasks become steps with namespaced ids", func(t *testing.T) {
		task := &execution.Task{Instruction: "ship the feature"}
		method := validMethod()
		plan, err := Decompose(task, &method)
		require.NoError(t, err)
		assert.Equal(t, "ship the feature", plan.Goal)
		require.Len(t, plan.Steps, 2)
		assert.Equal(t, "sample.first", plan.Steps[0].ID)
		assert.Equal(t, "do first", plan.Steps[0].Description, "instruction without template used verbatim")
		assert.Equal(t, "Complete first subtask", plan.Steps[0].Expected)
		assert.Equal(t, "sample.second", plan.Steps[1].ID)
		assert.Equal(t, []string{"sample.first"}, plan.Dependencies["sample.second"])
		assert.Empty(t, plan.Dependencies["sample.first"], "no deps entry without DependsOn")
	})
	t.Run("nil task yields empty goal", func(t *testing.T) {
		method := validMethod()
		plan, err := Decompose(nil, &method)
		require.NoError(t, err)
		assert.Empty(t, plan.Goal)
	})
	t.Run("instruction template expanded", func(t *testing.T) {
		method := Method{
			Name:     "tpl",
			TaskType: execution.TaskTypeExplain,
			Subtasks: []SubtaskSpec{
				{Name: "step", Type: execution.TaskTypeExplain, Instruction: "work on {{.Instruction}} now"},
			},
		}
		plan, err := Decompose(&execution.Task{Instruction: "the goal"}, &method)
		require.NoError(t, err)
		assert.Equal(t, "work on the goal now", plan.Steps[0].Description)
	})
}

func TestDecomposeResolved(t *testing.T) {
	method := validMethod()
	resolved := ResolveMethod(method)

	t.Run("nil resolved errors", func(t *testing.T) {
		_, err := DecomposeResolved(&execution.Task{}, nil)
		require.EqualError(t, err, "htn: no resolved method provided for decomposition")
	})
	t.Run("nil inner method errors", func(t *testing.T) {
		_, err := DecomposeResolved(&execution.Task{}, &ResolvedMethod{Spec: MethodSpec{Name: "x"}})
		require.EqualError(t, err, "htn: no resolved method provided for decomposition")
	})
	t.Run("no operators errors", func(t *testing.T) {
		_, err := DecomposeResolved(&execution.Task{}, &ResolvedMethod{Method: &Method{Name: "x"}})
		require.EqualError(t, err, "htn: resolved method has no operators")
	})
	t.Run("operators become steps with tool and params", func(t *testing.T) {
		plan, err := DecomposeResolved(&execution.Task{Instruction: "the mission"}, &resolved)
		require.NoError(t, err)
		assert.Equal(t, "the mission", plan.Goal)
		require.Len(t, plan.Steps, 2)

		first := plan.Steps[0]
		assert.Equal(t, "first", first.ID)
		assert.Equal(t, "do first", first.Description)
		assert.Equal(t, "react", first.Tool)
		assert.Equal(t, "Complete first operation", first.Expected)
		assert.Equal(t, "explain", first.Params["operator_task_type"])
		assert.Equal(t, "react", first.Params["operator_executor"])
		assert.Equal(t, "first", first.Params["operator_name"])
		require.NotEmpty(t, first.Params["required_capabilities"])

		second := plan.Steps[1]
		assert.Equal(t, "react", second.Tool, "default executor carried into the step tool")
		assert.Equal(t, []string{"first"}, plan.Dependencies["second"])
	})
}

func TestExpandInstruction(t *testing.T) {
	assert.Equal(t, "plain", expandInstruction("plain", "goal"))
	assert.Equal(t, "goal and goal", expandInstruction("{{.Instruction}} and {{.Instruction}}", "goal"))
	assert.Equal(t, "", expandInstruction("", "goal"))
}
