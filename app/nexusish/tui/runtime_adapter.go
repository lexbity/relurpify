package tui

import (
	"context"

	nexusruntime "codeburg.org/lexbit/relurpify/app/nexusish/runtime"
)

type RuntimeState = nexusruntime.RuntimeState
type NodeInfo = nexusruntime.NodeInfo
type PendingPairingInfo = nexusruntime.PendingPairingInfo
type ChannelInfo = nexusruntime.ChannelInfo
type SessionInfo = nexusruntime.SessionInfo
type IssueTokenRequest = nexusruntime.IssueTokenRequest
type ListEventsRequest = nexusruntime.ListEventsRequest
type EventInfo = nexusruntime.EventInfo

type RuntimeAdapter interface {
	State(ctx context.Context) (RuntimeState, error)
	ApprovePairing(ctx context.Context, code string) error
	RejectPairing(ctx context.Context, code string) error
	RevokeNode(ctx context.Context, nodeID string) error
	CloseSession(ctx context.Context, sessionID string) error
	IssueToken(ctx context.Context, req IssueTokenRequest) (string, error)
	RevokeToken(ctx context.Context, tokenID string) error
	SetPolicyRuleEnabled(ctx context.Context, ruleID string, enabled bool) error
	RestartChannel(ctx context.Context, channel string) error
	ListEvents(ctx context.Context, req ListEventsRequest) ([]EventInfo, error)
}
