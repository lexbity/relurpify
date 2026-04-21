package agentenv_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestWithRegistry_ShallowCopy(t *testing.T) {
	r1 := capability.NewRegistry()
	r2 := capability.NewRegistry()
	env := agentenv.AgentEnvironment{Registry: r1}

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
	replacement := testutil.Env(t).Memory

	got := env.WithMemory(replacement)
	if got.Memory != replacement {
		t.Fatal("expected copied env to use replacement memory")
	}
	if env.Memory == replacement {
		t.Fatal("expected original env memory to remain unchanged")
	}
}

func TestWithRegistry_NilRegistry(t *testing.T) {
	env := agentenv.AgentEnvironment{Registry: capability.NewRegistry()}
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

func TestEnv_MemoryNotNil(t *testing.T) {
	env := testutil.Env(t)
	if env.Memory == nil {
		t.Fatal("expected non-nil memory")
	}
}
