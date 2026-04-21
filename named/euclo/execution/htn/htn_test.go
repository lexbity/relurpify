package htn

import (
	"testing"

	testutil "codeburg.org/lexbit/relurpify/testutil/euclotestutil"
)

func TestNewInitializesRunnerFromEnvironment(t *testing.T) {
	env := testutil.Env(t)

	runner := New(env)
	if runner == nil {
		t.Fatal("expected runner")
	}
	if runner.Model != env.Model {
		t.Fatal("expected model to be wired from environment")
	}
	if runner.Tools != env.Registry {
		t.Fatal("expected registry to be wired from environment")
	}
	if runner.Config != env.Config {
		t.Fatal("expected config to be wired from environment")
	}
}
