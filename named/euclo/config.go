package euclo

import (
	"time"

	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// EucloConfig configures the Euclo agent behavior.
type EucloConfig struct {
	// RecipeDirs are filesystem directories scanned for .yaml thought recipe files.
	RecipeDirs []string

	// BuiltinFamilies controls whether the standard keyword family set is registered.
	BuiltinFamilies bool

	// CapabilityClassifierModel overrides the model used for tier-2 classification.
	CapabilityClassifierModel contracts.LanguageModel

	// MaxStreamTokens is the token budget passed to context stream requests.
	MaxStreamTokens int

	// DefaultStreamMode controls blocking vs background for intake stream requests.
	// Background mode is only safe when tier-2 classification is bypassed (e.g.
	// context_hint override is present). When DefaultStreamMode is ModeBackground
	// and no context_hint is present, Execute logs a warning and falls back to
	// ModeBlocking for the tier-2 classification step.
	DefaultStreamMode contextstream.Mode

	// WorkspaceIngestionMode controls the default workspace scanning behavior.
	// "files_only" (default): only ingest explicitly selected user files.
	// "incremental": scan workspace incrementally (git-diff since last run).
	// "full": full workspace scan; expensive, use only when explicitly requested.
	WorkspaceIngestionMode string

	// IngestionIncludeGlobs and IngestionExcludeGlobs filter workspace scans.
	IngestionIncludeGlobs []string
	IngestionExcludeGlobs []string

	// HITLTimeout is the maximum duration Euclo waits for a HITL decision.
	// Zero uses the HITLBroker's default.
	HITLTimeout time.Duration

	// TelemetrySink is the telemetry backend for execution events.
	// When nil, a no-op sink is used.
	TelemetrySink core.Telemetry

	// DryRun indicates whether to execute in dry-run mode.
	// When true, mutation-capable steps report intended actions without executing.
	DryRun bool

	// SuppressOutcomeFeedback prevents the outcome feedback frame from being emitted.
	SuppressOutcomeFeedback bool
}

// DefaultConfig returns the default Euclo configuration.
func DefaultConfig() EucloConfig {
	return EucloConfig{
		RecipeDirs:              []string{},
		BuiltinFamilies:         true,
		MaxStreamTokens:         8192,
		DefaultStreamMode:       contextstream.ModeBlocking,
		WorkspaceIngestionMode:  "files_only",
		IngestionIncludeGlobs:   []string{},
		IngestionExcludeGlobs:   []string{},
		HITLTimeout:             5 * time.Minute,
		TelemetrySink:           nil,
		DryRun:                  false,
		SuppressOutcomeFeedback: false,
	}
}

// Option constructs configuration options for the agent.
// Defined in agent.go, re-exported here for convenience.
