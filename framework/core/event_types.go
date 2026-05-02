package core

import (
	"encoding/json"
	"time"

	"codeburg.org/lexbit/relurpify/relurpnet/identity"
)

// FrameworkEvent is the replayable framework event envelope used by the V2 event log.
type FrameworkEvent struct {
	Seq            uint64              `json:"seq"`
	Timestamp      time.Time           `json:"ts"`
	Type           string              `json:"type"`
	CausedBy       []uint64            `json:"caused_by,omitempty"`
	Payload        json.RawMessage     `json:"payload"`
	Actor          identity.EventActor `json:"actor"`
	IdempotencyKey string              `json:"idem_key,omitempty"`
	Partition      string              `json:"partition"`
}

const (
	FrameworkEventAgentRunStarted   = "agent.run.started.v1"
	FrameworkEventAgentRunCompleted = "agent.run.completed.v1"
	FrameworkEventAgentRunFailed    = "agent.run.failed.v1"
	FrameworkEventCapabilityInvoked = "capability.invoked.v1"
	FrameworkEventCapabilityResult  = "capability.result.v1"
	FrameworkEventCapabilityError   = "capability.error.v1"
	FrameworkEventLLMRequested      = "llm.requested.v1"
	FrameworkEventLLMResponded      = "llm.responded.v1"
	FrameworkEventHITLRequested     = "hitl.requested.v1"
	FrameworkEventHITLResolved      = "hitl.resolved.v1"
	FrameworkEventPolicyEvaluated   = "policy.evaluated.v1"
)

const (
	FrameworkEventSessionCreated   = "session.created.v1"
	FrameworkEventSessionMessage   = "session.message.v1"
	FrameworkEventSessionCompacted = "session.compacted.v1"
	FrameworkEventSessionClosed    = "session.closed.v1"
)

const (
	FrameworkEventNodeConnected        = "node.connected.v1"
	FrameworkEventNodeDisconnected     = "node.disconnected.v1"
	FrameworkEventNodePairingRequested = "node.pairing.requested.v1"
	FrameworkEventNodePairingApproved  = "node.pairing.approved.v1"
	FrameworkEventNodePairingRejected  = "node.pairing.rejected.v1"
	FrameworkEventNodeHealth           = "node.health.v1"
)

const (
	FrameworkEventMessageInbound      = "message.inbound.v1"
	FrameworkEventMessageOutbound     = "message.outbound.v1"
	FrameworkEventChannelConnected    = "channel.connected.v1"
	FrameworkEventChannelDisconnected = "channel.disconnected.v1"
)

const (
	FrameworkEventApprovalRequested = "approval.requested.v1"
	FrameworkEventApprovalGranted   = "approval.granted.v1"
	FrameworkEventApprovalDenied    = "approval.denied.v1"
	FrameworkEventApprovalExpired   = "approval.expired.v1"
)

const (
	FrameworkEventSystemStarted    = "system.started.v1"
	FrameworkEventSystemCheckpoint = "system.checkpoint.v1"
	FrameworkEventConfigChanged    = "manifest.changed.v1"
	FrameworkEventManifestReloaded = "manifest.reloaded.v1"
)

// Phase 4: Knowledge chunk and context policy events
const (
	FrameworkEventChunkCommitted        = "chunk.committed.v1"
	FrameworkEventSummaryCommitted      = "summary.committed.v1"
	FrameworkEventContextPolicyReloaded = "context_policy.reloaded.v1"
	FrameworkEventProviderSessionEnded  = "provider.session.ended.v1"
)
