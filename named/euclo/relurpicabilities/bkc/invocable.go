package bkc

import (
	"context"
	"errors"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	frameworkplan "github.com/lexcodex/relurpify/framework/plan"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// Invocable implementations for BKC capabilities.
// BKC capabilities are supporting-only (IsPrimary = false).

// compileInvocable wraps compileCapability as an Invocable.
type compileInvocable struct {
	cap euclotypes.EucloCodingCapability
	env agentenv.AgentEnvironment
}

// NewCompileInvocable creates a new Invocable for the BKC compile capability.
func NewCompileInvocable(env agentenv.AgentEnvironment) execution.Invocable {
	return &compileInvocable{
		cap: NewCompileCapability(env),
		env: env,
	}
}

func (c *compileInvocable) ID() string { return euclorelurpic.CapabilityBKCCompile }

func (c *compileInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	return c.executeBKC(ctx, in, c.cap)
}

func (c *compileInvocable) IsPrimary() bool { return false }

// streamInvocable wraps streamCapability as an Invocable.
type streamInvocable struct {
	cap euclotypes.EucloCodingCapability
	env agentenv.AgentEnvironment
}

// NewStreamInvocable creates a new Invocable for the BKC stream capability.
func NewStreamInvocable(env agentenv.AgentEnvironment) execution.Invocable {
	return &streamInvocable{
		cap: NewStreamCapability(env),
		env: env,
	}
}

func (s *streamInvocable) ID() string { return euclorelurpic.CapabilityBKCStream }

func (s *streamInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	return s.executeBKC(ctx, in, s.cap)
}

func (s *streamInvocable) IsPrimary() bool { return false }

// checkpointInvocable wraps checkpointCapability as an Invocable.
type checkpointInvocable struct {
	cap euclotypes.EucloCodingCapability
	env agentenv.AgentEnvironment
}

// NewCheckpointInvocable creates a new Invocable for the BKC checkpoint capability.
func NewCheckpointInvocable(env agentenv.AgentEnvironment) execution.Invocable {
	return &checkpointInvocable{
		cap: NewCheckpointCapability(env),
		env: env,
	}
}

func (c *checkpointInvocable) ID() string { return euclorelurpic.CapabilityBKCCheckpoint }

func (c *checkpointInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	return c.executeBKC(ctx, in, c.cap)
}

func (c *checkpointInvocable) IsPrimary() bool { return false }

// invalidateInvocable wraps invalidateCapability as an Invocable.
type invalidateInvocable struct {
	cap euclotypes.EucloCodingCapability
	env agentenv.AgentEnvironment
}

// NewInvalidateInvocable creates a new Invocable for the BKC invalidate capability.
func NewInvalidateInvocable(env agentenv.AgentEnvironment) execution.Invocable {
	return &invalidateInvocable{
		cap: NewInvalidateCapability(env),
		env: env,
	}
}

func (i *invalidateInvocable) ID() string { return euclorelurpic.CapabilityBKCInvalidate }

func (i *invalidateInvocable) Invoke(ctx context.Context, in execution.InvokeInput) (*core.Result, error) {
	return i.executeBKC(ctx, in, i.cap)
}

func (i *invalidateInvocable) IsPrimary() bool { return false }

// executeBKC executes a BKC capability and converts the result to core.Result.
func executeBKC(ctx context.Context, in execution.InvokeInput, cap euclotypes.EucloCodingCapability) (*core.Result, error) {
	// Build CapabilitySnapshot from the registry tool permissions
	snapshot := eucloruntime.SnapshotCapabilities(in.Environment.Registry)

	artifactState := euclotypes.ArtifactStateFromContext(in.State)
	eligibility := cap.Eligible(artifactState, snapshot)
	if !eligibility.Eligible {
		return &core.Result{
			Success: false,
			Error:   errors.New(eligibility.Reason),
			Data:    map[string]any{"eligibility": eligibility},
		}, nil
	}

	// Get plan and workflow stores from service bundle
	var planStore frameworkplan.PlanStore
	var workflowStore memory.WorkflowStateStore
	if sb := in.ServiceBundle; sb.PlanStore != nil {
		planStore = sb.PlanStore
	}
	if sb := in.ServiceBundle; sb.WorkflowStore != nil {
		workflowStore = sb.WorkflowStore.(memory.WorkflowStateStore)
	}

	envelope := euclotypes.ExecutionEnvelope{
		Task:          in.Task,
		Mode:          in.Mode,
		Profile:       in.Profile,
		Registry:      in.Environment.Registry,
		State:         in.State,
		Memory:        in.Environment.Memory,
		Environment:   in.Environment,
		PlanStore:     planStore,
		WorkflowStore: workflowStore,
		Telemetry:     in.Telemetry,
	}

	result := cap.Execute(ctx, envelope)

	var err error
	if result.FailureInfo != nil {
		err = errors.New(result.FailureInfo.Message)
	}
	return &core.Result{
		Success: result.Status == euclotypes.ExecutionStatusCompleted,
		Data: map[string]any{
			"summary":   result.Summary,
			"artifacts": result.Artifacts,
			"status":    result.Status,
		},
		Error: err,
	}, nil
}

// Helper method to allow the invocables to call executeBKC
func (c *compileInvocable) executeBKC(ctx context.Context, in execution.InvokeInput, cap euclotypes.EucloCodingCapability) (*core.Result, error) {
	return executeBKC(ctx, in, cap)
}

func (s *streamInvocable) executeBKC(ctx context.Context, in execution.InvokeInput, cap euclotypes.EucloCodingCapability) (*core.Result, error) {
	return executeBKC(ctx, in, cap)
}

func (c *checkpointInvocable) executeBKC(ctx context.Context, in execution.InvokeInput, cap euclotypes.EucloCodingCapability) (*core.Result, error) {
	return executeBKC(ctx, in, cap)
}

func (i *invalidateInvocable) executeBKC(ctx context.Context, in execution.InvokeInput, cap euclotypes.EucloCodingCapability) (*core.Result, error) {
	return executeBKC(ctx, in, cap)
}
