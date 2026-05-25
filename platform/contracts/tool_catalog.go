package contracts

import (
	"strings"
	"unicode"
)

// ToolBackend identifies how a tool is executed.
type ToolBackend string

const (
	ToolBackendSubprocess ToolBackend = "subprocess"
	ToolBackendGoNative   ToolBackend = "go_native"
	ToolBackendMCP        ToolBackend = "mcp"
)

// ToolManifest describes a single tool definition loaded from relurpify_cfg.
type ToolManifest struct {
	Name          string                 `yaml:"name" json:"name"`
	Version       string                 `yaml:"version,omitempty" json:"version,omitempty"`
	Family        string                 `yaml:"family,omitempty" json:"family,omitempty"`
	Intent        []string               `yaml:"intent,omitempty" json:"intent,omitempty"`
	Description   string                 `yaml:"description,omitempty" json:"description,omitempty"`
	Guidance      ToolManifestGuidance   `yaml:"guidance,omitempty" json:"guidance,omitempty"`
	Parameters    []ToolParameter        `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Execution     ToolManifestExecution  `yaml:"execution" json:"execution"`
	Returns       ToolManifestReturns    `yaml:"returns,omitempty" json:"returns,omitempty"`
	Errors        map[string]string      `yaml:"errors,omitempty" json:"errors,omitempty"`
	Capability    ToolManifestCapability `yaml:"capability" json:"capability"`
	SourcePath    string                 `yaml:"-" json:"-"`
	CanonicalName string                 `yaml:"-" json:"-"`
}

// ToolManifestGuidance captures agent-facing usage hints.
type ToolManifestGuidance struct {
	UseWhen   []string `yaml:"use_when,omitempty" json:"use_when,omitempty"`
	AvoidWhen []string `yaml:"avoid_when,omitempty" json:"avoid_when,omitempty"`
}

// ToolManifestExecution describes the backend used to run a tool.
type ToolManifestExecution struct {
	Backend          ToolBackend                    `yaml:"backend" json:"backend"`
	Implementation   string                         `yaml:"implementation,omitempty" json:"implementation,omitempty"`
	Command          *ToolManifestCommand           `yaml:"command,omitempty" json:"command,omitempty"`
	PlatformVariants map[string]ToolManifestCommand `yaml:"platform_variants,omitempty" json:"platform_variants,omitempty"`
	Sandbox          *ToolManifestSandbox           `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
	Stdin            string                         `yaml:"stdin,omitempty" json:"stdin,omitempty"`
	DefaultArgs      []string                       `yaml:"default_args,omitempty" json:"default_args,omitempty"`
	AllowStdin       bool                           `yaml:"allow_stdin,omitempty" json:"allow_stdin,omitempty"`
	SupportsWorkdir  bool                           `yaml:"supports_workdir,omitempty" json:"supports_workdir,omitempty"`
	MCP              *ToolManifestMCP               `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}

// ToolManifestCommand describes a command template.
type ToolManifestCommand struct {
	Base  []string                    `yaml:"base,omitempty" json:"base,omitempty"`
	Args  []string                    `yaml:"args,omitempty" json:"args,omitempty"`
	Flags map[string]ToolManifestFlag `yaml:"flags,omitempty" json:"flags,omitempty"`
}

// ToolManifestFlag describes a boolean or conditional flag expansion.
type ToolManifestFlag struct {
	WhenTrue  []string `yaml:"when_true,omitempty" json:"when_true,omitempty"`
	WhenFalse []string `yaml:"when_false,omitempty" json:"when_false,omitempty"`
}

// ToolManifestSandbox captures execution sandbox constraints for a tool.
type ToolManifestSandbox struct {
	AllowedRoot    string `yaml:"allowed_root,omitempty" json:"allowed_root,omitempty"`
	TimeoutSeconds int    `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
	NetworkAccess  bool   `yaml:"network_access,omitempty" json:"network_access,omitempty"`
}

// ToolManifestMCP describes MCP execution routing.
type ToolManifestMCP struct {
	Server string `yaml:"server,omitempty" json:"server,omitempty"`
	Method string `yaml:"method,omitempty" json:"method,omitempty"`
}

// ToolManifestReturns captures the structured output shape.
type ToolManifestReturns struct {
	Type  string `yaml:"type,omitempty" json:"type,omitempty"`
	Shape Schema `yaml:"shape,omitempty" json:"shape,omitempty"`
}

// ToolManifestCapability maps the tool to capability classification.
type ToolManifestCapability struct {
	TrustClass  string   `yaml:"trust_class,omitempty" json:"trust_class,omitempty"`
	RiskClass   []string `yaml:"risk_class,omitempty" json:"risk_class,omitempty"`
	EffectClass []string `yaml:"effect_class,omitempty" json:"effect_class,omitempty"`
}

// NormalizeToolName canonicalizes tool identifiers for lookups.
func NormalizeToolName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-' || r == '.' || r == '/' || unicode.IsSpace(r):
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	return strings.ReplaceAll(out, "__", "_")
}
