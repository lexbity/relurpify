package session

import "codeburg.org/lexbit/relurpify/framework/core"

type SessionScope = core.SessionScope

const (
	SessionScopeMain           = core.SessionScopeMain
	SessionScopePerChannelPeer = core.SessionScopePerChannelPeer
	SessionScopePerThread      = core.SessionScopePerThread
)

type SessionBoundary = core.SessionBoundary
type SessionBinding = core.SessionBinding

const RestrictedExternalTenantID = core.RestrictedExternalTenantID
