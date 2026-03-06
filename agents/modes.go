package agents

import (
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/search"
)

// Mode enumerates the supported execution profiles for the coding agent.
type Mode string

// ControlFlow selects the execution runtime behind a mode profile.
type ControlFlow string

const (
	ControlFlowReAct     ControlFlow = "react"
	ControlFlowPipeline  ControlFlow = "pipeline"
	ControlFlowArchitect ControlFlow = "architect"
)

const (
	ModeCode      Mode = "code"
	ModeArchitect Mode = "architect"
	ModeAsk       Mode = "ask"
	ModeDebug     Mode = "debug"
	ModeDocument  Mode = "docs"
	defaultMode        = ModeCode
)

// ToolScope defines the rough permission envelope for a mode.
type ToolScope struct {
	AllowRead    bool
	AllowWrite   bool
	AllowExecute bool
	AllowNetwork bool
}

// ModeProfile bundles temperature, tooling envelope, and documentation for a
// mode so the orchestrator can enforce consistent behavior.
type ModeProfile struct {
	Name         Mode
	Title        string
	Description  string
	Temperature  float64
	ControlFlow  ControlFlow
	Capabilities []core.Capability
	ToolScope    ToolScope
	Restrictions []string

	ContextProfile    ContextProfile
	PreferredStrategy string
}

// ContextProfile defines context preferences for a mode.
type ContextProfile struct {
	PreferredDetailLevel DetailLevel
	MaxWorkingSetSize    int
	MaxConciseFiles      int
	CompressionThreshold float64
	MinHistorySize       int
	SearchMode           search.SearchMode
	MaxSearchResults     int
	PreloadPatterns      []string
	PreloadDependencies  bool
	DependencyDepth      int
	LoadASTUpfront       bool
	PreferSignatures     bool
	UseProjectMemory     bool
	UseGlobalMemory      bool
	MemoryQueryDepth     int
}

// defaultModeProfiles returns the baked-in description for every agent mode so
// the CLI can operate even before user manifests override the settings.
func defaultModeProfiles() map[Mode]ModeProfile {
	profiles := make(map[Mode]ModeProfile, len(ModeProfiles))
	for mode, profile := range ModeProfiles {
		profiles[mode] = profile
	}
	return profiles
}

// ModeProfiles stores built-in profiles keyed by mode.
var ModeProfiles = map[Mode]ModeProfile{
	ModeCode: {
		Name:        ModeCode,
		Title:       "Code Mode",
		Description: "General-purpose development with read/write/execute access.",
		Temperature: 0.3,
		ControlFlow: ControlFlowReAct,
		Capabilities: []core.Capability{
			core.CapabilityPlan,
			core.CapabilityCode,
			core.CapabilityExplain,
			core.CapabilityRefactor,
		},
		ToolScope: ToolScope{
			AllowRead:    true,
			AllowWrite:   true,
			AllowExecute: true,
			AllowNetwork: false,
		},
		ContextProfile: ContextProfile{
			PreferredDetailLevel: DetailDetailed,
			MaxWorkingSetSize:    10,
			MaxConciseFiles:      30,
			CompressionThreshold: 0.8,
			MinHistorySize:       5,
			SearchMode:           search.SearchHybrid,
			MaxSearchResults:     15,
			PreloadPatterns:      []string{"**/*.go", "**/*.py"},
			PreloadDependencies:  true,
			DependencyDepth:      1,
			LoadASTUpfront:       true,
			PreferSignatures:     false,
			UseProjectMemory:     true,
			UseGlobalMemory:      false,
			MemoryQueryDepth:     5,
		},
		PreferredStrategy: "adaptive",
	},
	ModeArchitect: {
		Name:        ModeArchitect,
		Title:       "Architect Mode",
		Description: "High-level architecture planning with read-only tools.",
		Temperature: 0.1,
		ControlFlow: ControlFlowArchitect,
		Capabilities: []core.Capability{
			core.CapabilityPlan,
			core.CapabilityExplain,
		},
		ToolScope: ToolScope{
			AllowRead:    true,
			AllowWrite:   false,
			AllowExecute: false,
			AllowNetwork: false,
		},
		Restrictions: []string{
			"No filesystem writes",
			"No shell command execution",
		},
		ContextProfile: ContextProfile{
			PreferredDetailLevel: DetailConcise,
			MaxWorkingSetSize:    5,
			MaxConciseFiles:      100,
			CompressionThreshold: 0.9,
			MinHistorySize:       10,
			SearchMode:           search.SearchSemantic,
			MaxSearchResults:     30,
			PreloadPatterns:      []string{"**/*"},
			PreloadDependencies:  false,
			DependencyDepth:      0,
			LoadASTUpfront:       true,
			PreferSignatures:     true,
			UseProjectMemory:     true,
			UseGlobalMemory:      true,
			MemoryQueryDepth:     10,
		},
		PreferredStrategy: "conservative",
	},
	ModeAsk: {
		Name:        ModeAsk,
		Title:       "Ask Mode",
		Description: "Information retrieval, explanation, and documentation lookup.",
		Temperature: 0.2,
		ControlFlow: ControlFlowReAct,
		Capabilities: []core.Capability{
			core.CapabilityExplain,
		},
		ToolScope: ToolScope{
			AllowRead:    true,
			AllowWrite:   false,
			AllowExecute: false,
			AllowNetwork: false,
		},
		ContextProfile: ContextProfile{
			PreferredDetailLevel: DetailConcise,
			MaxWorkingSetSize:    0,
			MaxConciseFiles:      50,
			CompressionThreshold: 0.85,
			MinHistorySize:       5,
			SearchMode:           search.SearchHybrid,
			MaxSearchResults:     20,
			PreloadDependencies:  false,
			DependencyDepth:      0,
			LoadASTUpfront:       true,
			PreferSignatures:     true,
			UseProjectMemory:     true,
			UseGlobalMemory:      true,
			MemoryQueryDepth:     10,
		},
		PreferredStrategy: "aggressive",
	},
	ModeDebug: {
		Name:        ModeDebug,
		Title:       "Debug Mode",
		Description: "Focused on diagnostics, log analysis, and running tests.",
		Temperature: 0.1,
		ControlFlow: ControlFlowReAct,
		Capabilities: []core.Capability{
			core.CapabilityExplain,
			core.CapabilityExecute,
		},
		ToolScope: ToolScope{
			AllowRead:    true,
			AllowWrite:   true,
			AllowExecute: true,
			AllowNetwork: false,
		},
		ContextProfile: ContextProfile{
			PreferredDetailLevel: DetailFull,
			MaxWorkingSetSize:    5,
			MaxConciseFiles:      20,
			CompressionThreshold: 0.75,
			MinHistorySize:       3,
			SearchMode:           search.SearchKeyword,
			MaxSearchResults:     10,
			PreloadDependencies:  true,
			DependencyDepth:      2,
			LoadASTUpfront:       false,
			PreferSignatures:     false,
			UseProjectMemory:     true,
			UseGlobalMemory:      false,
			MemoryQueryDepth:     3,
		},
		PreferredStrategy: "aggressive",
	},
	ModeDocument: {
		Name:        ModeDocument,
		Title:       "Documentation Mode",
		Description: "Produces README and API docs; writes limited to doc files.",
		Temperature: 0.4,
		ControlFlow: ControlFlowReAct,
		Capabilities: []core.Capability{
			core.CapabilityExplain,
			core.CapabilityPlan,
		},
		ToolScope: ToolScope{
			AllowRead:    true,
			AllowWrite:   true,
			AllowExecute: false,
			AllowNetwork: false,
		},
		Restrictions: []string{
			"Write operations restricted to documentation paths.",
		},
		ContextProfile: ContextProfile{
			PreferredDetailLevel: DetailDetailed,
			MaxWorkingSetSize:    15,
			MaxConciseFiles:      40,
			CompressionThreshold: 0.8,
			MinHistorySize:       5,
			SearchMode:           search.SearchSemantic,
			MaxSearchResults:     25,
			PreloadPatterns:      []string{"**/*.go", "**/*.md"},
			PreloadDependencies:  false,
			DependencyDepth:      0,
			LoadASTUpfront:       true,
			PreferSignatures:     true,
			UseProjectMemory:     true,
			UseGlobalMemory:      false,
			MemoryQueryDepth:     5,
		},
		PreferredStrategy: "balanced",
	},
}

// GetStrategyForMode returns a context strategy tuned to the mode.
func GetStrategyForMode(mode Mode) ContextStrategy {
	profile, ok := ModeProfiles[mode]
	if !ok {
		profile = ModeProfiles[defaultMode]
	}
	switch profile.PreferredStrategy {
	case "aggressive":
		return NewAggressiveStrategy()
	case "conservative":
		return NewConservativeStrategy()
	case "adaptive", "balanced":
		return NewAdaptiveStrategy()
	default:
		return NewAdaptiveStrategy()
	}
}
