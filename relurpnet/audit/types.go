package audit

import "codeburg.org/lexbit/relurpify/framework/core"

type AuditAction = core.AuditAction

const (
	AuditActionFileAccess = core.AuditActionFileAccess
	AuditActionExec       = core.AuditActionExec
	AuditActionNetwork    = core.AuditActionNetwork
	AuditActionCapability = core.AuditActionCapability
	AuditActionIPC        = core.AuditActionIPC
	AuditActionTool       = core.AuditActionTool
	AuditActionRequest    = core.AuditActionRequest
)

type AuditRecord = core.AuditRecord
type AuditQuery = core.AuditQuery
type AuditChainEntry = core.AuditChainEntry
type AuditChainFilter = core.AuditChainFilter
type AuditChainVerification = core.AuditChainVerification
type AuditChainReader = core.AuditChainReader
