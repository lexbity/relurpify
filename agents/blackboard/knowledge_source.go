package blackboard

import (
	"context"

	graph "codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// KnowledgeSourceSpec describes one specialist's scheduling and capability
// requirements. This keeps blackboard runtime metadata explicit without adding
// blackboard-specific behavior to framework/graph.
type KnowledgeSourceSpec struct {
	Name                 string
	Priority             int
	CooldownCycles       int
	RequiredCapabilities []core.CapabilitySelector
	Contract             graph.NodeContract
}

// KnowledgeSource is the interface every specialist must satisfy.
// A KS reads from the blackboard, performs focused work, and writes results
// back. The controller invokes CanActivate each cycle to determine eligibility
// before calling Execute.
type KnowledgeSource interface {
	// Name returns a stable identifier used for logging and priority ties.
	Name() string
	// CanActivate returns true when this KS has something to contribute in the
	// current blackboard state.
	CanActivate(bb *Blackboard) bool
	// Execute reads from bb, does work, and writes results back.
	// The semctx parameter provides typed access to pre-resolved semantic context.
	Execute(ctx context.Context, bb *Blackboard, tools *capability.Registry, model core.LanguageModel, semctx agentspec.AgentSemanticContext) error
	// Priority breaks ties when multiple KS can activate. Higher wins.
	Priority() int
}

// KnowledgeSourceSpecProvider allows a source to publish explicit runtime
// metadata without changing the base execution interface.
type KnowledgeSourceSpecProvider interface {
	KnowledgeSourceSpec() KnowledgeSourceSpec
}

// ResolvedKnowledgeSource wraps one KS with normalized runtime metadata.
type ResolvedKnowledgeSource struct {
	Source   KnowledgeSource
	Spec     KnowledgeSourceSpec
	Contract graph.NodeContract
}

// ResolveKnowledgeSource normalizes metadata and contract defaults for one KS.
func ResolveKnowledgeSource(source KnowledgeSource) ResolvedKnowledgeSource {
	spec := KnowledgeSourceSpec{
		Name:     source.Name(),
		Priority: source.Priority(),
		Contract: defaultKnowledgeSourceContract(),
	}
	if provider, ok := source.(KnowledgeSourceSpecProvider); ok {
		declared := provider.KnowledgeSourceSpec()
		if declared.Name != "" {
			spec.Name = declared.Name
		}
		if declared.Priority != 0 {
			spec.Priority = declared.Priority
		}
		if declared.CooldownCycles > 0 {
			spec.CooldownCycles = declared.CooldownCycles
		}
		if len(declared.RequiredCapabilities) > 0 {
			spec.RequiredCapabilities = append([]core.CapabilitySelector(nil), declared.RequiredCapabilities...)
		}
		if hasKnowledgeSourceContract(declared.Contract) {
			spec.Contract = declared.Contract
		}
	}
	if spec.Contract.RequiredCapabilities == nil && len(spec.RequiredCapabilities) > 0 {
		spec.Contract.RequiredCapabilities = append([]core.CapabilitySelector(nil), spec.RequiredCapabilities...)
	}
	if !hasStateBoundaryPolicy(spec.Contract.ContextPolicy) {
		spec.Contract.ContextPolicy = defaultKnowledgeSourceContract().ContextPolicy
	}
	return ResolvedKnowledgeSource{
		Source:   source,
		Spec:     spec,
		Contract: spec.Contract,
	}
}

func hasKnowledgeSourceContract(contract graph.NodeContract) bool {
	return len(contract.RequiredCapabilities) > 0 ||
		contract.SideEffectClass != "" ||
		contract.Idempotency != "" ||
		contract.PreferredPlacement != "" ||
		contract.MaxRiskClass != "" ||
		contract.RequiredTrustClass != "" ||
		contract.Recoverability != "" ||
		contract.CheckpointPolicy != "" ||
		hasStateBoundaryPolicy(contract.ContextPolicy)
}

func hasStateBoundaryPolicy(policy core.StateBoundaryPolicy) bool {
	return len(policy.ReadKeys) > 0 ||
		len(policy.WriteKeys) > 0 ||
		policy.AllowHistoryAccess ||
		len(policy.AllowedMemoryClasses) > 0 ||
		len(policy.AllowedDataClasses) > 0 ||
		policy.MaxStateEntryBytes != 0 ||
		policy.MaxInlineCollectionItems != 0 ||
		policy.PreferArtifactReferences
}

func defaultKnowledgeSourceContract() graph.NodeContract {
	return graph.NodeContract{
		SideEffectClass:  graph.SideEffectContext,
		Idempotency:      graph.IdempotencyReplaySafe,
		Recoverability:   graph.NodeRecoverabilityInProcess,
		CheckpointPolicy: graph.CheckpointPolicyPreferred,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "blackboard.*", "bkc.*", "ast.*"},
			WriteKeys:                []string{"blackboard.*"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState, core.StateDataClassRoutingFlag},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 32,
		},
	}
}
