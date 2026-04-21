package execution

import (
	"errors"
	"testing"

	reactpkg "codeburg.org/lexbit/relurpify/agents/react"
	"codeburg.org/lexbit/relurpify/framework/core"
	eucloruntime "codeburg.org/lexbit/relurpify/named/euclo/runtime"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestSelectExecutorSupportsKnownFamiliesAndFallback(t *testing.T) {
	reactRunner := &reactpkg.ReActAgent{}
	ensureCalls := 0
	factory := ExecutorFactory{
		Model:       testutil.StubModel{},
		Config:      &core.Config{},
		Registry:    testutil.EnvMinimal().Registry,
		React:       reactRunner,
		EnsureReact: func() error { ensureCalls++; return nil },
	}

	cases := []struct {
		name   string
		family eucloruntime.ExecutorFamily
		path   string
	}{
		{name: "planner", family: eucloruntime.ExecutorFamilyPlanner, path: "planner_executor"},
		{name: "htn", family: eucloruntime.ExecutorFamilyHTN, path: "htn_executor"},
		{name: "rewoo", family: eucloruntime.ExecutorFamilyRewoo, path: "rewoo_executor"},
		{name: "reflection", family: eucloruntime.ExecutorFamilyReflection, path: "reflection_executor"},
		{name: "default", family: "", path: "react_executor"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			selection, err := SelectExecutor(factory, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutorDescriptor: eucloruntime.WorkUnitExecutorDescriptor{
				ExecutorID: "executor." + tc.name,
				Family:     tc.family,
				Reason:     "test",
			}},
			})
			if err != nil {
				t.Fatalf("SelectExecutor: %v", err)
			}
			if selection.Workflow == nil {
				t.Fatal("expected workflow executor")
			}
			if selection.Runtime.Path != tc.path {
				t.Fatalf("unexpected path: %q", selection.Runtime.Path)
			}
		})
	}

	if ensureCalls == 0 {
		t.Fatal("expected react bootstrap to be used for dependent families")
	}
}

func TestSelectExecutorReturnsEnsureReactError(t *testing.T) {
	wantErr := errors.New("react unavailable")
	_, err := SelectExecutor(ExecutorFactory{
		EnsureReact: func() error { return wantErr },
	}, eucloruntime.UnitOfWork{ExecutionDescriptor: eucloruntime.ExecutionDescriptor{ExecutorDescriptor: eucloruntime.WorkUnitExecutorDescriptor{
		Family: eucloruntime.ExecutorFamilyHTN,
	}},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected ensure react error, got %v", err)
	}
}
