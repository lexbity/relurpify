package embedfs

import (
	"io/fs"
	"testing"
)

func TestWorkspaceEmbed_DecomposedPaths(t *testing.T) {
	efs := DefaultFS()

	paths := []string{
		"workspace/workspace.yaml",
		"workspace/model/profiles/default.llm.yaml",
		"workspace/model/provider/ollama.provider.yaml",
		"workspace/security/sandbox.policy.yaml",
		"workspace/security/shell.policy.yaml",
		"workspace/security/localtool.policy.yaml",
		"workspace/security/workspaceingestion.policy.yaml",
	}
	for _, path := range paths {
		if _, err := fs.Stat(efs, path); err != nil {
			t.Errorf("embedded template %s: %v", path, err)
		}
	}
}

func TestWorkspaceEmbed_NoAgentYAML(t *testing.T) {
	efs := DefaultFS()
	if _, err := fs.Stat(efs, "workspace/agent.yaml"); err == nil {
		t.Error("workspace/agent.yaml should not exist in embedded templates")
	}
}

func TestWorkspaceEmbed_NoAgentsDir(t *testing.T) {
	efs := DefaultFS()
	if _, err := fs.Stat(efs, "workspace/agents"); err == nil {
		t.Error("workspace/agents/ should not exist in embedded templates")
	}
}

func TestWorkspaceEmbed_HasTools(t *testing.T) {
	efs := DefaultFS()
	toolPath := "workspace/tools/file/file_read.tool.yaml"
	if _, err := fs.Stat(efs, toolPath); err != nil {
		t.Errorf("embedded template %s: %v", toolPath, err)
	}
}

func TestWorkspaceEmbed_HasSecurityPolicies(t *testing.T) {
	efs := DefaultFS()
	for _, name := range []string{"sandbox", "shell", "localtool", "workspaceingestion"} {
		path := "workspace/security/" + name + ".policy.yaml"
		if _, err := fs.Stat(efs, path); err != nil {
			t.Errorf("embedded security policy %s: %v", path, err)
		}
	}
}

func TestWorkspaceEmbed_ModelDirNotEmpty(t *testing.T) {
	efs := DefaultFS()
	profiles, err := fs.ReadDir(efs, "workspace/model/profiles")
	if err != nil {
		t.Fatalf("read model/profiles: %v", err)
	}
	if len(profiles) == 0 {
		t.Error("model/profiles/ is empty")
	}
	providers, err := fs.ReadDir(efs, "workspace/model/provider")
	if err != nil {
		t.Fatalf("read model/provider: %v", err)
	}
	if len(providers) == 0 {
		t.Error("model/provider/ is empty")
	}
}

func TestWorkspaceEmbed_PromptPaths(t *testing.T) {
	efs := DefaultFS()
	prompts := []string{
		"prompts/agents/agent.euclo.default.prompt",
		"prompts/framework/base.system.prompt",
	}
	for _, path := range prompts {
		if _, err := fs.Stat(efs, path); err != nil {
			t.Errorf("embedded prompt %s: %v", path, err)
		}
	}
}
