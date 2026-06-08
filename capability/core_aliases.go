package capability

import (
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/telemetry"
)

const (
	EventCapabilityCall   = telemetry.EventCapabilityCall
	EventCapabilityResult = telemetry.EventCapabilityResult
	EventStateChange      = telemetry.EventStateChange
	EventToolCall         = telemetry.EventToolCall
	EventToolResult       = telemetry.EventToolResult
)

var EstimatePayloadBytes = execution.EstimatePayloadBytes
var EstimatePayloadTokens = execution.EstimatePayloadTokens
var RedactMetadataMap = execution.RedactMetadataMap

const PolicyTargetCapability = policy.PolicyTargetCapability
