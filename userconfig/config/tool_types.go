package config

import (
	"strings"
	"unicode"
)

// ToolParameterType enumerates the allowed parameter types.
type ToolParameterType string

const (
	ToolParamString  ToolParameterType = "string"
	ToolParamInteger ToolParameterType = "integer"
	ToolParamNumber  ToolParameterType = "number"
	ToolParamBoolean ToolParameterType = "boolean"
	ToolParamArray   ToolParameterType = "array"
	ToolParamObject  ToolParameterType = "object"
)

// ToolParameter describes a single parameter accepted by a tool.
type ToolParameter struct {
	Name        string            `yaml:"name" json:"name"`
	Type        ToolParameterType `yaml:"type" json:"type"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool              `yaml:"required,omitempty" json:"required,omitempty"`
	Default     any               `yaml:"default,omitempty" json:"default,omitempty"`
}

// Flag expansion styles for tool manifest flags.
const (
	FlagStyleEquals   = "equals"
	FlagStyleSeparate = "separate"
)

// ToolBackend identifies how a tool is executed.
type ToolBackend string

const (
	ToolBackendSubprocess ToolBackend = "subprocess"
	ToolBackendGoNative   ToolBackend = "go_native"
	ToolBackendComposite  ToolBackend = "composite"
)

// ToolRateLimit configures per-tool rate limiting.
type ToolRateLimit struct {
	PerSecond float64 `yaml:"per_second,omitempty" json:"per_second,omitempty"`
	Burst     int     `yaml:"burst,omitempty" json:"burst,omitempty"`
}

// ToolManifest describes a single tool definition loaded from relurpify_cfg.
type ToolManifest struct {
	Name          string                   `yaml:"name" json:"name"`
	Version       string                   `yaml:"version,omitempty" json:"version,omitempty"`
	Family        string                   `yaml:"family,omitempty" json:"family,omitempty"`
	Intent        []string                 `yaml:"intent,omitempty" json:"intent,omitempty"`
	Description   string                   `yaml:"description,omitempty" json:"description,omitempty"`
	Guidance      *ToolManifestGuidance    `yaml:"guidance,omitempty" json:"guidance,omitempty"`
	Parameters    []ToolParameter          `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Execution     ToolManifestExecution    `yaml:"execution" json:"execution"`
	Returns       ToolManifestReturns      `yaml:"returns,omitempty" json:"returns,omitempty"`
	Errors        map[string]string        `yaml:"errors,omitempty" json:"errors,omitempty"`
	Capability    ToolManifestCapability   `yaml:"capability" json:"capability"`
	RateLimit     *ToolRateLimit           `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	Composition   *ToolManifestComposition `yaml:"composition,omitempty" json:"composition,omitempty"`
	SourcePath    string                   `yaml:"-" json:"-"`
	CanonicalName string                   `yaml:"-" json:"-"`
}

// ToolManifestSandbox captures execution sandbox constraints for a tool.
type ToolManifestSandbox struct {
	AllowedRoot    string   `yaml:"allowed_root,omitempty" json:"allowed_root,omitempty"`
	TimeoutSeconds int      `yaml:"timeout_seconds,omitempty" json:"timeout_seconds,omitempty"`
	NetworkAccess  bool     `yaml:"network_access,omitempty" json:"network_access,omitempty"`
	AllowFlags     bool     `yaml:"allow_flags,omitempty" json:"allow_flags,omitempty"`
	MemoryMB       int64    `yaml:"memory_mb,omitempty" json:"memory_mb,omitempty"`
	PidsLimit      int64    `yaml:"pids_limit,omitempty" json:"pids_limit,omitempty"`
	CPUs           float64  `yaml:"cpus,omitempty" json:"cpus,omitempty"`
	AllowHosts     []string `yaml:"allow_hosts,omitempty" json:"allow_hosts,omitempty"`
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
}

// ToolManifestCommand describes a command template.
type ToolManifestCommand struct {
	Base  []string                    `yaml:"base,omitempty" json:"base,omitempty"`
	Args  []string                    `yaml:"args,omitempty" json:"args,omitempty"`
	Flags map[string]ToolManifestFlag `yaml:"flags,omitempty" json:"flags,omitempty"`
}

// ToolManifestTelemetry provides observability hints for tool invocations.
type ToolManifestTelemetry struct {
	SpanName        string   `yaml:"span_name,omitempty" json:"span_name,omitempty"`
	ExtraAttributes []string `yaml:"extra_attributes,omitempty" json:"extra_attributes,omitempty"`
}

// ToolManifestGuidance captures agent-facing usage hints.
type ToolManifestGuidance struct {
	UseWhen   []string `yaml:"use_when,omitempty" json:"use_when,omitempty"`
	AvoidWhen []string `yaml:"avoid_when,omitempty" json:"avoid_when,omitempty"`
}

// Chunking mode constants.
const (
	ChunkingModeWhole    = "whole"
	ChunkingModePerItem  = "per_item"
	ChunkingModePerField = "per_field"
)

// ToolManifestFlag describes a boolean or typed flag expansion.
type ToolManifestFlag struct {
	WhenTrue  []string `yaml:"when_true,omitempty" json:"when_true,omitempty"`
	WhenFalse []string `yaml:"when_false,omitempty" json:"when_false,omitempty"`
	Param     string   `yaml:"param,omitempty" json:"param,omitempty"`
	Style     string   `yaml:"style,omitempty" json:"style,omitempty"`
	Type      string   `yaml:"type,omitempty" json:"type,omitempty"`
	Repeat    bool     `yaml:"repeat,omitempty" json:"repeat,omitempty"`
}

// ToolManifestComposition defines sub-tools executed sequentially.
type ToolManifestComposition struct {
	Steps []ToolManifestCompositionStep `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// ToolManifestCompositionStep is a single sub-step within a composite tool.
type ToolManifestCompositionStep struct {
	Tool  string         `yaml:"tool" json:"tool"`
	Args  map[string]any `yaml:"args,omitempty" json:"args,omitempty"`
	Alias string         `yaml:"alias,omitempty" json:"alias,omitempty"`
}

// ToolManifestReturnsChunking declares how structured stdout is decomposed.
type ToolManifestReturnsChunking struct {
	Mode      string   `yaml:"mode,omitempty" json:"mode,omitempty"`
	ItemPath  string   `yaml:"item_path,omitempty" json:"item_path,omitempty"`
	RefFields []string `yaml:"ref_fields,omitempty" json:"ref_fields,omitempty"`
}

// ToolManifestReturns captures the structured output shape.
type ToolManifestReturns struct {
	Type     string                       `yaml:"type,omitempty" json:"type,omitempty"`
	Chunking *ToolManifestReturnsChunking `yaml:"chunking,omitempty" json:"chunking,omitempty"`
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
