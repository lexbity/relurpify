package core

import (
	"fmt"
	"strings"
	"time"

	agentspec "codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type InsertionAction = agentspec.InsertionAction

const (
	InsertionActionDirect       InsertionAction = agentspec.InsertionActionDirect
	InsertionActionSummarized   InsertionAction = agentspec.InsertionActionSummarized
	InsertionActionMetadataOnly InsertionAction = agentspec.InsertionActionMetadataOnly
	InsertionActionHITLRequired InsertionAction = agentspec.InsertionActionHITLRequired
	InsertionActionDenied       InsertionAction = agentspec.InsertionActionDenied
)

// CapabilityExposure defines the exposure level of a capability.
type CapabilityExposure = agentspec.CapabilityExposure

const (
	// CapabilityExposureHidden means the capability is not exposed.
	CapabilityExposureHidden = agentspec.CapabilityExposureHidden
	// CapabilityExposureInspectable means the capability can be inspected but not called.
	CapabilityExposureInspectable = agentspec.CapabilityExposureInspectable
	// CapabilityExposureCallable means the capability can be called.
	CapabilityExposureCallable = agentspec.CapabilityExposureCallable
)

// CapabilityExposurePolicy configures visibility of admitted capabilities.
type CapabilityExposurePolicy = agentspec.CapabilityExposurePolicy

// CapabilityInsertionPolicy configures how capability output may be inserted.
type CapabilityInsertionPolicy = agentspec.CapabilityInsertionPolicy

type ContentDisposition string

const (
	ContentDispositionRaw          ContentDisposition = "raw"
	ContentDispositionSummarized   ContentDisposition = "summarized"
	ContentDispositionTransformed  ContentDisposition = "transformed"
	ContentDispositionMetadataOnly ContentDisposition = "metadata-only"
)

type InsertionDecision struct {
	Action           InsertionAction `json:"action" yaml:"action"`
	Reason           string          `json:"reason,omitempty" yaml:"reason,omitempty"`
	RequiresHITL     bool            `json:"requires_hitl,omitempty" yaml:"requires_hitl,omitempty"`
	PolicySnapshotID string          `json:"policy_snapshot_id,omitempty" yaml:"policy_snapshot_id,omitempty"`
}

type ContentBlockInsertion struct {
	ContentType string            `json:"content_type,omitempty" yaml:"content_type,omitempty"`
	Decision    InsertionDecision `json:"decision" yaml:"decision"`
}

type ApprovalBinding struct {
	CapabilityID   string        `json:"capability_id,omitempty" yaml:"capability_id,omitempty"`
	CapabilityName string        `json:"capability_name,omitempty" yaml:"capability_name,omitempty"`
	ProviderID     string        `json:"provider_id,omitempty" yaml:"provider_id,omitempty"`
	SessionID      string        `json:"session_id,omitempty" yaml:"session_id,omitempty"`
	EffectClasses  []EffectClass `json:"effect_classes,omitempty" yaml:"effect_classes,omitempty"`
	TargetResource string        `json:"target_resource,omitempty" yaml:"target_resource,omitempty"`
	TaskID         string        `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	WorkflowID     string        `json:"workflow_id,omitempty" yaml:"workflow_id,omitempty"`
}

func (b ApprovalBinding) PermissionMetadata() map[string]string {
	metadata := map[string]string{}
	if b.CapabilityID != "" {
		metadata["capability_id"] = b.CapabilityID
	}
	if b.CapabilityName != "" {
		metadata["capability_name"] = b.CapabilityName
	}
	if b.ProviderID != "" {
		metadata["provider_id"] = b.ProviderID
	}
	if b.SessionID != "" {
		metadata["session_id"] = b.SessionID
	}
	if len(b.EffectClasses) > 0 {
		parts := make([]string, 0, len(b.EffectClasses))
		for _, effect := range b.EffectClasses {
			if effect == "" {
				continue
			}
			parts = append(parts, string(effect))
		}
		if len(parts) > 0 {
			metadata["effect_classes"] = strings.Join(parts, ",")
		}
	}
	if b.TargetResource != "" {
		metadata["target_resource"] = b.TargetResource
	}
	if b.TaskID != "" {
		metadata["task_id"] = b.TaskID
	}
	if b.WorkflowID != "" {
		metadata["workflow_id"] = b.WorkflowID
	}
	return metadata
}

type PolicySnapshot struct {
	ID                 string                                    `json:"id" yaml:"id"`
	CapturedAt         time.Time                                 `json:"captured_at" yaml:"captured_at"`
	AgentID            string                                    `json:"agent_id,omitempty" yaml:"agent_id,omitempty"`
	ToolPolicies       map[string]agentspec.ToolPolicy           `json:"tool_policies,omitempty" yaml:"tool_policies,omitempty"`
	CapabilityPolicies []agentspec.CapabilityPolicy              `json:"capability_policies,omitempty" yaml:"capability_policies,omitempty"`
	ExposurePolicies   []agentspec.CapabilityExposurePolicy      `json:"exposure_policies,omitempty" yaml:"exposure_policies,omitempty"`
	InsertionPolicies  []agentspec.CapabilityInsertionPolicy     `json:"insertion_policies,omitempty" yaml:"insertion_policies,omitempty"`
	GlobalPolicies     map[string]agentspec.AgentPermissionLevel `json:"global_policies,omitempty" yaml:"global_policies,omitempty"`
	ProviderPolicies   map[string]agentspec.ProviderPolicy       `json:"provider_policies,omitempty" yaml:"provider_policies,omitempty"`
	RuntimeSafety      *agentspec.RuntimeSafetySpec              `json:"runtime_safety,omitempty" yaml:"runtime_safety,omitempty"`
	Revocations        RevocationSnapshot                        `json:"revocations,omitempty" yaml:"revocations,omitempty"`
}

type CapabilityResultEnvelope struct {
	Descriptor      CapabilityDescriptor    `json:"descriptor" yaml:"descriptor"`
	Result          *contracts.ToolResult   `json:"result,omitempty" yaml:"result,omitempty"`
	ContentBlocks   []ContentBlock          `json:"-" yaml:"-"`
	BlockInsertions []ContentBlockInsertion `json:"block_insertions,omitempty" yaml:"block_insertions,omitempty"`
	Provenance      ContentProvenance       `json:"provenance,omitempty" yaml:"provenance,omitempty"`
	Disposition     ContentDisposition      `json:"disposition,omitempty" yaml:"disposition,omitempty"`
	Insertion       InsertionDecision       `json:"insertion,omitempty" yaml:"insertion,omitempty"`
	Approval        *ApprovalBinding        `json:"approval,omitempty" yaml:"approval,omitempty"`
	Policy          *PolicySnapshot         `json:"policy,omitempty" yaml:"policy,omitempty"`
	RecordedAt      time.Time               `json:"recorded_at" yaml:"recorded_at"`
}

func NewCapabilityResultEnvelope(descriptor CapabilityDescriptor, result *contracts.ToolResult, disposition ContentDisposition, snapshot *PolicySnapshot, approval *ApprovalBinding) *CapabilityResultEnvelope {
	provenance := ContentProvenance{
		CapabilityID: descriptor.ID,
		ProviderID:   descriptor.Source.ProviderID,
		TrustClass:   descriptor.TrustClass,
		Disposition:  disposition,
	}
	envelope := &CapabilityResultEnvelope{
		Descriptor:  descriptor,
		Result:      result,
		Provenance:  provenance,
		Disposition: disposition,
		Insertion:   DefaultInsertionDecision(descriptor, disposition),
		Approval:    approval,
		Policy:      snapshot,
		RecordedAt:  time.Now().UTC(),
	}
	if snapshot != nil {
		envelope.Insertion.PolicySnapshotID = snapshot.ID
	}
	envelope.ContentBlocks = capabilityResultBlocks(result, provenance)
	envelope.BlockInsertions = buildContentBlockInsertions(envelope.ContentBlocks, envelope.Insertion)
	return envelope
}

func ApplyInsertionDecision(envelope *CapabilityResultEnvelope, decision InsertionDecision) *CapabilityResultEnvelope {
	if envelope == nil {
		return nil
	}
	if decision.PolicySnapshotID == "" && envelope.Policy != nil {
		decision.PolicySnapshotID = envelope.Policy.ID
	}
	decision.RequiresHITL = decision.Action == InsertionActionHITLRequired
	envelope.Insertion = decision
	envelope.BlockInsertions = buildContentBlockInsertions(envelope.ContentBlocks, decision)
	return envelope
}

func DefaultInsertionDecision(descriptor CapabilityDescriptor, disposition ContentDisposition) InsertionDecision {
	switch descriptor.TrustClass {
	case TrustClassBuiltinTrusted, TrustClassWorkspaceTrusted:
		return InsertionDecision{Action: InsertionActionDirect, Reason: "trusted capability output allowed for direct insertion"}
	case TrustClassLLMGenerated, TrustClassToolResult:
		return InsertionDecision{Action: InsertionActionSummarized, Reason: "generated capability output requires summarized insertion"}
	case TrustClassRemoteApproved:
		return InsertionDecision{Action: InsertionActionSummarized, Reason: "remote-approved capability output requires summarized insertion"}
	case TrustClassProviderLocalUntrusted, TrustClassRemoteDeclared:
		return InsertionDecision{Action: InsertionActionMetadataOnly, Reason: "untrusted capability output defaults to metadata-only insertion"}
	}
	switch disposition {
	case ContentDispositionMetadataOnly:
		return InsertionDecision{Action: InsertionActionMetadataOnly, Reason: "metadata-only content disposition"}
	case ContentDispositionSummarized:
		return InsertionDecision{Action: InsertionActionSummarized, Reason: "summarized content disposition"}
	default:
		return InsertionDecision{Action: InsertionActionSummarized, Reason: "capability output requires summarized insertion by default"}
	}
}

func SummarizeCapabilityResultEnvelope(source *CapabilityResultEnvelope, summary string) *CapabilityResultEnvelope {
	if source == nil {
		return nil
	}
	summary = strings.TrimSpace(summary)
	result := &contracts.ToolResult{
		Success: true,
		Data:    map[string]interface{}{"summary": summary},
	}
	if source.Result != nil {
		result.Success = source.Result.Success
		result.Error = source.Result.Error
		result.Metadata = cloneInterfaceMap(source.Result.Metadata)
	}
	envelope := NewCapabilityResultEnvelope(source.Descriptor, result, ContentDispositionSummarized, source.Policy, source.Approval)
	envelope.RecordedAt = source.RecordedAt

	// Append summarize derivation step to provenance chain
	provenance := envelope.Provenance
	if provenance.Derivation == nil {
		// Start a new derivation chain
		origin := OriginDerivation("capability_result")
		provenance.Derivation = &origin
	} else {
		// Append to existing chain
		derived := provenance.Derivation.Derive("compress_summarize", "capability_result", 0.1, summary)
		provenance.Derivation = &derived
	}
	envelope.Provenance = provenance

	decision := source.Insertion
	if decision.Action == "" {
		decision = envelope.Insertion
	}
	switch decision.Action {
	case InsertionActionDirect:
		decision.Action = InsertionActionSummarized
		decision.Reason = "summarized insertion preserves provenance"
	case InsertionActionSummarized, InsertionActionMetadataOnly, InsertionActionHITLRequired, InsertionActionDenied:
	default:
		decision = envelope.Insertion
	}
	return ApplyInsertionDecision(envelope, decision)
}

func ToolResultEnvelope(result *contracts.ToolResult) (*CapabilityResultEnvelope, bool) {
	if result == nil || result.Metadata == nil {
		return nil, false
	}
	raw, ok := result.Metadata["capability_result_envelope"]
	if !ok || raw == nil {
		return nil, false
	}
	envelope, ok := raw.(*CapabilityResultEnvelope)
	return envelope, ok && envelope != nil
}

// CapabilityExecutionEnvelope returns the capability envelope attached to an
// execution result.
func CapabilityExecutionEnvelope(result *contracts.CapabilityExecutionResult) (*CapabilityResultEnvelope, bool) {
	return ToolResultEnvelope(result)
}

func capabilityResultBlocks(result *contracts.ToolResult, provenance ContentProvenance) []ContentBlock {
	if result == nil {
		return nil
	}
	blocks := make([]ContentBlock, 0, 2)
	if len(result.Data) > 0 {
		if summary, ok := result.Data["summary"].(string); ok && strings.TrimSpace(summary) != "" && len(result.Data) == 1 {
			blocks = append(blocks, TextContentBlock{
				Text:       summary,
				Provenance: provenance,
			})
		} else {
			blocks = append(blocks, StructuredContentBlock{
				Data:       result.Data,
				Provenance: provenance,
			})
		}
	}
	if msg := strings.TrimSpace(result.Error); msg != "" {
		blocks = append(blocks, ErrorContentBlock{
			Message:    msg,
			Provenance: provenance,
		})
	}
	return blocks
}

func buildContentBlockInsertions(blocks []ContentBlock, decision InsertionDecision) []ContentBlockInsertion {
	if len(blocks) == 0 {
		return nil
	}
	insertions := make([]ContentBlockInsertion, 0, len(blocks))
	for _, block := range blocks {
		if block == nil {
			continue
		}
		blockDecision := moreRestrictiveInsertionDecision(decision, defaultBlockInsertionDecision(block, decision))
		insertions = append(insertions, ContentBlockInsertion{
			ContentType: block.ContentType(),
			Decision:    blockDecision,
		})
	}
	if len(insertions) == 0 {
		return nil
	}
	return insertions
}

func defaultBlockInsertionDecision(block ContentBlock, inherited InsertionDecision) InsertionDecision {
	switch block.(type) {
	case BinaryReferenceContentBlock, EmbeddedResourceContentBlock, ResourceLinkContentBlock:
		decision := inherited
		decision.Action = InsertionActionMetadataOnly
		decision.Reason = "resource and binary content defaults to metadata-only insertion"
		decision.RequiresHITL = false
		return decision
	default:
		return inherited
	}
}

func moreRestrictiveInsertionDecision(base, candidate InsertionDecision) InsertionDecision {
	if insertionRestrictiveness(candidate.Action) < insertionRestrictiveness(base.Action) {
		return base
	}
	if candidate.PolicySnapshotID == "" {
		candidate.PolicySnapshotID = base.PolicySnapshotID
	}
	candidate.RequiresHITL = candidate.Action == InsertionActionHITLRequired
	return candidate
}

func ApprovalBindingFromCapability(descriptor CapabilityDescriptor, state map[string]interface{}, args map[string]interface{}) *ApprovalBinding {
	targetResource := inferTargetResource(args)
	taskID := ""
	workflowID := ""
	if state != nil {
		if v, ok := state["task.id"].(string); ok {
			taskID = strings.TrimSpace(v)
		}
		if v, ok := state["architect.workflow_id"].(string); ok {
			workflowID = strings.TrimSpace(v)
		}
	}
	if descriptor.Source.ProviderID == "" &&
		descriptor.Source.SessionID == "" &&
		len(descriptor.EffectClasses) == 0 &&
		targetResource == "" &&
		taskID == "" &&
		workflowID == "" {
		return nil
	}
	binding := &ApprovalBinding{
		CapabilityID:   descriptor.ID,
		CapabilityName: descriptor.Name,
		ProviderID:     descriptor.Source.ProviderID,
		SessionID:      descriptor.Source.SessionID,
		TargetResource: targetResource,
		TaskID:         taskID,
		WorkflowID:     workflowID,
	}
	if len(descriptor.EffectClasses) > 0 {
		binding.EffectClasses = descriptor.EffectClasses
	}
	return binding
}

func inferTargetResource(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	for _, key := range []string{"path", "target", "resource", "uri", "url", "file", "binary", "database_path", "host"} {
		if value, ok := args[key]; ok {
			target := strings.TrimSpace(fmt.Sprint(value))
			if target != "" {
				return target
			}
		}
	}
	return ""
}

func cloneInterfaceMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
