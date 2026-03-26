package rewoo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/agents/internal/workflowutil"
	"github.com/lexcodex/relurpify/framework/authorization"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
)

// Initialize configures the agent with framework services.
// It wires ContextPolicy and PermissionManager with sensible defaults if not pre-injected.
func (a *RewooAgent) Initialize(cfg *core.Config) error {
	a.Config = cfg
	if a.Tools == nil {
		a.Tools = NewCapabilityRegistry()
	}

	// Set defaults for uninitialized options
	opts := a.Options
	if opts.MaxReplanAttempts < 0 {
		opts.MaxReplanAttempts = 0
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 20
	}
	if opts.OnFailure == "" {
		opts.OnFailure = StepOnFailureSkip
	}
	if opts.GraphConfig.MaxParallelSteps <= 0 {
		opts.GraphConfig.MaxParallelSteps = 4
	}
	if opts.GraphConfig.MaxNodeVisits <= 0 {
		opts.GraphConfig.MaxNodeVisits = 1024
	}
	a.Options = opts

	// Set up ContextPolicy if not injected
	if a.ContextPolicy == nil {
		if err := a.initializeContextPolicy(cfg); err != nil {
			return fmt.Errorf("rewoo: context policy init failed: %w", err)
		}
	}

	// Set up PermissionManager if not injected
	if a.PermissionManager == nil {
		if err := a.initializePermissionManager(cfg); err != nil {
			return fmt.Errorf("rewoo: permission manager init failed: %w", err)
		}
	}

	a.initialised = true
	return nil
}

// initializeContextPolicy creates a default ContextPolicy with adaptive strategy.
func (a *RewooAgent) initializeContextPolicy(cfg *core.Config) error {
	// Select strategy based on config
	var strategy contextmgr.ContextStrategy
	ctxCfg := a.Options.ContextConfig
	switch strings.ToLower(ctxCfg.StrategyName) {
	case "conservative":
		strategy = contextmgr.NewConservativeStrategy()
	case "aggressive":
		strategy = contextmgr.NewAggressiveStrategy()
	default:
		strategy = contextmgr.NewAdaptiveStrategy()
	}

	// Build preferences from options
	preferences := contextmgr.ContextPolicyPreferences{
		MinHistorySize:       5,
		CompressionThreshold: 0.8,
	}
	if ctxCfg.MinHistorySize > 0 {
		preferences.MinHistorySize = ctxCfg.MinHistorySize
	}
	if ctxCfg.CompressionThreshold > 0 && ctxCfg.CompressionThreshold <= 1 {
		preferences.CompressionThreshold = ctxCfg.CompressionThreshold
	}

	// Map detail level
	detailLevel := contextmgr.DetailDetailed
	switch strings.ToLower(ctxCfg.PreferredDetailLevel) {
	case "minimal":
		detailLevel = contextmgr.DetailMinimal
	case "concise":
		detailLevel = contextmgr.DetailConcise
	case "full":
		detailLevel = contextmgr.DetailFull
	}
	preferences.PreferredDetailLevel = detailLevel

	// Get agent spec from config if available
	var spec *core.AgentContextSpec
	if cfg != nil && cfg.AgentSpec != nil {
		spec = &cfg.AgentSpec.Context
	}

	// Create the policy
	policy := contextmgr.NewContextPolicy(contextmgr.ContextPolicyConfig{
		Strategy:      strategy,
		LanguageModel: a.Model,
		MemoryStore:   a.Memory,
		IndexManager:  a.IndexManager,
		SearchEngine:  a.SearchEngine,
		Preferences:   preferences,
	}, spec)

	// Set budget reservations
	system := ctxCfg.BudgetSystemTokens
	if system <= 0 {
		system = 800
	}
	tools := ctxCfg.BudgetToolTokens
	if tools <= 0 {
		tools = 1500
	}
	output := ctxCfg.BudgetOutputTokens
	if output <= 0 {
		output = 1000
	}
	policy.Budget.SetReservations(system, tools, output)

	a.ContextPolicy = policy
	return nil
}

// initializePermissionManager creates a default PermissionManager.
// It builds a minimal PermissionSet that allows workspace read/write and all registered tools.
func (a *RewooAgent) initializePermissionManager(cfg *core.Config) error {
	permCfg := a.Options.PermConfig
	workspacePath := permCfg.WorkspacePath
	if workspacePath == "" {
		workspacePath = "."
	}

	// Build default permission set using helper
	perm := DefaultPermissionSet(a.Tools, workspacePath)

	// Create an audit logger backed by memory store if available
	var auditLogger core.AuditLogger
	surfaces := workflowutil.ResolveRuntimeSurfaces(a.Memory)
	if surfaces.Runtime != nil {
		auditLogger = NewRewooAuditLogger(surfaces.Runtime)
	} else {
		// Fallback to no-op audit logger if no memory store
		auditLogger = &noopAuditLogger{}
	}

	// Create HITL broker if enabled
	var hitlProvider authorization.HITLProvider
	if permCfg.EnableHITL {
		hitlProvider = authorization.NewHITLBroker(5 * time.Minute)
	}

	// Create the permission manager
	pm, err := authorization.NewPermissionManager(
		workspacePath,
		perm,
		auditLogger,
		hitlProvider,
	)
	if err != nil {
		return err
	}

	// Set default policy
	defaultPolicy := core.AgentPermissionAsk
	switch strings.ToLower(permCfg.DefaultPolicy) {
	case "allow":
		defaultPolicy = core.AgentPermissionAllow
	case "deny":
		defaultPolicy = core.AgentPermissionDeny
	}
	pm.SetDefaultPolicy(defaultPolicy)

	// Wire into the registry
	a.Tools.UsePermissionManager("rewoo", pm)

	a.PermissionManager = pm
	return nil
}

// noopAuditLogger implements core.AuditLogger as a no-op (used when no memory store).
type noopAuditLogger struct{}

func (n *noopAuditLogger) Log(_ context.Context, _ core.AuditRecord) error {
	return nil
}

func (n *noopAuditLogger) Query(_ context.Context, _ core.AuditQuery) ([]core.AuditRecord, error) {
	return nil, nil
}

// NewCapabilityRegistry returns a new empty capability registry.
func NewCapabilityRegistry() *capability.Registry {
	return capability.NewRegistry()
}
