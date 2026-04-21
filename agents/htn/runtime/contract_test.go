package runtime_test

import (
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/agents/htn"
	"codeburg.org/lexbit/relurpify/framework/core"
)

func TestMethodValidateRejectsUnknownDependency(t *testing.T) {
	method := htn.Method{
		Name:     "bad-method",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []htn.SubtaskSpec{
			{Name: "analyze", Type: core.TaskTypeAnalysis, DependsOn: []string{"missing"}},
		},
	}
	err := method.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown subtask") {
		t.Fatalf("expected unknown subtask validation error, got %v", err)
	}
}

func TestMethodValidateAcceptsExplicitCapabilityExecutor(t *testing.T) {
	method := htn.Method{
		Name:     "custom-dispatch",
		TaskType: core.TaskTypeAnalysis,
		Subtasks: []htn.SubtaskSpec{
			{Name: "delegate", Type: core.TaskTypeAnalysis, Executor: "agent:reviewer"},
		},
	}
	if err := method.Validate(); err != nil {
		t.Fatalf("expected custom capability executor to validate, got %v", err)
	}
}

func TestHTNAgentInitializeRejectsInvalidMethodLibrary(t *testing.T) {
	agent := &htn.HTNAgent{
		Config:  &core.Config{},
		Methods: &htn.MethodLibrary{},
	}
	agent.Methods.Register(htn.Method{
		Name:     "broken",
		TaskType: core.TaskTypePlanning,
		Subtasks: []htn.SubtaskSpec{
			{Name: "plan", Type: core.TaskTypePlanning, DependsOn: []string{"missing"}},
		},
	})
	err := agent.Initialize(agent.Config)
	if err == nil || !strings.Contains(err.Error(), "invalid method library") {
		t.Fatalf("expected invalid method library error, got %v", err)
	}
}

func TestResolvedMethodValidateRejectsOperatorMismatch(t *testing.T) {
	resolved := htn.ResolveMethod(htn.Method{
		Name:     "code-new",
		TaskType: core.TaskTypeCodeGeneration,
		Subtasks: []htn.SubtaskSpec{
			{Name: "code", Type: core.TaskTypeCodeGeneration},
		},
	})
	resolved.Operators[0].TaskType = core.TaskTypeAnalysis
	err := resolved.Validate()
	if err == nil || !strings.Contains(err.Error(), "does not match subtask type") {
		t.Fatalf("expected operator mismatch validation error, got %v", err)
	}
}
