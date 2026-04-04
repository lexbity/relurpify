package ayenitd

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	fsandbox "github.com/lexcodex/relurpify/framework/sandbox"
)

// WorkspaceConfig is the resolved configuration produced from CLI flags, YAML
// workspace config, and environment. It is the input to ayenitd.Open().
type WorkspaceConfig struct {
	// Required
	Workspace      string // absolute path to workspace root
	ManifestPath   string // agent manifest YAML
	OllamaEndpoint string
	OllamaModel    string // overrides manifest if non-empty

	// Optional
	ConfigPath          string // workspace config YAML (relurpify.yaml etc)
	AgentsDir           string // named agent definition overlay directory
	AgentName           string // initial agent to load
	LogPath             string
	TelemetryPath       string
	EventsPath          string
	MemoryPath          string
	MaxIterations       int
	SkipASTIndex        bool
	HITLTimeout         time.Duration
	AuditLimit          int
	Sandbox             fsandbox.SandboxConfig
	DebugLLM            bool
	DebugAgent          bool
	AllowedCapabilities []core.CapabilitySelector
	// ReindexInterval, if non-zero, schedules periodic AST re-indexing.
	// Zero (default) disables the background re-index job.
	ReindexInterval time.Duration
}

// AgentLabel returns the agent name to use for configuration.
func (cfg WorkspaceConfig) AgentLabel() string {
	if cfg.AgentName != "" {
		return cfg.AgentName
	}
	return "default"
}
