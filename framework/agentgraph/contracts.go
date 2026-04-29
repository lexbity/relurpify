package agentgraph

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// SideEffectClass describes the replay sensitivity of a node's execution.
type SideEffectClass string

const (
	SideEffectNone     SideEffectClass = "none"
	SideEffectContext  SideEffectClass = "context"
	SideEffectLocal    SideEffectClass = "local"
	SideEffectExternal SideEffectClass = "external"
	SideEffectHuman    SideEffectClass = "human"
)

// IdempotencyClass describes whether re-running a node is expected to be safe.
type IdempotencyClass string

const (
	IdempotencyUnknown    IdempotencyClass = "unknown"
	IdempotencyReplaySafe IdempotencyClass = "replay-safe"
	IdempotencySingleShot IdempotencyClass = "single-shot"
)

// PlacementPreference guides deterministic preflight placement selection.
type PlacementPreference string

const (
	PlacementPreferenceAny    PlacementPreference = "any"
	PlacementPreferenceLocal  PlacementPreference = "local"
	PlacementPreferenceRemote PlacementPreference = "remote"
	PlacementPreferenceSticky PlacementPreference = "sticky-session"
)

// CheckpointPolicyClass models whether persisted recovery is optional or required.
type CheckpointPolicyClass string

const (
	CheckpointPolicyNone      CheckpointPolicyClass = "none"
	CheckpointPolicyPreferred CheckpointPolicyClass = "preferred"
	CheckpointPolicyRequired  CheckpointPolicyClass = "required"
)

// NodeRecoverability models the recovery guarantees expected for a node.
type NodeRecoverability string

const (
	NodeRecoverabilityNone      NodeRecoverability = "none"
	NodeRecoverabilityInProcess NodeRecoverability = "in-process"
	NodeRecoverabilityPersisted NodeRecoverability = "persisted"
)

// NodeContract describes execution requirements, replay semantics, and state
// boundaries for a node. Placement, recoverability, and checkpoint policy are
// contract metadata, not runtime persistence ownership.
type NodeContract struct {
	RequiredCapabilities []core.CapabilitySelector `json:"required_capabilities,omitempty" yaml:"required_capabilities,omitempty"`
	SideEffectClass      SideEffectClass           `json:"side_effect_class,omitempty" yaml:"side_effect_class,omitempty"`
	Idempotency          IdempotencyClass          `json:"idempotency,omitempty" yaml:"idempotency,omitempty"`

	PreferredPlacement PlacementPreference      `json:"preferred_placement,omitempty" yaml:"preferred_placement,omitempty"`
	MaxRiskClass       core.RiskClass           `json:"max_risk_class,omitempty" yaml:"max_risk_class,omitempty"`
	RequiredTrustClass core.TrustClass          `json:"required_trust_class,omitempty" yaml:"required_trust_class,omitempty"`
	Recoverability     NodeRecoverability       `json:"recoverability,omitempty" yaml:"recoverability,omitempty"`
	CheckpointPolicy   CheckpointPolicyClass    `json:"checkpoint_policy,omitempty" yaml:"checkpoint_policy,omitempty"`
	ContextPolicy      core.StateBoundaryPolicy `json:"context_policy,omitempty" yaml:"context_policy,omitempty"`
}

// ContractNode extends Node with an explicit execution contract.
type ContractNode interface {
	Node
	Contract() NodeContract
}

// ResolveNodeContract returns the node contract, falling back to a default
// contract for nodes that do not implement ContractNode.
func ResolveNodeContract(node Node) NodeContract {
	if node == nil {
		return NodeContract{}
	}
	if contractNode, ok := node.(ContractNode); ok {
		return contractNode.Contract()
	}
	return defaultNodeContract(node)
}

func defaultNodeContract(node Node) NodeContract {
	if node == nil {
		return NodeContract{}
	}
	switch node.Type() {
	case NodeTypeHuman:
		return NodeContract{
			SideEffectClass: SideEffectHuman,
			Idempotency:     IdempotencySingleShot,
			ContextPolicy: core.StateBoundaryPolicy{
				ReadKeys:                 []string{"task.*", "approval.*"},
				WriteKeys:                []string{"approval.*"},
				AllowHistoryAccess:       true,
				AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
				AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState},
				MaxStateEntryBytes:       4096,
				MaxInlineCollectionItems: 16,
			},
		}
	case NodeTypeTerminal, NodeTypeConditional, NodeTypeObservation, NodeTypeSystem:
		return NodeContract{
			SideEffectClass: SideEffectNone,
			Idempotency:     IdempotencyReplaySafe,
			ContextPolicy: core.StateBoundaryPolicy{
				ReadKeys:                 []string{"task.*", "plan.*", "react.*", "architect.*"},
				WriteKeys:                []string{"plan.*", "react.*", "architect.*"},
				AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
				AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStepMetadata, core.StateDataClassRoutingFlag, core.StateDataClassStructuredState},
				MaxStateEntryBytes:       4096,
				MaxInlineCollectionItems: 32,
			},
		}
	case NodeTypeStream:
		return NodeContract{
			SideEffectClass: SideEffectContext,
			Idempotency:     IdempotencyReplaySafe,
			ContextPolicy: core.StateBoundaryPolicy{
				ReadKeys:                 []string{"task.*", "contextstream.*"},
				WriteKeys:                []string{"contextstream.*"},
				AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking},
				AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStructuredState},
				MaxStateEntryBytes:       4096,
				MaxInlineCollectionItems: 16,
			},
		}
	case NodeTypeTool:
		return NodeContract{
			SideEffectClass: SideEffectExternal,
			Idempotency:     IdempotencyUnknown,
			ContextPolicy: core.StateBoundaryPolicy{
				ReadKeys:                 []string{"task.*", "react.*", "planner.*", "architect.*"},
				WriteKeys:                []string{"react.*", "planner.*", "tool.*", "artifact.*"},
				AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking, core.MemoryClassStreamed},
				AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStepMetadata, core.StateDataClassArtifactRef, core.StateDataClassMemoryRef, core.StateDataClassStructuredState},
				MaxStateEntryBytes:       4096,
				MaxInlineCollectionItems: 16,
				PreferArtifactReferences: true,
			},
		}
	default:
		return NodeContract{}
	}
}

func validateNodeContract(node Node, contract NodeContract) error {
	for _, selector := range contract.RequiredCapabilities {
		if err := core.ValidateCapabilitySelector(selector); err != nil {
			return fmt.Errorf("node %s has invalid capability selector: %w", node.ID(), err)
		}
	}
	switch contract.SideEffectClass {
	case "", SideEffectNone, SideEffectContext, SideEffectLocal, SideEffectExternal, SideEffectHuman:
	default:
		return fmt.Errorf("node %s has invalid side effect class %q", node.ID(), contract.SideEffectClass)
	}
	switch contract.Idempotency {
	case "", IdempotencyUnknown, IdempotencyReplaySafe, IdempotencySingleShot:
	default:
		return fmt.Errorf("node %s has invalid idempotency %q", node.ID(), contract.Idempotency)
	}
	switch contract.PreferredPlacement {
	case "", PlacementPreferenceAny, PlacementPreferenceLocal, PlacementPreferenceRemote, PlacementPreferenceSticky:
	default:
		return fmt.Errorf("node %s has invalid placement preference %q", node.ID(), contract.PreferredPlacement)
	}
	switch contract.Recoverability {
	case "", NodeRecoverabilityNone, NodeRecoverabilityInProcess, NodeRecoverabilityPersisted:
	default:
		return fmt.Errorf("node %s has invalid recoverability %q", node.ID(), contract.Recoverability)
	}
	switch contract.CheckpointPolicy {
	case "", CheckpointPolicyNone, CheckpointPolicyPreferred, CheckpointPolicyRequired:
	default:
		return fmt.Errorf("node %s has invalid checkpoint policy %q", node.ID(), contract.CheckpointPolicy)
	}
	if err := core.ValidateStateBoundaryPolicy(contract.ContextPolicy); err != nil {
		return fmt.Errorf("node %s has invalid context policy: %w", node.ID(), err)
	}
	if node != nil && node.Type() == NodeTypeTool && len(contract.RequiredCapabilities) == 0 {
		if _, ok := node.(ContractNode); ok {
			return fmt.Errorf("node %s is a tool node but declares no required capabilities", node.ID())
		}
	}
	return nil
}

func toolNodeContract(tool Tool) NodeContract {
	contract := NodeContract{
		SideEffectClass: SideEffectExternal,
		Idempotency:     IdempotencyUnknown,
		ContextPolicy: core.StateBoundaryPolicy{
			ReadKeys:                 []string{"task.*", "react.*", "planner.*", "architect.*"},
			WriteKeys:                []string{"react.*", "planner.*", "tool.*", "artifact.*"},
			AllowedMemoryClasses:     []core.MemoryClass{core.MemoryClassWorking, core.MemoryClassStreamed},
			AllowedDataClasses:       []core.StateDataClass{core.StateDataClassTaskMetadata, core.StateDataClassStepMetadata, core.StateDataClassArtifactRef, core.StateDataClassMemoryRef, core.StateDataClassStructuredState},
			MaxStateEntryBytes:       4096,
			MaxInlineCollectionItems: 16,
			PreferArtifactReferences: true,
		},
	}
	if tool == nil {
		return contract
	}
	desc := core.ToolDescriptor(context.Background(), tool)
	if desc.ID != "" || desc.Name != "" {
		contract.RequiredCapabilities = []core.CapabilitySelector{{
			ID:   desc.ID,
			Name: desc.Name,
			Kind: core.CapabilityKindTool,
		}}
	}
	contract.SideEffectClass = classifyToolSideEffects(desc)
	contract.Idempotency = classifyToolIdempotency(desc)
	return contract
}

// LintNodeState applies the node's declared state boundary policy to a context snapshot.
func LintNodeState(node Node, env *contextdata.Envelope) []core.StateBoundaryViolation {
	if node == nil || env == nil {
		return nil
	}
	return core.LintStateMap(env.Snapshot(), ResolveNodeContract(node).ContextPolicy)
}

func classifyToolSideEffects(desc core.CapabilityDescriptor) SideEffectClass {
	if len(desc.EffectClasses) == 0 {
		return SideEffectNone
	}
	hasContextOnly := true
	hasExternal := false
	for _, effect := range desc.EffectClasses {
		switch effect {
		case core.EffectClassContextInsertion:
			continue
		case core.EffectClassFilesystemMutation, core.EffectClassProcessSpawn:
			hasContextOnly = false
		case core.EffectClassNetworkEgress, core.EffectClassCredentialUse, core.EffectClassExternalState, core.EffectClassSessionCreation:
			hasContextOnly = false
			hasExternal = true
		default:
			hasContextOnly = false
			hasExternal = true
		}
	}
	if hasContextOnly {
		return SideEffectContext
	}
	if hasExternal {
		return SideEffectExternal
	}
	return SideEffectLocal
}

func classifyToolIdempotency(desc core.CapabilityDescriptor) IdempotencyClass {
	if len(desc.EffectClasses) == 0 {
		return IdempotencyReplaySafe
	}
	for _, effect := range desc.EffectClasses {
		switch effect {
		case core.EffectClassFilesystemMutation, core.EffectClassNetworkEgress, core.EffectClassCredentialUse, core.EffectClassExternalState, core.EffectClassSessionCreation:
			return IdempotencySingleShot
		}
	}
	for _, risk := range desc.RiskClasses {
		switch risk {
		case core.RiskClassDestructive, core.RiskClassNetwork, core.RiskClassCredentialed, core.RiskClassExfiltration, core.RiskClassSessioned:
			return IdempotencySingleShot
		}
	}
	return IdempotencyReplaySafe
}
