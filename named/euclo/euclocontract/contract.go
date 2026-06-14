// Package euclocontract provides the built-in euclo agent contract.
// DefaultContract returns euclo's identity, spec defaults, and executable
// baseline without requiring any per-agent YAML file. The returned contract
// is overlaid with the split security bundle (LocalTool/Shell/Sandbox) before
// being applied to the runtime.
package euclocontract

import (
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/userconfig/config"
)

const (
	agentID = "euclo"
)

// boolPtr returns a pointer to v for use in struct literals.
func boolPtr(v bool) *bool { return &v }

// DefaultContract returns euclo's built-in runtime contract: AgentSpec (coding
// implementation, ollama/gemma4:e4b defaults, ToolExecutionPolicy defaults with
// cli_git and bash => "ask"), a minimal executable baseline (git, rg), and
// conservative resource/security defaults.
//
// It is pure and allocation-fresh; callers own the returned value.
// The returned contract is overlaid with the split security bundle
// (LocalTool/Shell/Sandbox) before being applied to the runtime.
func DefaultContract() *config.EffectiveAgentContract {
	return config.BuildEffectiveAgentContract(
		agentID,
		defaultAgentSpec(),
		defaultPermissions(),
		config.ResourceSpec{},
		config.SecuritySpec{},
		config.SourceSummary{
			GlobalDefaults: true,
		},
	)
}

func defaultAgentSpec() *config.AgentSpec {
	return &config.AgentSpec{
		Implementation: "coding",
		Version:        "2",
		Prompt:         "",
		Model: config.AgentModelConfig{
			Provider:    "ollama",
			Name:        "gemma4:e4b",
			Temperature: 0,
			MaxTokens:   4096,
		},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
			"bash":    {Execute: config.AgentPermissionAsk},
		},
		Bash: config.AgentBashPermissions{},
		Logging: &config.AgentLoggingSpec{
			LLM:   boolPtr(true),
			Agent: boolPtr(true),
		},
	}
}

func defaultPermissions() permissions.PermissionSet {
	return permissions.PermissionSet{
		Executables: []permissions.ExecutablePermission{
			{Binary: "git"},
			{Binary: "rg"},
		},
		FileSystem: []permissions.FileSystemPermission{
			{Action: "fs:read", Path: "${workspace}/**"},
			{Action: "fs:write", Path: "${workspace}/**"},
			// fs:list is required so workspace indexing (directory traversal)
			// is statically authorized; without it directory walks fall through
			// to the HITL "ask" path and block boot with no approver.
			{Action: "fs:list", Path: "${workspace}/**"},
		},
	}
}
