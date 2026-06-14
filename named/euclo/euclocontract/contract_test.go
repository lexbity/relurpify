package euclocontract

import (
	"testing"

	"codeburg.org/lexbit/relurpify/userconfig/config"
)

func TestDefaultContract_ReturnsNonNil(t *testing.T) {
	c := DefaultContract()
	if c == nil {
		t.Fatal("DefaultContract() returned nil")
	}
}

func TestDefaultContract_HasEucloIdentity(t *testing.T) {
	c := DefaultContract()
	if c.AgentID != "euclo" {
		t.Errorf("AgentID = %q, want %q", c.AgentID, "euclo")
	}
	if c.AgentSpec == nil {
		t.Fatal("AgentSpec is nil")
	}
	if c.AgentSpec.Implementation != "coding" {
		t.Errorf("Implementation = %q, want %q", c.AgentSpec.Implementation, "coding")
	}
	if c.AgentSpec.Version != "2" {
		t.Errorf("Version = %q, want %q", c.AgentSpec.Version, "2")
	}
}

func TestDefaultContract_HasDefaultModel(t *testing.T) {
	c := DefaultContract()
	m := c.AgentSpec.Model
	if m.Provider != "ollama" {
		t.Errorf("Model.Provider = %q, want %q", m.Provider, "ollama")
	}
	if m.Name != "gemma4:e4b" {
		t.Errorf("Model.Name = %q, want %q", m.Name, "gemma4:e4b")
	}
	if m.Temperature != 0 {
		t.Errorf("Model.Temperature = %v, want 0", m.Temperature)
	}
	if m.MaxTokens <= 0 {
		t.Errorf("Model.MaxTokens = %d, want > 0", m.MaxTokens)
	}
}

func TestDefaultContract_CliGitAndBashDefaultToAsk(t *testing.T) {
	c := DefaultContract()
	policy := c.AgentSpec.ToolExecutionPolicy

	if p, ok := policy["cli_git"]; !ok {
		t.Error("cli_git not in ToolExecutionPolicy")
	} else if p.Execute != config.AgentPermissionAsk {
		t.Errorf("cli_git.Execute = %q, want %q", p.Execute, config.AgentPermissionAsk)
	}

	if p, ok := policy["bash"]; !ok {
		t.Error("bash not in ToolExecutionPolicy")
	} else if p.Execute != config.AgentPermissionAsk {
		t.Errorf("bash.Execute = %q, want %q", p.Execute, config.AgentPermissionAsk)
	}
}

func TestDefaultContract_HasGitExecutableBaseline(t *testing.T) {
	c := DefaultContract()
	perms := c.Permissions

	foundGit := false
	foundRg := false
	for _, e := range perms.Executables {
		switch e.Binary {
		case "git":
			foundGit = true
		case "rg":
			foundRg = true
		}
	}
	if !foundGit {
		t.Error("git not in Executables baseline")
	}
	if !foundRg {
		t.Error("rg not in Executables baseline")
	}

	// Baseline executables should not restrict args (nil args = all args allowed).
	for _, e := range perms.Executables {
		if len(e.Args) > 0 {
			t.Errorf("executable %q has args restriction %v; baseline should allow all args", e.Binary, e.Args)
		}
	}
}

func TestDefaultContract_HasFileSystemPermissions(t *testing.T) {
	c := DefaultContract()
	perms := c.Permissions

	if len(perms.FileSystem) == 0 {
		t.Fatal("no FileSystem permissions")
	}

	hasRead := false
	hasWrite := false
	for _, fs := range perms.FileSystem {
		switch fs.Action {
		case "fs:read":
			hasRead = true
		case "fs:write":
			hasWrite = true
		}
	}
	if !hasRead {
		t.Error("missing fs:read permission")
	}
	if !hasWrite {
		t.Error("missing fs:write permission")
	}
}

func TestDefaultContract_SourcesMarkedAsGlobalDefaults(t *testing.T) {
	c := DefaultContract()
	if !c.Sources.GlobalDefaults {
		t.Error("Sources.GlobalDefaults is false, want true")
	}
}

func TestDefaultContract_ReturnsDistinctInstances(t *testing.T) {
	a := DefaultContract()
	b := DefaultContract()
	if a == b {
		t.Error("consecutive calls returned the same pointer")
	}
	// Mutating one must not affect the other.
	a.AgentSpec.Implementation = "mutated"
	if b.AgentSpec.Implementation == "mutated" {
		t.Error("mutating one instance affected the other")
	}
}

func TestDefaultContract_LoggingEnabled(t *testing.T) {
	c := DefaultContract()
	if c.AgentSpec.Logging == nil {
		t.Fatal("Logging is nil")
	}
	if c.AgentSpec.Logging.LLM == nil || !*c.AgentSpec.Logging.LLM {
		t.Error("Logging.LLM is not true")
	}
	if c.AgentSpec.Logging.Agent == nil || !*c.AgentSpec.Logging.Agent {
		t.Error("Logging.Agent is not true")
	}
}
