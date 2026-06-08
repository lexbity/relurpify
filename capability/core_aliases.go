package capability

import (
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

const PolicyTargetCapability = policy.PolicyTargetCapability
