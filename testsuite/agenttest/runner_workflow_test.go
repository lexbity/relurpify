package agenttest

import (
	"reflect"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
)

func TestInstantiateAgentByNameConfiguresWorkflowPaths(t *testing.T) {
	workspace := t.TempDir()

	agent := instantiateAgentByName(workspace, "coding", agentenv.AgentEnvironment{
		Registry: capability.NewRegistry(),
		Config:   &core.Config{MaxIterations: 1},
	})
	value := reflect.ValueOf(agent)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	field := value.FieldByName("CheckpointPath")
	if !field.IsValid() || field.Kind() != reflect.String {
		t.Fatalf("expected agent checkpoint field, got %T", agent)
	}
	if field.String() == "" {
		t.Fatal("expected checkpoint path to be configured")
	}
}
