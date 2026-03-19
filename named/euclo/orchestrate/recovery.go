package orchestrate

import (
	"context"
	"fmt"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

// RecoveryLevel indicates the scope at which a recovery attempt operates.
type RecoveryLevel string

const (
	RecoveryLevelParadigm   RecoveryLevel = "paradigm"
	RecoveryLevelCapability RecoveryLevel = "capability"
	RecoveryLevelProfile    RecoveryLevel = "profile"
	RecoveryLevelMode       RecoveryLevel = "mode"
)

// RecoveryAttempt records a single recovery action within a recovery stack.
type RecoveryAttempt struct {
	Level    RecoveryLevel
	Strategy euclotypes.RecoveryStrategy
	From     string
	To       string
	Reason   string
	Success  bool
}

// RecoveryStack tracks all recovery attempts during a profile execution,
// enforcing a maximum depth to prevent infinite recovery loops.
type RecoveryStack struct {
	Attempts  []RecoveryAttempt
	MaxDepth  int
	Exhausted bool
}

// NewRecoveryStack creates a RecoveryStack with the default max depth of 3.
func NewRecoveryStack() *RecoveryStack {
	return &RecoveryStack{MaxDepth: 3}
}

// CanAttempt returns true if the stack has not exceeded its maximum depth
// and is not marked exhausted.
func (s *RecoveryStack) CanAttempt() bool {
	if s == nil {
		return false
	}
	return !s.Exhausted && len(s.Attempts) < s.MaxDepth
}

// Record appends a recovery attempt to the stack and marks it exhausted
// if the maximum depth has been reached.
func (s *RecoveryStack) Record(attempt RecoveryAttempt) {
	if s == nil {
		return
	}
	s.Attempts = append(s.Attempts, attempt)
	if len(s.Attempts) >= s.MaxDepth {
		s.Exhausted = true
	}
}

// RecoveryController processes RecoveryHint values from failed capabilities
// and attempts structured fallback at multiple levels: paradigm switch,
// capability fallback, profile escalation, and mode escalation.
type RecoveryController struct {
	Capabilities CapabilityRegistryI
	Profiles     *euclotypes.ExecutionProfileRegistry
	Modes        *euclotypes.ModeRegistry
	Environment  agentenv.AgentEnvironment
}

// NewRecoveryController creates a RecoveryController with the given registries.
func NewRecoveryController(
	caps CapabilityRegistryI,
	profiles *euclotypes.ExecutionProfileRegistry,
	modes *euclotypes.ModeRegistry,
	env agentenv.AgentEnvironment,
) *RecoveryController {
	return &RecoveryController{
		Capabilities: caps,
		Profiles:     profiles,
		Modes:        modes,
		Environment:  env,
	}
}

// AttemptRecovery processes a RecoveryHint from a failed capability execution
// and attempts a single recovery action. It records the attempt in the
// recovery stack and returns the result of the recovery (or the original
// failed result if recovery is not possible).
func (rc *RecoveryController) AttemptRecovery(
	ctx context.Context,
	hint euclotypes.RecoveryHint,
	failedResult euclotypes.ExecutionResult,
	env euclotypes.ExecutionEnvelope,
	stack *RecoveryStack,
) euclotypes.ExecutionResult {
	if rc == nil || stack == nil || !stack.CanAttempt() {
		return failedResult
	}

	switch hint.Strategy {
	case euclotypes.RecoveryStrategyParadigmSwitch:
		return rc.attemptParadigmSwitch(ctx, hint, failedResult, env, stack)
	case euclotypes.RecoveryStrategyCapabilityFallback:
		return rc.attemptCapabilityFallback(ctx, hint, failedResult, env, stack)
	case euclotypes.RecoveryStrategyProfileEscalation:
		return rc.attemptProfileEscalation(ctx, hint, failedResult, env, stack)
	case euclotypes.RecoveryStrategyModeEscalation:
		return rc.handleModeEscalation(hint, failedResult, stack)
	default:
		return failedResult
	}
}

// attemptParadigmSwitch annotates the envelope with the suggested paradigm
// and re-executes the same capability. The capability's Execute() must
// respect the paradigm override in the envelope context.
func (rc *RecoveryController) attemptParadigmSwitch(
	ctx context.Context,
	hint euclotypes.RecoveryHint,
	failedResult euclotypes.ExecutionResult,
	env euclotypes.ExecutionEnvelope,
	stack *RecoveryStack,
) euclotypes.ExecutionResult {
	if hint.SuggestedParadigm == "" {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelParadigm,
			Strategy: hint.Strategy,
			From:     paradigmFromFailure(failedResult),
			To:       "",
			Reason:   "no suggested paradigm",
			Success:  false,
		})
		return failedResult
	}

	// Find the original capability that failed.
	producerID := producerIDFromFailure(failedResult)
	if producerID == "" || rc.Capabilities == nil {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelParadigm,
			Strategy: hint.Strategy,
			From:     paradigmFromFailure(failedResult),
			To:       hint.SuggestedParadigm,
			Reason:   "cannot identify failed capability",
			Success:  false,
		})
		return failedResult
	}

	cap, ok := rc.Capabilities.Lookup(producerID)
	if !ok {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelParadigm,
			Strategy: hint.Strategy,
			From:     paradigmFromFailure(failedResult),
			To:       hint.SuggestedParadigm,
			Reason:   fmt.Sprintf("capability %s not found", producerID),
			Success:  false,
		})
		return failedResult
	}

	// Annotate envelope with paradigm override.
	retryEnv := env
	if retryEnv.Task != nil {
		if retryEnv.Task.Context == nil {
			retryEnv.Task.Context = map[string]any{}
		}
		retryEnv.Task.Context["euclo.paradigm_override"] = hint.SuggestedParadigm
		retryEnv.Task.Context["euclo.recovery_active"] = true
	}

	result := cap.Execute(ctx, retryEnv)
	success := result.Status != euclotypes.ExecutionStatusFailed
	stack.Record(RecoveryAttempt{
		Level:    RecoveryLevelParadigm,
		Strategy: hint.Strategy,
		From:     paradigmFromFailure(failedResult),
		To:       hint.SuggestedParadigm,
		Reason:   fmt.Sprintf("paradigm switch from %s", paradigmFromFailure(failedResult)),
		Success:  success,
	})
	if success {
		return result
	}
	return failedResult
}

// attemptCapabilityFallback looks up the suggested capability and executes it
// as a replacement for the failed capability.
func (rc *RecoveryController) attemptCapabilityFallback(
	ctx context.Context,
	hint euclotypes.RecoveryHint,
	failedResult euclotypes.ExecutionResult,
	env euclotypes.ExecutionEnvelope,
	stack *RecoveryStack,
) euclotypes.ExecutionResult {
	if rc.Capabilities == nil {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelCapability,
			Strategy: hint.Strategy,
			From:     producerIDFromFailure(failedResult),
			To:       hint.SuggestedCapability,
			Reason:   "capability registry unavailable",
			Success:  false,
		})
		return failedResult
	}

	// Check eligibility.
	artifacts := euclotypes.ArtifactStateFromContext(env.State)
	snapshot := snapshotFromEnv(env)

	candidates := fallbackCapabilitiesFromHint(hint)
	if len(candidates) == 0 {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelCapability,
			Strategy: hint.Strategy,
			From:     producerIDFromFailure(failedResult),
			To:       "",
			Reason:   "no suggested capability",
			Success:  false,
		})
		return failedResult
	}

	failureReason := "no eligible capability in fallback chain"
	failureTarget := firstFallbackCapability(candidates)
	for _, candidateID := range candidates {
		cap, ok := rc.Capabilities.Lookup(candidateID)
		if !ok {
			failureReason = fmt.Sprintf("capability %s not found", candidateID)
			failureTarget = candidateID
			continue
		}
		eligibility := cap.Eligible(artifacts, snapshot)
		if !eligibility.Eligible {
			failureReason = fmt.Sprintf("fallback not eligible: %s", eligibility.Reason)
			failureTarget = candidateID
			continue
		}
		retryEnv := env
		if retryEnv.Task != nil {
			if retryEnv.Task.Context == nil {
				retryEnv.Task.Context = map[string]any{}
			}
			retryEnv.Task.Context["euclo.recovery_active"] = true
		}
		result := cap.Execute(ctx, retryEnv)
		success := result.Status != euclotypes.ExecutionStatusFailed
		if success {
			stack.Record(RecoveryAttempt{
				Level:    RecoveryLevelCapability,
				Strategy: hint.Strategy,
				From:     producerIDFromFailure(failedResult),
				To:       candidateID,
				Reason:   "capability fallback",
				Success:  true,
			})
			return result
		}
		failureReason = "capability fallback"
		failureTarget = candidateID
	}

	stack.Record(RecoveryAttempt{
		Level:    RecoveryLevelCapability,
		Strategy: hint.Strategy,
		From:     producerIDFromFailure(failedResult),
		To:       failureTarget,
		Reason:   failureReason,
		Success:  false,
	})
	return failedResult
}

// attemptProfileEscalation looks up a fallback profile from the current
// profile descriptor and re-runs profile execution with the new profile.
func (rc *RecoveryController) attemptProfileEscalation(
	ctx context.Context,
	hint euclotypes.RecoveryHint,
	failedResult euclotypes.ExecutionResult,
	env euclotypes.ExecutionEnvelope,
	stack *RecoveryStack,
) euclotypes.ExecutionResult {
	if rc.Profiles == nil {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelProfile,
			Strategy: hint.Strategy,
			From:     env.Profile.ProfileID,
			To:       "",
			Reason:   "profile registry unavailable",
			Success:  false,
		})
		return failedResult
	}

	// Get fallback profiles from the current profile's descriptor.
	currentDesc, ok := rc.Profiles.Lookup(env.Profile.ProfileID)
	if !ok || len(currentDesc.FallbackProfiles) == 0 {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelProfile,
			Strategy: hint.Strategy,
			From:     env.Profile.ProfileID,
			To:       "",
			Reason:   "no fallback profiles available",
			Success:  false,
		})
		return failedResult
	}

	// Try the preferred fallback profile first when provided.
	fallbackID := preferredFallbackProfile(hint, currentDesc.FallbackProfiles)
	fallbackDesc, ok := rc.Profiles.Lookup(fallbackID)
	if !ok {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelProfile,
			Strategy: hint.Strategy,
			From:     env.Profile.ProfileID,
			To:       fallbackID,
			Reason:   fmt.Sprintf("fallback profile %s not found", fallbackID),
			Success:  false,
		})
		return failedResult
	}

	// Build a new profile selection for the fallback.
	fallbackProfile := euclotypes.ExecutionProfileSelection{
		ProfileID:            fallbackDesc.ProfileID,
		FallbackProfileIDs:   fallbackDesc.FallbackProfiles,
		RequiredArtifacts:    fallbackDesc.RequiredArtifacts,
		CompletionContract:   fallbackDesc.CompletionContract,
		PhaseRoutes:          fallbackDesc.PhaseRoutes,
		MutationAllowed:      fallbackDesc.MutationPolicy == "allowed",
		VerificationRequired: fallbackDesc.VerificationRequired,
	}

	// Find a profile-level capability for the fallback profile.
	artifacts := euclotypes.ArtifactStateFromContext(env.State)
	snapshot := snapshotFromEnv(env)
	candidates := rc.Capabilities.ForProfile(fallbackID)
	var fallbackCap CapabilityI
	for _, cap := range candidates {
		if eligibility := cap.Eligible(artifacts, snapshot); eligibility.Eligible {
			fallbackCap = cap
			break
		}
	}

	if fallbackCap == nil {
		stack.Record(RecoveryAttempt{
			Level:    RecoveryLevelProfile,
			Strategy: hint.Strategy,
			From:     env.Profile.ProfileID,
			To:       fallbackID,
			Reason:   "no eligible capability for fallback profile",
			Success:  false,
		})
		return failedResult
	}

	// Execute with the fallback profile.
	fallbackEnv := env
	fallbackEnv.Profile = fallbackProfile
	if fallbackEnv.Task != nil {
		if fallbackEnv.Task.Context == nil {
			fallbackEnv.Task.Context = map[string]any{}
		}
		fallbackEnv.Task.Context["euclo.recovery_active"] = true
	}
	result := fallbackCap.Execute(ctx, fallbackEnv)
	success := result.Status != euclotypes.ExecutionStatusFailed
	stack.Record(RecoveryAttempt{
		Level:    RecoveryLevelProfile,
		Strategy: hint.Strategy,
		From:     env.Profile.ProfileID,
		To:       fallbackID,
		Reason:   "profile escalation",
		Success:  success,
	})
	if success {
		return result
	}
	return failedResult
}

func fallbackCapabilitiesFromHint(hint euclotypes.RecoveryHint) []string {
	var out []string
	if hint.SuggestedCapability != "" {
		out = append(out, hint.SuggestedCapability)
	}
	if hint.Context == nil {
		return out
	}
	if raw, ok := hint.Context["fallback_capabilities"].([]string); ok {
		out = append(out, raw...)
		return uniqueRecoveryStrings(out)
	}
	if raw, ok := hint.Context["fallback_capabilities"].([]any); ok {
		for _, item := range raw {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
	}
	return uniqueRecoveryStrings(out)
}

func firstFallbackCapability(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func preferredFallbackProfile(hint euclotypes.RecoveryHint, defaults []string) string {
	if hint.Context != nil {
		if preferred, ok := hint.Context["preferred_profile"].(string); ok && preferred != "" {
			return preferred
		}
	}
	if len(defaults) == 0 {
		return ""
	}
	return defaults[0]
}

func uniqueRecoveryStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// handleModeEscalation does NOT auto-execute a mode change. Instead, it
// returns a partial result with an escalation recommendation. Mode changes
// require user approval via HITL, so we only signal the recommendation.
func (rc *RecoveryController) handleModeEscalation(
	hint euclotypes.RecoveryHint,
	failedResult euclotypes.ExecutionResult,
	stack *RecoveryStack,
) euclotypes.ExecutionResult {
	stack.Record(RecoveryAttempt{
		Level:    RecoveryLevelMode,
		Strategy: hint.Strategy,
		From:     "",
		To:       "",
		Reason:   "mode escalation recommended (requires user approval)",
		Success:  false,
	})

	// Return the original failure with an enriched recovery hint indicating
	// that mode escalation was recommended but not executed.
	result := failedResult
	result.RecoveryHint = &euclotypes.RecoveryHint{
		Strategy: euclotypes.RecoveryStrategyModeEscalation,
		Context: map[string]any{
			"escalation_status": "recommended",
			"requires_approval": true,
			"original_hint":     hint,
		},
	}
	return result
}

// RecoveryTraceArtifact creates an artifact recording the recovery stack
// for observability and debugging.
func RecoveryTraceArtifact(stack *RecoveryStack, producerID string) euclotypes.Artifact {
	attempts := make([]map[string]any, 0, len(stack.Attempts))
	for _, a := range stack.Attempts {
		attempts = append(attempts, map[string]any{
			"level":    string(a.Level),
			"strategy": string(a.Strategy),
			"from":     a.From,
			"to":       a.To,
			"reason":   a.Reason,
			"success":  a.Success,
		})
	}
	return euclotypes.Artifact{
		ID:         "recovery_trace",
		Kind:       euclotypes.ArtifactKindRecoveryTrace,
		Summary:    fmt.Sprintf("%d recovery attempts, exhausted=%v", len(stack.Attempts), stack.Exhausted),
		ProducerID: producerID,
		Status:     "produced",
		Payload: map[string]any{
			"attempts":  attempts,
			"max_depth": stack.MaxDepth,
			"exhausted": stack.Exhausted,
		},
	}
}

// Helper functions for extracting info from failed results.

func paradigmFromFailure(result euclotypes.ExecutionResult) string {
	if result.FailureInfo != nil && result.FailureInfo.ParadigmUsed != "" {
		return result.FailureInfo.ParadigmUsed
	}
	return "unknown"
}

func producerIDFromFailure(result euclotypes.ExecutionResult) string {
	for _, art := range result.Artifacts {
		if art.ProducerID != "" {
			return art.ProducerID
		}
	}
	if result.FailureInfo != nil {
		return result.FailureInfo.Code
	}
	return ""
}
