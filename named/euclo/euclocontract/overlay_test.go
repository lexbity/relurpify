package euclocontract

import (
	"testing"

	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/userconfig/config"
	"codeburg.org/lexbit/relurpify/userconfig/config/security"
)

func TestOverlaySecurityBundle_NilBase(t *testing.T) {
	_, err := config.OverlaySecurityBundle(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil base")
	}
}

func TestOverlaySecurityBundle_NilBundle(t *testing.T) {
	base := config.BuildEffectiveAgentContract("test", &config.AgentSpec{
		Implementation: "coding",
		Model:          config.AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
		},
	}, permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})

	result, err := config.OverlaySecurityBundle(base, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.AgentID != "test" {
		t.Errorf("AgentID = %q, want %q", result.AgentID, "test")
	}
	if p := result.AgentSpec.ToolExecutionPolicy["cli_git"]; p.Execute != config.AgentPermissionAsk {
		t.Errorf("cli_git.Execute = %q, want %q", p.Execute, config.AgentPermissionAsk)
	}
}

func TestOverlaySecurityBundle_Purity(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", &config.AgentSpec{
		Implementation: "coding",
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
		},
		Bash: config.AgentBashPermissions{
			DenyPatterns: []string{"original-deny"},
		},
	}, permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})

	bundle := &security.Bundle{
		LocalTool: map[string]security.ToolPolicy{
			"cli_git": {Execute: "allow"},
		},
		Shell: &security.ShellBlacklist{
			Rules: []security.BlacklistRule{
				{ID: "test-rule", Pattern: "git reset --hard", Action: "block"},
			},
		},
	}

	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == base {
		t.Fatal("result is same pointer as base")
	}

	if p := base.AgentSpec.ToolExecutionPolicy["cli_git"]; p.Execute != config.AgentPermissionAsk {
		t.Errorf("base cli_git was mutated: Execute = %q, want %q", p.Execute, config.AgentPermissionAsk)
	}
	if len(base.AgentSpec.Bash.DenyPatterns) != 1 || base.AgentSpec.Bash.DenyPatterns[0] != "original-deny" {
		t.Errorf("base Bash.DenyPatterns mutated: %v", base.AgentSpec.Bash.DenyPatterns)
	}

	if p := result.AgentSpec.ToolExecutionPolicy["cli_git"]; p.Execute != config.AgentPermissionAllow {
		t.Errorf("result cli_git.Execute = %q, want %q", p.Execute, config.AgentPermissionAllow)
	}
}

func TestOverlaySecurityBundle_LocalToolAllow(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", defaultOverlaySpec(), permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})
	bundle := &security.Bundle{
		LocalTool: map[string]security.ToolPolicy{
			"cli_git": {Execute: "allow"},
		},
	}
	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p := result.AgentSpec.ToolExecutionPolicy["cli_git"]; p.Execute != config.AgentPermissionAllow {
		t.Errorf("allow: Execute = %q, want %q", p.Execute, config.AgentPermissionAllow)
	}
}

func TestOverlaySecurityBundle_LocalToolDeny(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", defaultOverlaySpec(), permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})
	bundle := &security.Bundle{
		LocalTool: map[string]security.ToolPolicy{
			"cli_git": {Execute: "deny"},
		},
	}
	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p := result.AgentSpec.ToolExecutionPolicy["cli_git"]; p.Execute != config.AgentPermissionDeny {
		t.Errorf("deny: Execute = %q, want %q", p.Execute, config.AgentPermissionDeny)
	}
}

func TestOverlaySecurityBundle_LocalToolAsk(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", defaultOverlaySpec(), permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})
	bundle := &security.Bundle{
		LocalTool: map[string]security.ToolPolicy{
			"cli_git": {Execute: "ask"},
		},
	}
	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p := result.AgentSpec.ToolExecutionPolicy["cli_git"]; p.Execute != config.AgentPermissionAsk {
		t.Errorf("ask: Execute = %q, want %q", p.Execute, config.AgentPermissionAsk)
	}
}

func TestOverlaySecurityBundle_LocalToolNewTool(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", defaultOverlaySpec(), permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})
	bundle := &security.Bundle{
		LocalTool: map[string]security.ToolPolicy{
			"new_tool": {Execute: "deny"},
		},
	}
	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p := result.AgentSpec.ToolExecutionPolicy["new_tool"]; p.Execute != config.AgentPermissionDeny {
		t.Errorf("new_tool.Execute = %q, want %q", p.Execute, config.AgentPermissionDeny)
	}
	if _, ok := result.AgentSpec.ToolExecutionPolicy["cli_git"]; !ok {
		t.Error("cli_git missing from result")
	}
}

func TestOverlaySecurityBundle_ShellDenyPatterns(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", defaultOverlaySpec(), permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})
	bundle := &security.Bundle{
		Shell: &security.ShellBlacklist{
			Rules: []security.BlacklistRule{
				{ID: "r1", Pattern: "git reset --hard", Action: "block"},
				{ID: "r2", Pattern: "rm -rf /", Action: "block"},
				{ID: "r3", Pattern: "git push --force", Action: "hitl"},
			},
		},
	}
	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deny := result.AgentSpec.Bash.DenyPatterns
	if len(deny) != 2 {
		t.Fatalf("DenyPatterns length = %d, want 2: %v", len(deny), deny)
	}
	if deny[0] != "git reset --hard" {
		t.Errorf("DenyPatterns[0] = %q, want %q", deny[0], "git reset --hard")
	}
	if deny[1] != "rm -rf /" {
		t.Errorf("DenyPatterns[1] = %q, want %q", deny[1], "rm -rf /")
	}
}

func TestOverlaySecurityBundle_Sandbox(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", defaultOverlaySpec(), permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{}, config.SourceSummary{})
	bundle := &security.Bundle{
		Sandbox: &security.SandboxPolicy{
			ReadOnlyRoot:    true,
			NoNewPrivileges: true,
		},
	}
	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Security.ReadOnlyRoot {
		t.Error("ReadOnlyRoot is false, want true")
	}
	if !result.Security.NoNewPrivileges {
		t.Error("NoNewPrivileges is false, want true")
	}
}

func TestOverlaySecurityBundle_SandboxOverrides(t *testing.T) {
	base := config.BuildEffectiveAgentContract("euclo", defaultOverlaySpec(), permissions.PermissionSet{}, config.ResourceSpec{}, config.SecuritySpec{RunAsUser: 1001, ReadOnlyRoot: true, NoNewPrivileges: true}, config.SourceSummary{})
	bundle := &security.Bundle{
		Sandbox: &security.SandboxPolicy{
			ReadOnlyRoot:    false,
			NoNewPrivileges: false,
		},
	}
	result, err := config.OverlaySecurityBundle(base, bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Security.ReadOnlyRoot {
		t.Error("ReadOnlyRoot is true after overlay with false")
	}
	if result.Security.NoNewPrivileges {
		t.Error("NoNewPrivileges is true after overlay with false")
	}
}

func defaultOverlaySpec() *config.AgentSpec {
	return &config.AgentSpec{
		Implementation: "coding",
		Version:        "2",
		Model:          config.AgentModelConfig{Provider: "ollama", Name: "gemma4:e4b"},
		ToolExecutionPolicy: map[string]config.ToolPolicy{
			"cli_git": {Execute: config.AgentPermissionAsk},
			"bash":    {Execute: config.AgentPermissionAsk},
		},
	}
}
