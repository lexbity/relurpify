package capability

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// AddPrecheck appends a pre-invocation guard to the registry.
func (r *CapabilityRegistry) AddPrecheck(p InvocationPrecheck) {
	if r == nil || p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prechecks = append(r.prechecks, p)
}

// AddPostcheck appends a post-invocation hook to the registry.
func (r *CapabilityRegistry) AddPostcheck(p PostInvocationHook) {
	if r == nil || p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.postchecks = append(r.postchecks, p)
}

// SetGuidanceBroker configures optional guidance routing for recoverable precheck failures.
func (r *CapabilityRegistry) SetGuidanceBroker(broker RecoveryGuidanceBroker) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.guidanceBroker = broker
}

func (r *CapabilityRegistry) runPrechecks(desc CapabilityDescriptor, args map[string]interface{}) error {
	r.mu.RLock()
	prechecks := append([]InvocationPrecheck{}, r.prechecks...)
	r.mu.RUnlock()
	for _, precheck := range prechecks {
		if err := precheck.Check(desc, args); err != nil {
			return fmt.Errorf("capability %s blocked: %w", desc.ID, err)
		}
	}
	return nil
}

func (r *CapabilityRegistry) runPostchecks(desc CapabilityDescriptor, result *contracts.ToolResult) error {
	r.mu.RLock()
	postchecks := append([]PostInvocationHook{}, r.postchecks...)
	r.mu.RUnlock()
	for _, postcheck := range postchecks {
		if err := postcheck.Record(desc, result); err != nil {
			return err
		}
	}
	return nil
}

func (r *CapabilityRegistry) handleDoomLoopGuidance(ctx context.Context, doomErr DoomLoopError) (bool, error) {
	r.mu.RLock()
	broker := r.guidanceBroker
	r.mu.RUnlock()
	if broker == nil {
		return false, &doomErr
	}
	decision, err := broker.RequestRecovery(ctx, RecoveryGuidanceRequest{
		Title:       "Execution loop detected",
		Description: describeLoop(doomErr),
		Context: map[string]any{
			"doom_loop_kind": doomErr.Kind,
			"call_count":     doomErr.CallCount,
			"evidence":       append([]string(nil), doomErr.Evidence...),
		},
	})
	if err != nil {
		return false, err
	}
	switch decision.ChoiceID {
	case "continue":
		return true, nil
	case "skip":
		return false, fmt.Errorf("doom loop skipped by user")
	case "replan":
		return false, fmt.Errorf("doom loop requires replanning")
	case "stop", "":
		fallthrough
	default:
		return false, &doomErr
	}
}
