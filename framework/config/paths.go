package config

import "path/filepath"

const DirName = "relurpify_cfg"

// Paths describes the canonical relurpify_cfg layout for one workspace.
type Paths struct {
	Workspace string
}

// New returns the canonical workspace path layout rooted at workspace.
func New(workspace string) Paths {
	if workspace == "" {
		workspace = "."
	}
	return Paths{Workspace: workspace}
}

func (p Paths) ConfigRoot() string {
	return filepath.Join(p.Workspace, DirName)
}

func (p Paths) ConfigFile() string {
	return filepath.Join(p.ConfigRoot(), "config.yaml")
}

func (p Paths) NexusConfigFile() string {
	return filepath.Join(p.ConfigRoot(), "nexus.yaml")
}

func (p Paths) ManifestFile() string {
	return filepath.Join(p.ConfigRoot(), "agent.manifest.yaml")
}

func (p Paths) AgentsDir() string {
	return filepath.Join(p.ConfigRoot(), "agents")
}

func (p Paths) SkillsDir() string {
	return filepath.Join(p.ConfigRoot(), "skills")
}

func (p Paths) LogsDir() string {
	return filepath.Join(p.ConfigRoot(), "logs")
}

func (p Paths) LogFile(name string) string {
	if name == "" {
		name = "relurpish.log"
	}
	return filepath.Join(p.LogsDir(), name)
}

func (p Paths) TelemetryDir() string {
	return filepath.Join(p.ConfigRoot(), "telemetry")
}

func (p Paths) TelemetryFile(name string) string {
	if name == "" {
		name = "telemetry.jsonl"
	}
	return filepath.Join(p.TelemetryDir(), name)
}

func (p Paths) EventsFile() string {
	return filepath.Join(p.ConfigRoot(), "events.db")
}

func (p Paths) NodesFile() string {
	return filepath.Join(p.ConfigRoot(), "nodes.db")
}

func (p Paths) SessionStoreFile() string {
	return filepath.Join(p.ConfigRoot(), "sessions.db")
}

func (p Paths) IdentityStoreFile() string {
	return filepath.Join(p.ConfigRoot(), "identities.db")
}

func (p Paths) AdminTokenStoreFile() string {
	return filepath.Join(p.ConfigRoot(), "admin_tokens.db")
}

func (p Paths) PolicyRulesFile() string {
	return filepath.Join(p.ConfigRoot(), "policy_rules.yaml")
}

func (p Paths) MemoryDir() string {
	return filepath.Join(p.ConfigRoot(), "memory")
}

func (p Paths) ASTIndexDir() string {
	return filepath.Join(p.MemoryDir(), "ast_index")
}

func (p Paths) ASTIndexDB() string {
	return filepath.Join(p.ASTIndexDir(), "index.db")
}

func (p Paths) RetrievalDB() string {
	return filepath.Join(p.MemoryDir(), "retrieval.db")
}

func (p Paths) SessionsDir() string {
	return filepath.Join(p.ConfigRoot(), "sessions")
}

func (p Paths) CheckpointsDir() string {
	return filepath.Join(p.SessionsDir(), "checkpoints")
}

func (p Paths) WorkflowStateFile() string {
	return filepath.Join(p.SessionsDir(), "workflow_state.db")
}

func (p Paths) ExportsDir() string {
	return filepath.Join(p.ConfigRoot(), "exports")
}

func (p Paths) TestsuitesDir() string {
	return filepath.Join(p.ConfigRoot(), "testsuites")
}

func (p Paths) TestRunsDir() string {
	return filepath.Join(p.ConfigRoot(), "test_runs")
}

func (p Paths) TestRunDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	return filepath.Join(segments...)
}

func (p Paths) TestRunLogsDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "logs")
	return filepath.Join(segments...)
}

func (p Paths) TestRunTelemetryDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "telemetry")
	return filepath.Join(segments...)
}

func (p Paths) TestRunArtifactsDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "artifacts")
	return filepath.Join(segments...)
}

func (p Paths) TestRunTmpDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "tmp")
	return filepath.Join(segments...)
}

func (p Paths) ShellBlacklistFile() string {
	return filepath.Join(p.ConfigRoot(), "shell_blacklist.yaml")
}

func (p Paths) ModelProfilesDir() string {
	return filepath.Join(p.ConfigRoot(), "model_profiles")
}
