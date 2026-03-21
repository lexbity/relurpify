package gateway

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/rex/events"
)

type SignalDecision string

const (
	SignalDecisionStart  SignalDecision = "start"
	SignalDecisionSignal SignalDecision = "signal"
	SignalDecisionReject SignalDecision = "reject"
)

type Decision struct {
	Decision   SignalDecision `json:"decision"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	Reason     string         `json:"reason,omitempty"`
}

type WorkflowStateReader interface {
	GetWorkflow(context.Context, string) (*memory.WorkflowRecord, bool, error)
	GetRun(context.Context, string) (*memory.WorkflowRunRecord, bool, error)
}

// WorkflowGateway is the v2 rex gateway contract.
type WorkflowGateway interface {
	IdentityFor(events.CanonicalEvent) string
	Decide(events.CanonicalEvent) SignalDecision
	Resolve(context.Context, events.CanonicalEvent) (Decision, error)
}

// DefaultGateway implements deterministic identity and workflow-aware start/signal rules.
type DefaultGateway struct {
	Store WorkflowStateReader
}

func (g DefaultGateway) IdentityFor(event events.CanonicalEvent) string {
	if workflowID := strings.TrimSpace(stringValue(event.Payload["workflow_id"])); workflowID != "" {
		return workflowID
	}
	sum := sha1.Sum([]byte(strings.Join([]string{
		strings.TrimSpace(event.Type),
		strings.TrimSpace(event.ActorID),
		strings.TrimSpace(event.Partition),
		strings.TrimSpace(event.IdempotencyKey),
	}, "::")))
	return "rexwf:" + hex.EncodeToString(sum[:8])
}

func (g DefaultGateway) Decide(event events.CanonicalEvent) SignalDecision {
	decision, err := g.Resolve(context.Background(), event)
	if err != nil {
		return SignalDecisionReject
	}
	return decision.Decision
}

func (g DefaultGateway) Resolve(ctx context.Context, event events.CanonicalEvent) (Decision, error) {
	event, err := events.DefaultNormalizer{}.Normalize(event)
	if err != nil {
		return Decision{Decision: SignalDecisionReject, Reason: "invalid_event"}, err
	}
	workflowID := g.IdentityFor(event)
	runID := firstNonEmpty(stringValue(event.Payload["run_id"]), workflowID+":run")

	switch classifyEvent(event.Type) {
	case SignalDecisionStart:
			if err := g.ensureStartAllowed(ctx, workflowID); err != nil {
				return Decision{Decision: SignalDecisionReject, WorkflowID: workflowID, RunID: runID, Reason: "start_rejected"}, err
			}
			if g.hasWorkflow(ctx, workflowID) {
				return Decision{Decision: SignalDecisionSignal, WorkflowID: workflowID, RunID: runID, Reason: "existing_workflow"}, nil
			}
		return Decision{Decision: SignalDecisionStart, WorkflowID: workflowID, RunID: runID, Reason: "new_workflow"}, nil
	case SignalDecisionSignal:
		if err := g.validateSignalEvent(ctx, workflowID, runID, event); err != nil {
			return Decision{Decision: SignalDecisionReject, WorkflowID: workflowID, RunID: runID, Reason: "signal_rejected"}, err
		}
		return Decision{Decision: SignalDecisionSignal, WorkflowID: workflowID, RunID: runID, Reason: "signal_accepted"}, nil
	default:
		return Decision{Decision: SignalDecisionReject, WorkflowID: workflowID, RunID: runID, Reason: "unsupported_event"}, fmt.Errorf("unsupported event type %q", event.Type)
	}
}

func (g DefaultGateway) ensureStartAllowed(ctx context.Context, workflowID string) error {
	if g.Store == nil {
		return nil
	}
	workflow, ok, err := g.Store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	switch workflow.Status {
	case memory.WorkflowRunStatusCompleted, memory.WorkflowRunStatusFailed, memory.WorkflowRunStatusCanceled:
		return nil
	default:
		return nil
	}
}

func (g DefaultGateway) validateSignalEvent(ctx context.Context, workflowID, runID string, event events.CanonicalEvent) error {
	if strings.TrimSpace(workflowID) == "" {
		return errors.New("workflow identity required")
	}
	if g.Store != nil {
		workflow, ok, err := g.Store.GetWorkflow(ctx, workflowID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("workflow not found")
		}
		if workflow.Status == memory.WorkflowRunStatusCompleted || workflow.Status == memory.WorkflowRunStatusFailed {
			return errors.New("stale workflow signal")
		}
		if runID != "" {
			if run, ok, err := g.Store.GetRun(ctx, runID); err != nil {
				return err
			} else if ok {
				if run.Status == memory.WorkflowRunStatusCompleted || run.Status == memory.WorkflowRunStatusFailed {
					return errors.New("stale run signal")
				}
			}
		}
	}
	switch event.Type {
	case events.TypeCallbackReceived:
		expected := stringValue(event.Payload["expected_callback"])
		received := firstNonEmpty(stringValue(event.Payload["callback_key"]), stringValue(event.Payload["signal_key"]))
		if err := ValidateSignal(expected, received); err != nil {
			return err
		}
	case events.TypeWorkflowSignal:
		expected := stringValue(event.Payload["expected_signal"])
		if expected != "" {
			received := firstNonEmpty(stringValue(event.Payload["signal"]), stringValue(event.Payload["signal_key"]))
			if err := ValidateSignal(expected, received); err != nil {
				return err
			}
		}
	}
	if event.TrustClass == events.TrustUntrusted {
		return errors.New("untrusted signals rejected")
	}
	return nil
}

func (g DefaultGateway) hasWorkflow(ctx context.Context, workflowID string) bool {
	if g.Store == nil {
		return false
	}
	_, ok, err := g.Store.GetWorkflow(ctx, workflowID)
	return err == nil && ok
}

func classifyEvent(eventType string) SignalDecision {
	switch eventType {
	case events.TypeWorkflowSignal, events.TypeCallbackReceived, events.TypeWorkflowResume:
		return SignalDecisionSignal
	case events.TypeTaskRequested:
		return SignalDecisionStart
	default:
		if strings.Contains(eventType, ".signal.") || strings.Contains(eventType, ".callback.") || strings.Contains(eventType, ".resume.") {
			return SignalDecisionSignal
		}
		if strings.TrimSpace(eventType) == "" {
			return SignalDecisionReject
		}
		return SignalDecisionStart
	}
}

func ValidateSignal(expected, received string) error {
	if expected == "" || received == "" {
		return errors.New("signal identifiers required")
	}
	if expected != received {
		return errors.New("stale or unexpected signal")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringValue(raw any) string {
	if raw == nil {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "<nil>" {
		return ""
	}
	return value
}
