package cfgload

import (
	"testing"

	"codeburg.org/lexbit/relurpify/framework/cfgload/model"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceValidateRejectsInvalidFields(t *testing.T) {
	stateDir := "/abs/state"
	backend := "unknown"
	level := "verbose"
	format := "yaml"
	cfg := WorkspaceConfig{
		WorkspaceAbs: "/tmp/workspace",
		Paths:        WorkspacePaths{StateDir: &stateDir},
		Sandbox:      WorkspaceSandbox{Backend: &backend},
		Logging:      WorkspaceLogging{Level: &level, Format: &format},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "paths.state_dir must be relative")
	require.Contains(t, err.Error(), "sandbox.backend must be one of")
	require.Contains(t, err.Error(), "logging.level must be one of")
	require.Contains(t, err.Error(), "logging.format must be one of")
	require.Contains(t, err.Error(), "audit.retention_days required")
}

func TestWorkspaceValidateRejectsTraversalStateDir(t *testing.T) {
	stateDir := "../state"
	backend := "gvisor"
	level := "info"
	format := "json"
	retention := 7
	cfg := WorkspaceConfig{
		WorkspaceAbs: "/tmp/workspace",
		Paths:        WorkspacePaths{StateDir: &stateDir},
		Sandbox:      WorkspaceSandbox{Backend: &backend},
		Logging:      WorkspaceLogging{Level: &level, Format: &format},
		Audit:        WorkspaceAudit{RetentionDays: &retention},
	}
	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "paths.state_dir must stay within the workspace")
}

func TestWorkspaceValidateAcceptsValidConfig(t *testing.T) {
	stateDir := ".relurpify_state"
	backend := "gvisor"
	level := "info"
	format := "json"
	retention := 7
	cfg := WorkspaceConfig{
		WorkspaceAbs: "/tmp/workspace",
		Paths:        WorkspacePaths{StateDir: &stateDir},
		Sandbox:      WorkspaceSandbox{Backend: &backend},
		Logging:      WorkspaceLogging{Level: &level, Format: &format},
		Audit:        WorkspaceAudit{RetentionDays: &retention},
	}
	require.NoError(t, cfg.Validate())
}

func TestWorkspaceValidateModelRef(t *testing.T) {
	provider := &model.ResolvedProvider{
		Name:     "ollama",
		Kind:     "ollama",
		Endpoint: "http://localhost:11434",
	}
	modelName := "gemma4:e4b"
	cfg := WorkspaceConfig{
		Model: model.ModelRef{Provider: "ollama", Name: modelName},
	}
	require.NoError(t, cfg.ValidateModelRef([]*model.ResolvedProvider{provider}))
}
