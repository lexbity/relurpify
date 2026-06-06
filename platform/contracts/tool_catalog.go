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
	ToolBackendComposite  ToolBackend = "composite"
)

// ToolRateLimit configures per-tool rate limiting for automatic enforcement.
type ToolRateLimit struct {
	PerSecond float64 `yaml:"per_second,omitempty" json:"per_second,omitempty"`
	Burst     int     `yaml:"burst,omitempty" json:"burst,omitempty"`
}

// ToolManifestComposition defines sub-tools executed sequentially for
// composite tool definitions.
type ToolManifestComposition struct {
	Steps []ToolManifestCompositionStep `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// ToolManifestCompositionStep is a single sub-step within a composite tool.
type ToolManifestCompositionStep struct {
	Tool  string         `yaml:"tool" json:"tool"`
	Args  map[string]any `yaml:"args,omitempty" json:"args,omitempty"`
	Alias string         `yaml:"alias,omitempty" json:"alias,omitempty"` // variable name for step output
}

// ToolManifest describes a single tool definition loaded from relurpify_cfg.
type ToolManifest struct {
	Name          string                    `yaml:"name" json:"name"`
	Version       string                    `yaml:"version,omitempty" json:"version,omitempty"`
	Family        string                    `yaml:"family,omitempty" json:"family,omitempty"`
	Intent        []string                  `yaml:"intent,omitempty" json:"intent,omitempty"`
	Description   string                    `yaml:"description,omitempty" json:"description,omitempty"`
	Guidance      ToolManifestGuidance      `yaml:"guidance,omitempty" json:"guidance,omitempty"`
	Parameters    []ToolParameter           `yaml:"parameters,omitempty" json:"parameters,omitempty"`
	Execution     ToolManifestExecution     `yaml:"execution" json:"execution"`
	Returns       ToolManifestReturns       `yaml:"returns,omitempty" json:"returns,omitempty"`
	Errors        map[string]string         `yaml:"errors,omitempty" json:"errors,omitempty"`
	Capability    ToolManifestCapability    `yaml:"capability" json:"capability"`
	RateLimit     *ToolRateLimit            `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	Composition   *ToolManifestComposition  `yaml:"composition,omitempty" json:"composition,omitempty"`
	Telemetry     *ToolManifestTelemetry    `yaml:"telemetry,omitempty" json:"telemetry,omitempty"`
	SourcePath    string                    `yaml:"-" json:"-"`
	CanonicalName string                    `yaml:"-" json:"-"`
}

// ToolManifestTelemetry provides observability hints for tool invocations.
// All fields are optional; defaults are derived from the manifest name.
type ToolManifestTelemetry struct {
	// SpanName overrides the default OTel span name (tool.<name>).
	SpanName string `yaml:"span_name,omitempty" json:"span_name,omitempty"`
	// ExtraAttributes lists parameter names whose values are safe to emit
	// as span attributes (allowlist — secrets never leak).
	ExtraAttributes []string `yaml:"extra_attributes,omitempty" json:"extra_attributes,omitempty"`
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
	SupportsWorkdir  bool `yaml:"supports_workdir,omitempty" json:"supports_workdir,omitempty"`
}

// ToolManifestCommand describes a command template.
type ToolManifestCommand struct {
	Base  []string                    `yaml:"base,omitempty" json:"base,omitempty"`
	Args  []string                    `yaml:"args,omitempty" json:"args,omitempty"`
	Flags map[string]ToolManifestFlag `yaml:"flags,omitempty" json:"flags,omitempty"`
}

// ToolManifestFlag describes a boolean or typed flag expansion.
//
// A flag must use exactly one form:
//   - Boolean form: sets WhenTrue/WhenFalse based on a boolean parameter value.
//   - Typed form: binds a parameter value via the Param field, formatting it
//     into one or two argv tokens according to Style.
//
// In the typed form:
//   - Param names the manifest parameter whose value drives the flag.
//   - Style is "equals" (--output=VALUE, one token) or "separate" (--output VALUE,
//     two tokens). Empty defaults to "equals".
//   - Type hints at the parameter's schema type ("string", "integer", etc.).
//   - Repeat, when true, emits one flag instance per element for array values.
type ToolManifestFlag struct {
	WhenTrue  []string `yaml:"when_true,omitempty" json:"when_true,omitempty"`
	WhenFalse []string `yaml:"when_false,omitempty" json:"when_false,omitempty"`

	Param  string `yaml:"param,omitempty" json:"param,omitempty"`
	Style  string `yaml:"style,omitempty" json:"style,omitempty"`
	Type   string `yaml:"type,omitempty" json:"type,omitempty"`
	Repeat bool   `yaml:"repeat,omitempty" json:"repeat,omitempty"`
}

// ToolManifestFlagStyle enumerates the supported expansion styles for typed flags.
const (
	FlagStyleEquals   = "equals"   // --key=value   (single argv token)
	FlagStyleSeparate = "separate" // --key  value  (two argv tokens)
)

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

// ToolManifestReturns captures the structured output shape and chunking hints.
type ToolManifestReturns struct {
	Type     string                      `yaml:"type,omitempty" json:"type,omitempty"`
	Shape    Schema                      `yaml:"shape,omitempty" json:"shape,omitempty"`
	Chunking *ToolManifestReturnsChunking `yaml:"chunking,omitempty" json:"chunking,omitempty"`
}

// ToolManifestReturnsChunking declares how structured stdout is decomposed into
// context chunks. When absent the entire output is treated as a single opaque blob.
type ToolManifestReturnsChunking struct {
	Mode      string   `yaml:"mode,omitempty" json:"mode,omitempty"`           // whole | per_item | per_field
	ItemPath  string   `yaml:"item_path,omitempty" json:"item_path,omitempty"` // JSONPath-ish selector for per_item
	RefFields []string `yaml:"ref_fields,omitempty" json:"ref_fields,omitempty"` // fields promoted to retrieval refs
}

// ToolManifestChunkingMode enumerates the supported chunking strategies.
const (
	ChunkingModeWhole    = "whole"     // single opaque blob (default)
	ChunkingModePerItem  = "per_item"  // one chunk per array element at item_path
	ChunkingModePerField = "per_field" // one chunk per top-level field
)

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
