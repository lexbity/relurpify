package capability

import (
	"fmt"
	"strings"
	"time"

	agentspec "codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/taxonomy"
	relurpctx "codeburg.org/lexbit/relurpify/context"
	"codeburg.org/lexbit/relurpify/execution"
)


type ContentDisposition string

const (
	ContentDispositionRaw          ContentDisposition = "raw"
	ContentDispositionSummarized   ContentDisposition = "summarized"
	ContentDispositionTransformed  ContentDisposition = "transformed"
	ContentDispositionMetadataOnly ContentDisposition = "metadata-only"
)

type InsertionDecision struct {
	Action           agentspec.InsertionAction `json:"action"`
	Reason           string          `json:"reason,omitempty"`
	RequiresHITL     bool            `json:"requires_hitl,omitempty"`
	PolicySnapshotID string          `json:"policy_snapshot_id,omitempty"`
}

type ContentBlockInsertion struct {
	ContentType string            `json:"content_type,omitempty"`
	Decision    InsertionDecision `json:"decision"`
}

type ApprovalBinding struct {
	CapabilityID   string                  `json:"capability_id,omitempty"`
	CapabilityName string                  `json:"capability_name,omitempty"`
	ProviderID     string                  `json:"provider_id,omitempty"`
	SessionID      string                  `json:"session_id,omitempty"`
	EffectClasses  []taxonomy.EffectClass `json:"effect_classes,omitempty"`
	TargetResource string                  `json:"target_resource,omitempty"`
	TaskID         string                  `json:"task_id,omitempty"`
	WorkflowID     string                  `json:"workflow_id,omitempty"`
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
	ID                 string                                    `json:"id"`
	CapturedAt         time.Time                                 `json:"captured_at"`
	AgentID            string                                    `json:"agent_id,omitempty"`
	ToolPolicies       map[string]agentspec.ToolPolicy           `json:"tool_policies,omitempty"`
	CapabilityPolicies []agentspec.CapabilityPolicy              `json:"capability_policies,omitempty"`
	ExposurePolicies   []agentspec.CapabilityExposurePolicy      `json:"exposure_policies,omitempty"`
	InsertionPolicies  []agentspec.CapabilityInsertionPolicy     `json:"insertion_policies,omitempty"`
	GlobalPolicies     map[string]agentspec.AgentPermissionLevel `json:"global_policies,omitempty"`
	ProviderPolicies   map[string]agentspec.ProviderPolicy       `json:"provider_policies,omitempty"`
	RuntimeSafety      *agentspec.RuntimeSafetySpec              `json:"runtime_safety,omitempty"`
	Revocations        execution.RevocationSnapshot                        `json:"revocations,omitempty"`
}

type CapabilityResultEnvelope struct {
	Descriptor      CapabilityDescriptor    `json:"descriptor"`
	Result          *ports.ToolResult       `json:"result,omitempty"`
	ContentBlocks   []ContentBlock          `json:"-"`
	BlockInsertions []ContentBlockInsertion `json:"block_insertions,omitempty"`
	Provenance      ContentProvenance       `json:"provenance,omitempty"`
	Disposition     ContentDisposition      `json:"disposition,omitempty"`
	Insertion       InsertionDecision       `json:"insertion,omitempty"`
	Approval        *ApprovalBinding        `json:"approval,omitempty"`
	Policy          *PolicySnapshot         `json:"policy,omitempty"`
	RecordedAt      time.Time               `json:"recorded_at"`
}

func NewCapabilityResultEnvelope(descriptor CapabilityDescriptor, result *ports.ToolResult, disposition ContentDisposition, snapshot *PolicySnapshot, approval *ApprovalBinding) *CapabilityResultEnvelope {
	return newCapabilityResultEnvelopeWithBlocks(descriptor, result, disposition, snapshot, approval, nil)
}

// NewCapabilityResultEnvelopeWithBlocks creates an envelope using pre-computed
// content blocks. This allows callers to apply structured chunking (from
// framework/toolcapabilities) before envelope construction.
func NewCapabilityResultEnvelopeWithBlocks(descriptor CapabilityDescriptor, result *ports.ToolResult, disposition ContentDisposition, snapshot *PolicySnapshot, approval *ApprovalBinding, blocks []ContentBlock) *CapabilityResultEnvelope {
	return newCapabilityResultEnvelopeWithBlocks(descriptor, result, disposition, snapshot, approval, blocks)
}

func newCapabilityResultEnvelopeWithBlocks(descriptor CapabilityDescriptor, result *ports.ToolResult, disposition ContentDisposition, snapshot *PolicySnapshot, approval *ApprovalBinding, blocks []ContentBlock) *CapabilityResultEnvelope {
	provenance := ContentProvenance{
		CapabilityID: descriptor.ID,
		ProviderID:   descriptor.Source.ProviderID,
		TrustClass:   descriptor.TrustClass,
		Disposition:  disposition,
	}

	if blocks == nil {
		blocks = capabilityResultBlocks(result, provenance)
	}

	envelope := &CapabilityResultEnvelope{
		Descriptor:    descriptor,
		Result:        result,
		Provenance:    provenance,
		Disposition:   disposition,
		Insertion:     DefaultInsertionDecision(descriptor, disposition),
		Approval:      approval,
		Policy:        snapshot,
		RecordedAt:    time.Now().UTC(),
		ContentBlocks: blocks,
	}
	if snapshot != nil {
		envelope.Insertion.PolicySnapshotID = snapshot.ID
	}
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
	decision.RequiresHITL = decision.Action == agentspec.InsertionActionHITLRequired
	envelope.Insertion = decision
	envelope.BlockInsertions = buildContentBlockInsertions(envelope.ContentBlocks, decision)
	return envelope
}

func DefaultInsertionDecision(descriptor CapabilityDescriptor, disposition ContentDisposition) InsertionDecision {
	switch descriptor.TrustClass {
	case agentspec.TrustClassBuiltinTrusted, agentspec.TrustClassWorkspaceTrusted:
		return InsertionDecision{Action: agentspec.InsertionActionDirect, Reason: "trusted capability output allowed for direct insertion"}
	case agentspec.TrustClassLLMGenerated, agentspec.TrustClassToolResult:
		return InsertionDecision{Action: agentspec.InsertionActionSummarized, Reason: "generated capability output requires summarized insertion"}
	case agentspec.TrustClassRemoteApproved:
		return InsertionDecision{Action: agentspec.InsertionActionSummarized, Reason: "remote-approved capability output requires summarized insertion"}
	case agentspec.TrustClassProviderLocalUntrusted, agentspec.TrustClassRemoteDeclared:
		return InsertionDecision{Action: agentspec.InsertionActionMetadataOnly, Reason: "untrusted capability output defaults to metadata-only insertion"}
	}
	switch disposition {
	case ContentDispositionMetadataOnly:
		return InsertionDecision{Action: agentspec.InsertionActionMetadataOnly, Reason: "metadata-only content disposition"}
	case ContentDispositionSummarized:
		return InsertionDecision{Action: agentspec.InsertionActionSummarized, Reason: "summarized content disposition"}
	default:
		return InsertionDecision{Action: agentspec.InsertionActionSummarized, Reason: "capability output requires summarized insertion by default"}
	}
}

func SummarizeCapabilityResultEnvelope(source *CapabilityResultEnvelope, summary string) *CapabilityResultEnvelope {
	if source == nil {
		return nil
	}
	summary = strings.TrimSpace(summary)
	result := &ports.ToolResult{
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
		origin := relurpctx.OriginDerivation("capability_result")
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
	case agentspec.InsertionActionDirect:
		decision.Action = agentspec.InsertionActionSummarized
		decision.Reason = "summarized insertion preserves provenance"
	case agentspec.InsertionActionSummarized, agentspec.InsertionActionMetadataOnly, agentspec.InsertionActionHITLRequired, agentspec.InsertionActionDenied:
	default:
		decision = envelope.Insertion
	}
	return ApplyInsertionDecision(envelope, decision)
}

func ToolResultEnvelope(result *ports.ToolResult) (*CapabilityResultEnvelope, bool) {
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
func CapabilityExecutionEnvelope(result *ports.ToolResult) (*CapabilityResultEnvelope, bool) {
	return ToolResultEnvelope(result)
}

func CapabilityResultBlocks(result *ports.ToolResult, provenance ContentProvenance) []ContentBlock {
	return capabilityResultBlocks(result, provenance)
}

func capabilityResultBlocks(result *ports.ToolResult, provenance ContentProvenance) []ContentBlock {
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
		decision.Action = agentspec.InsertionActionMetadataOnly
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
	candidate.RequiresHITL = candidate.Action == agentspec.InsertionActionHITLRequired
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

func insertionRestrictiveness(action agentspec.InsertionAction) int {
	switch action {
	case agentspec.InsertionActionDirect:
		return 0
	case agentspec.InsertionActionSummarized:
		return 1
	case agentspec.InsertionActionMetadataOnly:
		return 2
	case agentspec.InsertionActionHITLRequired:
		return 3
	case agentspec.InsertionActionDenied:
		return 4
	default:
		return 4
	}
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
