package runtime

import (
	"time"

	nexusadminapi "github.com/lexcodex/relurpify/app/nexus/adminapi"
)

type RuntimeState struct {
	Online           bool
	PID              int
	BindAddr         string
	Uptime           time.Duration
	LastSeq          uint64
	TenantID         string
	PairedNodes      []NodeInfo
	PendingPairings  []PendingPairingInfo
	Channels         []ChannelInfo
	ActiveSessions   []SessionInfo
	Subjects         []SubjectInfo
	ExternalIDs      []ExternalIdentityInfo
	Tokens           []TokenInfo
	PolicyRules      []PolicyRuleInfo
	SecurityWarnings []string
	EventCounts      map[string]uint64
}

type NodeInfo struct {
	ID         string
	Name       string
	Platform   string
	TenantID   string
	TrustClass string
}

type PendingPairingInfo = nexusadminapi.PendingPairingInfo
type ChannelInfo = nexusadminapi.ChannelInfo
type SessionInfo = nexusadminapi.SessionInfo
type SubjectInfo = nexusadminapi.SubjectInfo

type ExternalIdentityInfo struct {
	TenantID    string
	Provider    string
	AccountID   string
	ExternalID  string
	SubjectID   string
	DisplayName string
}

type TokenInfo = nexusadminapi.TokenInfo

type PolicyRuleInfo struct {
	ID       string
	Name     string
	Priority int
	Enabled  bool
	Action   string
	Reason   string
}

type IssueTokenRequest struct {
	SubjectID string
	Scope     string
}

type ListEventsRequest struct {
	Limit int
}

type EventInfo = nexusadminapi.EventInfo
