package euclo

import (
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/persistence"
	contextports "codeburg.org/lexbit/relurpify/context/ports"
	"codeburg.org/lexbit/relurpify/execution/agentlifecycle"
	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/named/euclo/grounding"
	euclopolicy "codeburg.org/lexbit/relurpify/named/euclo/policy"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// EucloConfig configures the Euclo agent behavior.
type EucloConfig struct {
	// ThoughtRecipeDirs is reserved for future Euclo DSL thoughtrecipe directories.
	ThoughtRecipeDirs []string

	// BuiltinFamilies controls whether the standard keyword family set is registered.
	BuiltinFamilies bool

	// CapabilityClassifierModel overrides the model used for tier-2 classification.
	CapabilityClassifierModel model.LanguageModel

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

	// HITLBroker approves human-in-the-loop requests raised by the policy gate.
	// It is required to build the execution graph; a nil broker fails closed
	// rather than silently auto-approving.
	HITLBroker euclopolicy.HITLBroker

	// StateReground is the optional restart query over the knowledge layer's
	// grounded capture corpus (Wave 1 IF-2). When composed, recipe dispatch
	// re-grounds durable state.* captures before the recipe runs; when nil,
	// Euclo starts cold and emits exactly one info-level grounding.ports_unwired
	// boot event. Nil is a declared degraded mode, not a silent fallback.
	StateReground grounding.StateRegroundSource

	// TelemetrySink is the telemetry backend for execution events.
	// When nil, a no-op sink is used.
	TelemetrySink telemetry.Telemetry

	// CheckpointRepository stores materialized checkpoint artifacts.
	CheckpointRepository agentlifecycle.Repository

	// PersistenceWriter mirrors checkpoint payloads into the generic persistence layer.
	PersistenceWriter *persistence.Writer

	// LifecycleRepository receives persistent lifecycle records, including the
	// Selection Decision Record written for every dispatch (D11 §3.6.4). Nil is
	// a declared degraded mode: dispatch proceeds and no decision records are
	// persisted (provenance loss is surfaced by telemetry when persistence was
	// expected).
	LifecycleRepository contextports.LifecycleRepository

	// DryRun indicates whether to execute in dry-run mode.
	// When true, mutation-capable steps report intended actions without executing.
	DryRun bool

	// SuppressOutcomeFeedback prevents the outcome feedback frame from being emitted.
	SuppressOutcomeFeedback bool
}

// DefaultConfig returns the default Euclo configuration.
func DefaultConfig() EucloConfig {
	return EucloConfig{
		ThoughtRecipeDirs:       []string{},
		BuiltinFamilies:         true,
		MaxStreamTokens:         8192,
		DefaultStreamMode:       contextstream.ModeBlocking,
		WorkspaceIngestionMode:  "files_only",
		IngestionIncludeGlobs:   []string{},
		IngestionExcludeGlobs:   []string{},
		TelemetrySink:           nil,
		DryRun:                  false,
		SuppressOutcomeFeedback: false,
	}
}

// Option constructs configuration options for the agent.
// Defined in agent.go, re-exported here for convenience.
