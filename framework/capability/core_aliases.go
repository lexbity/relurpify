package capability

import (
	"codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/telemetry"
)

type Telemetry = telemetry.Telemetry
type Event = telemetry.Event
type BudgetTelemetry = telemetry.BudgetTelemetry
type CheckpointTelemetry = telemetry.CheckpointTelemetry

const (
	EventCapabilityCall   = telemetry.EventCapabilityCall
	EventCapabilityResult = telemetry.EventCapabilityResult
	EventStateChange      = telemetry.EventStateChange
	EventToolCall         = telemetry.EventToolCall
	EventToolResult       = telemetry.EventToolResult
)

type RevocationSnapshot = execution.RevocationSnapshot
type RuntimeSafetySpec = execution.RuntimeSafetySpec

var EstimatePayloadBytes = execution.EstimatePayloadBytes
var EstimatePayloadTokens = execution.EstimatePayloadTokens
var RedactMetadataMap = execution.RedactMetadataMap

type PolicyRequest = policy.PolicyRequest

const PolicyTargetCapability = policy.PolicyTargetCapability

type Task = execution.Task
type Config = execution.Config
type Result = execution.Result
