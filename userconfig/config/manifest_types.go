package config

import (
	"path/filepath"
	"strings"

	"codeburg.org/lexbit/relurpify/userconfig/config/secretscan"
)

// DirName is the canonical top-level config directory under a workspace.
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

// ConfigRoot returns the workspace-local relurpify_cfg directory.
func (p Paths) ConfigRoot() string {
	return filepath.Join(p.Workspace, DirName)
}

// WorkspaceFile returns the workspace.yaml path.
func (p Paths) WorkspaceFile() string {
	return filepath.Join(p.ConfigRoot(), "workspace.yaml")
}

// AgentsDir returns the agent manifest directory.
func (p Paths) AgentsDir() string {
	return filepath.Join(p.ConfigRoot(), "agents")
}

// StateRoot returns the runtime state directory.
func (p Paths) StateRoot() string {
	return filepath.Join(p.Workspace, secretscan.RuntimeStateDirName)
}

// RuntimeWorkspaceFile returns the persisted runtime workspace state path.
func (p Paths) RuntimeWorkspaceFile() string {
	return filepath.Join(p.StateRoot(), "workspace.yaml")
}

// RuntimeProvidersFile returns the persisted provider editor state path.
func (p Paths) RuntimeProvidersFile() string {
	return filepath.Join(p.StateRoot(), "providers.yaml")
}

// RuntimeKeybindingsFile returns the persisted keybinding editor state path.
func (p Paths) RuntimeKeybindingsFile() string {
	return filepath.Join(p.StateRoot(), "keybindings.yaml")
}

// LogsDir returns the runtime logs directory.
func (p Paths) LogsDir() string {
	return filepath.Join(p.StateRoot(), "logs")
}

// LogFile returns a log file path under the runtime logs directory.
func (p Paths) LogFile(name string) string {
	if name == "" {
		name = "relurpish.log"
	}
	return filepath.Join(p.LogsDir(), name)
}

// TelemetryDir returns the runtime telemetry directory.
func (p Paths) TelemetryDir() string {
	return filepath.Join(p.StateRoot(), "telemetry")
}

// TelemetryFile returns a telemetry file path under the runtime telemetry directory.
func (p Paths) TelemetryFile(name string) string {
	if name == "" {
		name = "telemetry.jsonl"
	}
	return filepath.Join(p.TelemetryDir(), name)
}

// EventsFile returns the runtime events database path.
func (p Paths) EventsFile() string {
	return filepath.Join(p.StateRoot(), "events.db")
}

// NodesFile returns the runtime nodes database path.
func (p Paths) NodesFile() string {
	return filepath.Join(p.StateRoot(), "nodes.db")
}

// SessionStoreFile returns the runtime sessions database path.
func (p Paths) SessionStoreFile() string {
	return filepath.Join(p.StateRoot(), "sessions.db")
}

// IdentityStoreFile returns the runtime identities database path.
func (p Paths) IdentityStoreFile() string {
	return filepath.Join(p.StateRoot(), "identities.db")
}

// AdminTokenStoreFile returns the runtime admin-token database path.
func (p Paths) AdminTokenStoreFile() string {
	return filepath.Join(p.StateRoot(), "admin_tokens.db")
}

// MemoryDir returns the runtime memory directory.
func (p Paths) MemoryDir() string {
	return filepath.Join(p.StateRoot(), "memory")
}

// ASTIndexDir returns the AST index directory.
func (p Paths) ASTIndexDir() string {
	return filepath.Join(p.MemoryDir(), "ast_index")
}

// ASTIndexDB returns the AST index database path.
func (p Paths) ASTIndexDB() string {
	return filepath.Join(p.ASTIndexDir(), "index.db")
}

// RetrievalDB returns the retrieval database path.
func (p Paths) RetrievalDB() string {
	return filepath.Join(p.MemoryDir(), "retrieval.db")
}

// SessionsDir returns the runtime sessions directory.
func (p Paths) SessionsDir() string {
	return filepath.Join(p.StateRoot(), "sessions")
}

// CheckpointsDir returns the checkpoint directory under sessions.
func (p Paths) CheckpointsDir() string {
	return filepath.Join(p.SessionsDir(), "checkpoints")
}

// WorkflowStateFile returns the workflow state database path.
func (p Paths) WorkflowStateFile() string {
	return filepath.Join(p.SessionsDir(), "workflow_state.db")
}

// ExportsDir returns the runtime exports directory.
func (p Paths) ExportsDir() string {
	return filepath.Join(p.StateRoot(), "exports")
}

// TestsuitesDir returns the runtime test-suite directory.
func (p Paths) TestsuitesDir() string {
	return filepath.Join(p.StateRoot(), "testsuites")
}

// TestRunsDir returns the runtime test-run root directory.
func (p Paths) TestRunsDir() string {
	return filepath.Join(p.StateRoot(), "test_run")
}

// TestSetupDir returns the setup directory for a test run.
func (p Paths) TestSetupDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "setup")
	return filepath.Join(segments...)
}

// TestRunDir returns the execution directory for a test run.
func (p Paths) TestRunDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "execution")
	return filepath.Join(segments...)
}

// TestRunLogsDir returns the logs directory for a test run.
func (p Paths) TestRunLogsDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "logs")
	return filepath.Join(segments...)
}

// TestRunTelemetryDir returns the telemetry directory for a test run.
func (p Paths) TestRunTelemetryDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "telemetry")
	return filepath.Join(segments...)
}

// TestRunArtifactsDir returns the artifacts directory for a test run.
func (p Paths) TestRunArtifactsDir(parts ...string) string {
	segments := append([]string{p.TestRunDir(parts...)}, "artifacts")
	return filepath.Join(segments...)
}

// TestRunTmpDir returns the temp directory for a test run.
func (p Paths) TestRunTmpDir(parts ...string) string {
	segments := append([]string{p.TestRunsDir()}, parts...)
	segments = append(segments, "tmp")
	return filepath.Join(segments...)
}

// ModelProfilesDir returns the model profile directory under config.
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

// ManifestMetadata describes identity fields.
type ManifestMetadata struct {
	Name        string `yaml:"name" json:"name"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// ResourceSpec declares resource limits.
type ResourceSpec struct {
	Limits ResourceLimit `yaml:"limits" json:"limits"`
}

// ResourceLimit tracks CPU/memory/disk quotas.
type ResourceLimit struct {
	CPU     string `yaml:"cpu" json:"cpu"`
	Memory  string `yaml:"memory" json:"memory"`
	DiskIO  string `yaml:"disk_io" json:"disk_io"`
	Network string `yaml:"network,omitempty" json:"network,omitempty"`
}

// SecuritySpec enumerates container security toggles.
type SecuritySpec struct {
	RunAsUser       int  `yaml:"run_as_user" json:"run_as_user"`
	ReadOnlyRoot    bool `yaml:"read_only_root" json:"read_only_root"`
	NoNewPrivileges bool `yaml:"no_new_privileges" json:"no_new_privileges"`
}

// AuditSpec configures audit verbosity.
type AuditSpec struct {
	Level         string `yaml:"level" json:"level"`
	RetentionDays int    `yaml:"retention_days" json:"retention_days"`
}

// ContextPolicy defines the context policy section in an agent manifest.
// This mirrors execution/context.ContextPolicyBundle to avoid import cycles.
type ContextPolicy struct {
	CompilationMode       string                   `yaml:"compilation_mode,omitempty" json:"compilation_mode,omitempty"`
	DefaultTrustClass     string                   `yaml:"default_trust_class,omitempty" json:"default_trust_class,omitempty"`
	Rankers               []string                 `yaml:"rankers,omitempty" json:"rankers,omitempty"`
	Scanners              []string                 `yaml:"scanners,omitempty" json:"scanners,omitempty"`
	Summarizers           []string                 `yaml:"summarizers,omitempty" json:"summarizers,omitempty"`
	Quota                 *QuotaSpec               `yaml:"quota,omitempty" json:"quota,omitempty"`
	RateLimit             *RateLimitSpec           `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	TrustDemotedPolicy    string                   `yaml:"trust_demoted_policy,omitempty" json:"trust_demoted_policy,omitempty"`
	DegradedChunkPolicy   string                   `yaml:"degraded_chunk_policy,omitempty" json:"degraded_chunk_policy,omitempty"`
	BudgetShortfallPolicy string                   `yaml:"budget_shortfall_policy,omitempty" json:"budget_shortfall_policy,omitempty"`
	SubstitutionPrefs     []SubstitutionPreference `yaml:"substitution_preferences,omitempty" json:"substitution_preferences,omitempty"`
}

// QuotaSpec defines quota limits.
type QuotaSpec struct {
	WindowSize         string `yaml:"window_size,omitempty" json:"window_size,omitempty"`
	MaxChunksPerWindow int    `yaml:"max_chunks_per_window,omitempty" json:"max_chunks_per_window,omitempty"`
	MaxTokensPerWindow int    `yaml:"max_tokens_per_window,omitempty" json:"max_tokens_per_window,omitempty"`
	PrincipalPattern   string `yaml:"principal_pattern,omitempty" json:"principal_pattern,omitempty"`
}

// RateLimitSpec defines rate limiting configuration.
type RateLimitSpec struct {
	RequestsPerSecond float64 `yaml:"requests_per_second,omitempty" json:"requests_per_second,omitempty"`
	BurstSize         int     `yaml:"burst_size,omitempty" json:"burst_size,omitempty"`
}

// SubstitutionPreference defines how to substitute content.
type SubstitutionPreference struct {
	SourceContentType string `yaml:"source_content_type,omitempty" json:"source_content_type,omitempty"`
	TargetContentType string `yaml:"target_content_type,omitempty" json:"target_content_type,omitempty"`
	Strategy          string `yaml:"strategy,omitempty" json:"strategy,omitempty"`
}
