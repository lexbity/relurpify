package execution

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestNewArchitectReturnsConfiguredRunner(t *testing.T) {
	env := testutil.Env(t)

	runner := NewArchitect(env)
	if runner == nil {
		t.Fatal("expected runner")
	}
	if runner.Model != env.Model {
		t.Fatal("expected model to be wired from environment")
	}
	if runner.PlannerTools != env.Registry {
		t.Fatal("expected planner registry to be wired from environment")
	}
	if runner.ExecutorTools != env.Registry {
		t.Fatal("expected executor registry to be wired from environment")
	}
}

func TestExecuteArchitectRejectsNilTask(t *testing.T) {
	env := testutil.Env(t)

	_, err := ExecuteArchitect(context.Background(), env, nil, core.NewContext())
	if err == nil {
		t.Fatal("expected error")
	}
}
