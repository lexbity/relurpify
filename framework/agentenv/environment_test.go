package agentenv_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/memory"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestWithRegistry_ShallowCopy(t *testing.T) {
	r1 := capability.NewRegistry()
	r2 := capability.NewRegistry()
	env := agentenv.WorkspaceEnvironment{Registry: r1}

	got := env.WithRegistry(r2)
	if got.Registry != r2 {
		t.Fatal("expected copied env to use replacement registry")
	}
	if env.Registry != r1 {
		t.Fatal("expected original env registry to remain unchanged")
	}
}

func TestWithMemory_ShallowCopy(t *testing.T) {
	env := testutil.Env(t)
	replacement := memory.NewWorkingMemoryStore()

	got := env.WithMemory(replacement)
	if got.WorkingMemory != replacement {
		t.Fatal("expected copied env to use replacement memory")
	}
	if env.WorkingMemory == replacement {
		t.Fatal("expected original env memory to remain unchanged")
	}
}

func TestWithRegistry_NilRegistry(t *testing.T) {
	env := agentenv.WorkspaceEnvironment{Registry: capability.NewRegistry()}
	got := env.WithRegistry(nil)
	if got.Registry != nil {
		t.Fatal("expected nil registry")
	}
}

func TestEnvMinimal_RegistryNotNil(t *testing.T) {
	env := testutil.EnvMinimal()
	if env.Registry == nil {
		t.Fatal("expected non-nil registry")
	}
	if env.Config == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestEnv_WorkingMemoryNotNil(t *testing.T) {
	env := testutil.Env(t)
	if env.WorkingMemory == nil {
		t.Fatal("expected non-nil working memory")
	}
}
