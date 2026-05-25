package manifest

import (
	"path/filepath"
	"strings"
)

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

func (p Paths) WorkspaceFile() string {
	return filepath.Join(p.ConfigRoot(), "workspace.yaml")
}

func (p Paths) AgentsDir() string {
	return filepath.Join(p.ConfigRoot(), "agents")
}

func (p Paths) SkillsDir() string {
	return filepath.Join(p.ConfigRoot(), "skills")
}

func (p Paths) StateRoot() string {
	return filepath.Join(p.Workspace, ".relurpify_state")
}

func (p Paths) LogsDir() string {
	return filepath.Join(p.StateRoot(), "logs")
}

func (p Paths) LogFile(name string) string {
	if name == "" {
		name = "relurpish.log"
	}
	return filepath.Join(p.LogsDir(), name)
}

func (p Paths) TelemetryDir() string {
	return filepath.Join(p.StateRoot(), "telemetry")
}

func (p Paths) TelemetryFile(name string) string {
	if name == "" {
		name = "telemetry.jsonl"
	}
	return filepath.Join(p.TelemetryDir(), name)
}

func (p Paths) EventsFile() string {
	return filepath.Join(p.StateRoot(), "events.db")
}

func (p Paths) NodesFile() string {
	return filepath.Join(p.StateRoot(), "nodes.db")
}

func (p Paths) SessionStoreFile() string {
	return filepath.Join(p.StateRoot(), "sessions.db")
}

func (p Paths) IdentityStoreFile() string {
	return filepath.Join(p.StateRoot(), "identities.db")
}

func (p Paths) AdminTokenStoreFile() string {
	return filepath.Join(p.StateRoot(), "admin_tokens.db")
}


func (p Paths) MemoryDir() string {
	return filepath.Join(p.StateRoot(), "memory")
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
	return filepath.Join(p.StateRoot(), "sessions")
}

func (p Paths) CheckpointsDir() string {
	return filepath.Join(p.SessionsDir(), "checkpoints")
}

func (p Paths) WorkflowStateFile() string {
	return filepath.Join(p.SessionsDir(), "workflow_state.db")
}

func (p Paths) ExportsDir() string {
	return filepath.Join(p.StateRoot(), "exports")
}

func (p Paths) TestsuitesDir() string {
	return filepath.Join(p.StateRoot(), "testsuites")
}

func (p Paths) TestRunsDir() string {
	return filepath.Join(p.StateRoot(), "test_run")
}

func (p Paths) TestSetupDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "setup")
	return filepath.Join(segments...)
}

func (p Paths) TestRunDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "execution")
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

func (p Paths) ModelProfilesDir() string {
	return filepath.Join(p.ConfigRoot(), "model", "profiles")
}

// GovernanceRoots returns the canonical workspace governance paths that should
// be protected from agent writes and executable mutation, including the
// relurpify_cfg root itself.
func (p Paths) GovernanceRoots(extra ...string) []string {
	roots := []string{
		p.ConfigRoot(),
		p.WorkspaceFile(),
		p.AgentsDir(),
		p.ModelProfilesDir(),
	}
	for _, path := range extra {
		if strings.TrimSpace(path) == "" {
			continue
		}
		roots = append(roots, path)
	}
	return roots
}
