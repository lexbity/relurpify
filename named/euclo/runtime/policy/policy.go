package policy

import (
	"codeburg.org/lexbit/relurpify/framework/capability"
	frameworkcore "codeburg.org/lexbit/relurpify/framework/core"
	runtimepkg "codeburg.org/lexbit/relurpify/named/euclo/runtime"
)

type TaskClassification = runtimepkg.TaskClassification
type ScoredClassification = runtimepkg.ScoredClassification
type ClassificationSignal = runtimepkg.ClassificationSignal
type ModeCandidate = runtimepkg.ModeCandidate
type RetrievalPolicy = runtimepkg.RetrievalPolicy
type ContextExpansion = runtimepkg.ContextExpansion
type VerificationPolicy = runtimepkg.VerificationPolicy
type ExecutionResultClass = runtimepkg.ExecutionResultClass
type ExecutionStatus = runtimepkg.ExecutionStatus
type DeferredIssueKind = runtimepkg.DeferredIssueKind
type DeferredIssueSeverity = runtimepkg.DeferredIssueSeverity
type DeferredIssueStatus = runtimepkg.DeferredIssueStatus
type DeferredExecutionIssue = runtimepkg.DeferredExecutionIssue
type ResolvedExecutionPolicy = runtimepkg.ResolvedExecutionPolicy
type ExecutorFamily = runtimepkg.ExecutorFamily
type WorkUnitExecutorDescriptor = runtimepkg.WorkUnitExecutorDescriptor
type SecurityDiagnostic = runtimepkg.SecurityDiagnostic
type SecurityRuntimeState = runtimepkg.SecurityRuntimeState
type SharedContextRuntimeState = runtimepkg.SharedContextRuntimeState
type CapabilityContractRuntimeState = runtimepkg.CapabilityContractRuntimeState

const (
	ExecutionResultClassCompleted              = runtimepkg.ExecutionResultClassCompleted
	ExecutionResultClassCompletedWithDeferrals = runtimepkg.ExecutionResultClassCompletedWithDeferrals
	ExecutionResultClassBlocked                = runtimepkg.ExecutionResultClassBlocked
	ExecutionResultClassFailed                 = runtimepkg.ExecutionResultClassFailed
	ExecutionResultClassCanceled               = runtimepkg.ExecutionResultClassCanceled
	ExecutionResultClassRestoreFailed          = runtimepkg.ExecutionResultClassRestoreFailed
	ExecutorFamilyReact                        = runtimepkg.ExecutorFamilyReact
	ExecutorFamilyPlanner                      = runtimepkg.ExecutorFamilyPlanner
	ExecutorFamilyHTN                          = runtimepkg.ExecutorFamilyHTN
	ExecutorFamilyRewoo                        = runtimepkg.ExecutorFamilyRewoo
	ExecutorFamilyReflection                   = runtimepkg.ExecutorFamilyReflection
)

var NormalizeTaskEnvelope = runtimepkg.NormalizeTaskEnvelope
var ClassifyTask = runtimepkg.ClassifyTask
var ClassifyTaskScored = runtimepkg.ClassifyTaskScored
var ResolveMode = runtimepkg.ResolveMode
var SelectExecutionProfile = runtimepkg.SelectExecutionProfile
var SnapshotCapabilities = runtimepkg.SnapshotCapabilities
var CollectSignals = runtimepkg.CollectSignals
var ResolveRetrievalPolicy = runtimepkg.ResolveRetrievalPolicy
var ApplyContextExpansion = runtimepkg.ApplyContextExpansion
var ResolveVerificationPolicy = runtimepkg.ResolveVerificationPolicy
var BuildResolvedExecutionPolicy = runtimepkg.BuildResolvedExecutionPolicy
var SelectExecutorDescriptor = runtimepkg.SelectExecutorDescriptor
var BuildCapabilityContractRuntimeState = runtimepkg.BuildCapabilityContractRuntimeState
var BuildCapabilityContractDeferredIssues = runtimepkg.BuildCapabilityContractDeferredIssues

func BuildPolicy(task *frameworkcore.Task, cfg *frameworkcore.Config, registry *capability.Registry, mode runtimepkg.ModeResolution, profile runtimepkg.ExecutionProfileSelection) ResolvedExecutionPolicy {
	return runtimepkg.BuildResolvedExecutionPolicy(task, cfg, registry, mode, profile)
}
