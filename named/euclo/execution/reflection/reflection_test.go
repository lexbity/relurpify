package reflection

import (
	"testing"

	testutil "github.com/lexcodex/relurpify/testutil/euclotestutil"
)

func TestNewBuildsReviewerAndDelegate(t *testing.T) {
	env := testutil.Env(t)

	runner := New(env)
	if runner == nil {
		t.Fatal("expected runner")
	}
	if runner.Reviewer != env.Model {
		t.Fatal("expected reviewer to be wired from environment")
	}
	if runner.Delegate == nil {
		t.Fatal("expected delegate runner")
	}
	if runner.Config != env.Config {
		t.Fatal("expected config to be wired from environment")
	}
}
