package event

import (
	"encoding/json"
	"time"

	"codeburg.org/lexbit/relurpify/platform/observability"
)

type FrameworkEvent struct {
	Seq            uint64              `json:"seq"`
	Timestamp      time.Time           `json:"ts"`
	Type           string              `json:"type"`
	CausedBy       []uint64            `json:"caused_by,omitempty"`
	Payload        json.RawMessage     `json:"payload"`
	Actor          observability.Actor `json:"actor"`
	IdempotencyKey string              `json:"idem_key,omitempty"`
	Partition      string              `json:"partition"`
}

const (
	EventAgentRunStarted       = "agent.run.started.v1"
	EventAgentRunCompleted     = "agent.run.completed.v1"
	EventAgentRunFailed        = "agent.run.failed.v1"
	EventCapabilityInvoked     = "capability.invoked.v1"
	EventCapabilityResult      = "capability.result.v1"
	EventCapabilityError       = "capability.error.v1"
	EventLLMRequested          = "llm.requested.v1"
	EventLLMResponded          = "llm.responded.v1"
	EventHITLRequested         = "hitl.requested.v1"
	EventHITLResolved          = "hitl.resolved.v1"
	EventPolicyEvaluated       = "policy.evaluated.v1"
	EventSessionCreated        = "session.created.v1"
	EventSessionMessage        = "session.message.v1"
	EventSessionCompacted      = "session.compacted.v1"
	EventSessionClosed         = "session.closed.v1"
	EventNodeConnected         = "node.connected.v1"
	EventNodeDisconnected      = "node.disconnected.v1"
	EventNodePairingRequested  = "node.pairing.requested.v1"
	EventNodePairingApproved   = "node.pairing.approved.v1"
	EventNodePairingRejected   = "node.pairing.rejected.v1"
	EventNodeHealth            = "node.health.v1"
	EventMessageInbound        = "message.inbound.v1"
	EventMessageOutbound       = "message.outbound.v1"
	EventChannelConnected      = "channel.connected.v1"
	EventChannelDisconnected   = "channel.disconnected.v1"
	EventApprovalRequested     = "approval.requested.v1"
	EventApprovalGranted       = "approval.granted.v1"
	EventApprovalDenied        = "approval.denied.v1"
	EventApprovalExpired       = "approval.expired.v1"
	EventSystemStarted         = "system.started.v1"
	EventSystemCheckpoint      = "system.checkpoint.v1"
	EventConfigChanged         = "manifest.changed.v1"
	EventManifestReloaded      = "manifest.reloaded.v1"
	EventChunkCommitted        = "chunk.committed.v1"
	EventSummaryCommitted      = "summary.committed.v1"
	EventContextPolicyReloaded = "context_policy.reloaded.v1"
	EventProviderSessionEnded  = "provider.session.ended.v1"
	EventContractResolved      = "contract.resolved.v1"
)
