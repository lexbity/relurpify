package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/agents"
	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/manifest"
	"github.com/lexcodex/relurpify/framework/persistence"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
	"github.com/lexcodex/relurpify/framework/workspacecfg"
	"github.com/stretchr/testify/require"
)

func TestRuntimeAdapterSessionInfoUsesLiveAgentModeAndStrategy(t *testing.T) {
	rt := &runtimesvc.Runtime{
		Config: runtimesvc.Config{
			Workspace:   "/workspace",
			OllamaModel: "base-model",
			AgentName:   "coding-go",
		},
		Agent: &agents.CodingAgent{},
		Registration: &fruntime.AgentRegistration{
			Manifest: &manifest.AgentManifest{
				Metadata: manifest.ManifestMetadata{Name: "coding-go"},
				Spec: manifest.ManifestSpec{
					Agent: &core.AgentRuntimeSpec{
						Model: core.AgentModelConfig{Name: "manifest-model"},
						Mode:  core.AgentModePrimary,
						Context: core.AgentContextSpec{
							MaxTokens: 4096,
						},
					},
				},
			},
		},
	}

	info := (&runtimeAdapter{rt: rt}).SessionInfo()

	require.Equal(t, "coding-go", info.Agent)
	require.Equal(t, "manifest-model", info.Model)
	require.Equal(t, "primary", info.Role)
	require.Equal(t, string(agents.ModeCode), info.Mode)
	require.Equal(t, agents.ModeProfiles[agents.ModeCode].PreferredStrategy, info.Strategy)
	require.Equal(t, 4096, info.MaxTokens)
}

func TestDescribeAgentRuntimeForReflectionUsesDelegateMode(t *testing.T) {
	mode, strategy := describeAgentRuntime(&agents.ReflectionAgent{
		Delegate: &agents.CodingAgent{},
	})

	require.Equal(t, string(agents.ModeCode), mode)
	require.Equal(t, "reflection", strategy)
}

func TestRuntimeAdapterListsWorkflows(t *testing.T) {
	workspace := t.TempDir()
	dbPath := workspacecfg.New(workspace).WorkflowStateFile()
	require.NoError(t, os.MkdirAll(filepath.Dir(dbPath), 0o755))
	store, err := persistence.NewSQLiteWorkflowStateStore(dbPath)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.CreateWorkflow(context.Background(), persistence.WorkflowRecord{
		WorkflowID:  "wf-1",
		TaskID:      "wf-1",
		TaskType:    core.TaskTypeCodeModification,
		Instruction: "Inspect me",
		Status:      persistence.WorkflowRunStatusRunning,
		UpdatedAt:   time.Now().UTC(),
	}))

	rt := &runtimesvc.Runtime{
		Config: runtimesvc.Config{
			Workspace: workspace,
		},
	}

	adapter := &runtimeAdapter{rt: rt}
	workflows, err := adapter.ListWorkflows(10)
	require.NoError(t, err)
	require.Len(t, workflows, 1)
	require.Equal(t, "wf-1", workflows[0].WorkflowID)

	details, err := adapter.GetWorkflow("wf-1")
	require.NoError(t, err)
	require.Equal(t, "wf-1", details.Workflow.WorkflowID)
}
