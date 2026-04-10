package orchestrate

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/interaction/gate"
)

type profileExecutionPlan struct {
	Phases []string
	Gates  []gate.PhaseGate
}

func newProfileExecutionPlan(profile euclotypes.ExecutionProfileSelection, gates []gate.PhaseGate) profileExecutionPlan {
	return profileExecutionPlan{
		Phases: OrderedPhases(profile.PhaseRoutes, gates),
		Gates:  gates,
	}
}

type profileExecutionEngine struct {
	controller *ProfileController
}

func newProfileExecutionEngine(pc *ProfileController) *profileExecutionEngine {
	return &profileExecutionEngine{controller: pc}
}

func (e *profileExecutionEngine) Execute(
	ctx context.Context,
	profile euclotypes.ExecutionProfileSelection,
	mode euclotypes.ModeResolution,
	env euclotypes.ExecutionEnvelope,
) (*core.Result, *ProfileControllerResult, error) {
	plan := newProfileExecutionPlan(profile, e.gatesFor(profile.ProfileID))
	artifacts := euclotypes.ArtifactStateFromContext(env.State)
	snapshot := snapshotFromEnv(env)
	pcResult := &ProfileControllerResult{}
	recoveryStack := NewRecoveryStack()

	profileCap := e.resolveProfileCapability(profile.ProfileID, artifacts, snapshot)
	if profileCap != nil {
		return e.executeProfileCapabilityPath(ctx, profile, mode, env, plan, artifacts, pcResult, recoveryStack, profileCap)
	}

	return e.executePhasePath(ctx, profile, mode, env, plan, artifacts, snapshot, pcResult, recoveryStack)
}

func (e *profileExecutionEngine) executeProfileCapabilityPath(
	ctx context.Context,
	profile euclotypes.ExecutionProfileSelection,
	mode euclotypes.ModeResolution,
	env euclotypes.ExecutionEnvelope,
	plan profileExecutionPlan,
	artifacts euclotypes.ArtifactState,
	pcResult *ProfileControllerResult,
	recoveryStack *RecoveryStack,
	profileCap CapabilityI,
) (*core.Result, *ProfileControllerResult, error) {
	initialConsumed := artifactKindsFromState(artifacts)
	capResult := profileCap.Execute(ctx, env)
	capID := profileCap.Descriptor().ID
	pcResult.CapabilityIDs = append(pcResult.CapabilityIDs, capID)
	pcResult.PhasesExecuted = plan.Phases

	pcResult.Artifacts = append(pcResult.Artifacts, capResult.Artifacts...)
	mergeCapabilityArtifactsToState(env.State, capResult.Artifacts)
	artifacts = euclotypes.ArtifactStateFromContext(env.State)

	for _, g := range plan.Gates {
		eval := gate.EvaluateGate(g, mode.ModeID, artifacts)
		pcResult.GateEvals = append(pcResult.GateEvals, eval)
	}

	if shouldAttemptRecovery(capResult) && e.recovery() != nil {
		recoveredResult := e.recovery().AttemptRecovery(ctx, *capResult.RecoveryHint, capResult, env, recoveryStack)
		if recoveredResult.Status != euclotypes.ExecutionStatusFailed {
			capResult = recoveredResult
			pcResult.Artifacts = append(pcResult.Artifacts, recoveredResult.Artifacts...)
			mergeCapabilityArtifactsToState(env.State, recoveredResult.Artifacts)
			artifacts = euclotypes.ArtifactStateFromContext(env.State)
		}
	}

	e.recordRecoveryTrace(env, pcResult, recoveryStack)
	pcResult.PhaseRecords = buildProfileCapabilityPhaseRecords(plan.Phases, initialConsumed, artifacts.All())
	recordProfileControllerObservability(env.State, pcResult, mode, profile)

	if capResult.Status != euclotypes.ExecutionStatusCompleted {
		return failedResult(env.Task, capResult, pcResult), pcResult,
			fmt.Errorf("capability %s failed: %s", capID, capResult.Summary)
	}

	return successResult(env.Task, capResult, pcResult), pcResult, nil
}

func (e *profileExecutionEngine) executePhasePath(
	ctx context.Context,
	profile euclotypes.ExecutionProfileSelection,
	mode euclotypes.ModeResolution,
	env euclotypes.ExecutionEnvelope,
	plan profileExecutionPlan,
	artifacts euclotypes.ArtifactState,
	snapshot euclotypes.CapabilitySnapshot,
	pcResult *ProfileControllerResult,
	recoveryStack *RecoveryStack,
) (*core.Result, *ProfileControllerResult, error) {
	gateIndex := 0
	for i, phase := range plan.Phases {
		if gateIndex < len(plan.Gates) && i > 0 {
			g := plan.Gates[gateIndex]
			eval := gate.EvaluateGate(g, mode.ModeID, artifacts)
			pcResult.GateEvals = append(pcResult.GateEvals, eval)
			if !eval.Passed {
				switch eval.Policy {
				case gate.GateFailBlock:
					pcResult.EarlyStop = true
					pcResult.EarlyStopPhase = phase
					e.recordRecoveryTrace(env, pcResult, recoveryStack)
					recordProfileControllerObservability(env.State, pcResult, mode, profile)
					return partialResult(env.Task, pcResult), pcResult, nil
				case gate.GateFailWarn:
				case gate.GateFailSkip:
					gateIndex++
					continue
				}
			}
			gateIndex++
		}

		phaseCap := e.resolveCapabilityForPhase(phase, profile.ProfileID, artifacts, snapshot)
		if phaseCap == nil {
			continue
		}

		consumedKinds := artifactKindsFromState(artifacts)
		capResult := phaseCap.Execute(ctx, env)
		capID := phaseCap.Descriptor().ID
		pcResult.CapabilityIDs = append(pcResult.CapabilityIDs, capID)
		pcResult.PhasesExecuted = append(pcResult.PhasesExecuted, phase)
		pcResult.PhaseRecords = append(pcResult.PhaseRecords, PhaseArtifactRecord{
			Phase:             phase,
			ArtifactsProduced: append([]euclotypes.Artifact{}, capResult.Artifacts...),
			ArtifactsConsumed: consumedKinds,
		})

		pcResult.Artifacts = append(pcResult.Artifacts, capResult.Artifacts...)
		mergeCapabilityArtifactsToState(env.State, capResult.Artifacts)
		artifacts = euclotypes.ArtifactStateFromContext(env.State)

		if capResult.Status != euclotypes.ExecutionStatusCompleted {
			recovered := false

			if shouldAttemptRecovery(capResult) && e.recovery() != nil && recoveryStack.CanAttempt() {
				recoveredResult := e.recovery().AttemptRecovery(ctx, *capResult.RecoveryHint, capResult, env, recoveryStack)
				if recoveredResult.Status == euclotypes.ExecutionStatusCompleted {
					pcResult.Artifacts = append(pcResult.Artifacts, recoveredResult.Artifacts...)
					mergeCapabilityArtifactsToState(env.State, recoveredResult.Artifacts)
					artifacts = euclotypes.ArtifactStateFromContext(env.State)
					recovered = true
				}
			}

			if !recovered {
				fallbackCap := e.resolveFallbackCapability(phase, profile.ProfileID, capID, artifacts, snapshot)
				if fallbackCap != nil {
					fallbackResult := fallbackCap.Execute(ctx, env)
					fallbackID := fallbackCap.Descriptor().ID
					pcResult.CapabilityIDs = append(pcResult.CapabilityIDs, fallbackID)
					pcResult.Artifacts = append(pcResult.Artifacts, fallbackResult.Artifacts...)
					mergeCapabilityArtifactsToState(env.State, fallbackResult.Artifacts)
					artifacts = euclotypes.ArtifactStateFromContext(env.State)
					if fallbackResult.Status != euclotypes.ExecutionStatusFailed {
						recovered = true
					}
				}
			}

			if !recovered {
				pcResult.EarlyStop = true
				pcResult.EarlyStopPhase = phase
				e.recordRecoveryTrace(env, pcResult, recoveryStack)
				recordProfileControllerObservability(env.State, pcResult, mode, profile)
				return failedResult(env.Task, capResult, pcResult), pcResult,
					fmt.Errorf("capability %s failed at phase %s: %s", capID, phase, capResult.Summary)
			}
		}
	}

	e.recordRecoveryTrace(env, pcResult, recoveryStack)
	recordProfileControllerObservability(env.State, pcResult, mode, profile)
	return completedResult(env.Task, pcResult), pcResult, nil
}

func (e *profileExecutionEngine) recordRecoveryTrace(
	env euclotypes.ExecutionEnvelope,
	pcResult *ProfileControllerResult,
	recoveryStack *RecoveryStack,
) {
	if recoveryStack == nil || len(recoveryStack.Attempts) == 0 {
		return
	}
	traceArt := RecoveryTraceArtifact(recoveryStack, "euclo:profile_controller")
	pcResult.Artifacts = append(pcResult.Artifacts, traceArt)
	pcResult.RecoveryAttempts = len(recoveryStack.Attempts)
	if env.State != nil && traceArt.Payload != nil {
		env.State.Set("euclo.recovery_trace", traceArt.Payload)
	}
}

func (e *profileExecutionEngine) resolveProfileCapability(
	profileID string,
	artifacts euclotypes.ArtifactState,
	snapshot euclotypes.CapabilitySnapshot,
) CapabilityI {
	if e.controller == nil {
		return nil
	}
	return e.controller.resolveProfileCapability(profileID, artifacts, snapshot)
}

func (e *profileExecutionEngine) resolveCapabilityForPhase(
	phase, profileID string,
	artifacts euclotypes.ArtifactState,
	snapshot euclotypes.CapabilitySnapshot,
) CapabilityI {
	if e.controller == nil {
		return nil
	}
	return e.controller.resolveCapabilityForPhase(phase, profileID, artifacts, snapshot)
}

func (e *profileExecutionEngine) resolveFallbackCapability(
	phase, profileID, excludeID string,
	artifacts euclotypes.ArtifactState,
	snapshot euclotypes.CapabilitySnapshot,
) CapabilityI {
	if e.controller == nil {
		return nil
	}
	return e.controller.resolveFallbackCapability(phase, profileID, excludeID, artifacts, snapshot)
}

func (e *profileExecutionEngine) gatesFor(profileID string) []gate.PhaseGate {
	if e.controller == nil || e.controller.Gates == nil {
		return nil
	}
	return e.controller.Gates[profileID]
}

func (e *profileExecutionEngine) recovery() *RecoveryController {
	if e.controller == nil {
		return nil
	}
	return e.controller.Recovery
}
