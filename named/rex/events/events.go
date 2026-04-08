package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/rex/envelope"
)

const (
	TrustInternal  = "internal"
	TrustTrusted   = "trusted"
	TrustUntrusted = "untrusted"
)

const (
	TypeTaskRequested    = "rex.task.requested.v1"
	TypeWorkflowResume   = "rex.workflow.resume.v1"
	TypeWorkflowSignal   = "rex.workflow.signal.v1"
	TypeCallbackReceived = "rex.callback.received.v1"
)

// CanonicalEvent is the transport-agnostic v2 rex event shape.
type CanonicalEvent struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Timestamp      time.Time      `json:"timestamp"`
	ActorID        string         `json:"actor_id,omitempty"`
	Partition      string         `json:"partition"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	TrustClass     string         `json:"trust_class"`
	Source         string         `json:"source,omitempty"`
}

// IngressAdapter normalizes inbound work into a canonical event.
type IngressAdapter interface {
	Name() string
	Normalize(payload map[string]any) (CanonicalEvent, error)
}

// EventNormalizer validates and normalizes canonical events.
type EventNormalizer interface {
	Normalize(CanonicalEvent) (CanonicalEvent, error)
}

// DefaultNormalizer applies canonical rex ingress validation.
type DefaultNormalizer struct{}

func (DefaultNormalizer) Normalize(event CanonicalEvent) (CanonicalEvent, error) {
	event.ID = strings.TrimSpace(event.ID)
	event.Type = strings.TrimSpace(event.Type)
	event.ActorID = strings.TrimSpace(event.ActorID)
	event.Partition = strings.TrimSpace(event.Partition)
	event.IdempotencyKey = strings.TrimSpace(event.IdempotencyKey)
	event.TrustClass = normalizeTrust(event.TrustClass)
	event.Source = strings.TrimSpace(event.Source)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	if event.ID == "" {
		event.ID = firstNonEmpty(stringValue(event.Payload["event_id"]), stringValue(event.Payload["task_id"]), stringValue(event.Payload["workflow_id"]))
	}
	if event.ID == "" {
		return CanonicalEvent{}, fmt.Errorf("canonical event id required")
	}
	if event.Type == "" {
		return CanonicalEvent{}, fmt.Errorf("canonical event type required")
	}
	if event.Partition == "" {
		event.Partition = "default"
	}
	if event.IdempotencyKey == "" {
		event.IdempotencyKey = firstNonEmpty(stringValue(event.Payload["idempotency_key"]), event.ID)
	}
	if event.TrustClass == "" {
		event.TrustClass = TrustUntrusted
	}
	switch event.TrustClass {
	case TrustInternal, TrustTrusted, TrustUntrusted:
	default:
		return CanonicalEvent{}, fmt.Errorf("trust class %q invalid", event.TrustClass)
	}
	if !AllowsIngress(event) {
		return CanonicalEvent{}, fmt.Errorf("event type %q rejected for trust class %q", event.Type, event.TrustClass)
	}
	return event, nil
}

// MapAdapter normalizes generic map payloads into canonical rex events.
type MapAdapter struct {
	NameID      string
	DefaultType string
	TrustClass  string
	Partition   string
	Source      string
	Normalizer  EventNormalizer
}

func (a MapAdapter) Name() string {
	if strings.TrimSpace(a.NameID) == "" {
		return "map"
	}
	return strings.TrimSpace(a.NameID)
}

func (a MapAdapter) Normalize(payload map[string]any) (CanonicalEvent, error) {
	event := CanonicalEvent{
		ID:             stringValue(payload["event_id"]),
		Type:           firstNonEmpty(stringValue(payload["type"]), a.DefaultType),
		Timestamp:      timeValue(payload["timestamp"]),
		ActorID:        firstNonEmpty(stringValue(payload["actor_id"]), stringValue(payload["actor"])),
		Partition:      firstNonEmpty(stringValue(payload["partition"]), a.Partition),
		IdempotencyKey: stringValue(payload["idempotency_key"]),
		Payload:        cloneMap(payload),
		TrustClass:     firstNonEmpty(stringValue(payload["trust_class"]), a.TrustClass),
		Source:         firstNonEmpty(stringValue(payload["source"]), a.Source, a.Name()),
	}
	normalizer := a.Normalizer
	if normalizer == nil {
		normalizer = DefaultNormalizer{}
	}
	return normalizer.Normalize(event)
}

// FromFrameworkEvent maps an internal framework event into the canonical rex form.
func FromFrameworkEvent(event core.FrameworkEvent) (CanonicalEvent, error) {
	payload := map[string]any{}
	if len(event.Payload) > 0 {
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return CanonicalEvent{}, err
		}
	}
	return DefaultNormalizer{}.Normalize(CanonicalEvent{
		ID:             fmt.Sprintf("%d", event.Seq),
		Type:           event.Type,
		Timestamp:      event.Timestamp,
		ActorID:        event.Actor.ID,
		Partition:      event.Partition,
		IdempotencyKey: event.IdempotencyKey,
		Payload:        payload,
		TrustClass:     TrustInternal,
		Source:         "framework",
	})
}

// AllowsIngress rejects untrusted ingress for internal-only event types.
func AllowsIngress(event CanonicalEvent) bool {
	switch event.Type {
	case TypeTaskRequested:
		return true
	case TypeWorkflowResume, TypeWorkflowSignal, TypeCallbackReceived:
		return event.TrustClass != TrustUntrusted
	default:
		return event.TrustClass == TrustInternal
	}
}

// ToEnvelope maps a canonical event into rex's normalized intake shape.
func ToEnvelope(event CanonicalEvent) envelope.Envelope {
	payload := event.Payload
	meta := map[string]string{
		"event_type":      event.Type,
		"event_id":        event.ID,
		"event_source":    event.Source,
		"event_partition": event.Partition,
		"event_trust":     event.TrustClass,
		"idempotency_key": event.IdempotencyKey,
	}
	env := envelope.Envelope{
		TaskID:             firstNonEmpty(stringValue(payload["task_id"]), event.ID),
		Instruction:        stringValue(payload["instruction"]),
		Workspace:          stringValue(payload["workspace"]),
		ModeHint:           stringValue(payload["mode_hint"]),
		ResumedRoute:       stringValue(payload["rex.route"]),
		EditPermitted:      boolValue(payload["edit_permitted"]) || boolValue(payload["mutation_allowed"]),
		WorkflowID:         stringValue(payload["workflow_id"]),
		RunID:              stringValue(payload["run_id"]),
		Source:             firstNonEmpty(event.Source, "event"),
		CapabilitySnapshot: stringSlice(payload["capability_snapshot"]),
		Metadata:           meta,
	}
	if env.Instruction == "" {
		env.Instruction = firstNonEmpty(stringValue(payload["summary"]), event.Type)
	}
	if actor := strings.TrimSpace(event.ActorID); actor != "" {
		env.Metadata["actor_id"] = actor
	}
	return env
}

// ToTask maps a canonical event into a rex-executable task while preserving ingress metadata.
func ToTask(event CanonicalEvent) *core.Task {
	env := ToEnvelope(event)
	contextMap := cloneMap(event.Payload)
	if contextMap == nil {
		contextMap = map[string]any{}
	}
	contextMap["workflow_id"] = env.WorkflowID
	contextMap["run_id"] = env.RunID
	contextMap["workspace"] = env.Workspace
	contextMap["mode_hint"] = env.ModeHint
	contextMap["source"] = env.Source
	contextMap["event_type"] = event.Type
	contextMap["event_id"] = event.ID
	contextMap["event_partition"] = event.Partition
	contextMap["event_trust_class"] = event.TrustClass
	contextMap["idempotency_key"] = event.IdempotencyKey
	contextMap["edit_permitted"] = env.EditPermitted
	if env.ResumedRoute != "" {
		contextMap["rex.route"] = env.ResumedRoute
	}
	if len(env.CapabilitySnapshot) > 0 {
		contextMap["capability_snapshot"] = append([]string{}, env.CapabilitySnapshot...)
	}
	return &core.Task{
		ID:          env.TaskID,
		Type:        taskTypeForEvent(event.Type, env.EditPermitted),
		Instruction: env.Instruction,
		Context:     contextMap,
		Metadata:    cloneStringMap(env.Metadata),
	}
}

func taskTypeForEvent(eventType string, editPermitted bool) core.TaskType {
	switch eventType {
	case TypeWorkflowResume:
		return core.TaskTypeAnalysis
	case TypeTaskRequested:
		if editPermitted {
			return core.TaskTypeCodeModification
		}
		return core.TaskTypeAnalysis
	default:
		if editPermitted {
			return core.TaskTypeCodeGeneration
		}
		return core.TaskTypeAnalysis
	}
}

func normalizeTrust(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", TrustUntrusted:
		return TrustUntrusted
	case TrustTrusted:
		return TrustTrusted
	case TrustInternal:
		return TrustInternal
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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

func boolValue(raw any) bool {
	value, ok := raw.(bool)
	return ok && value
}

func stringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := stringValue(item); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func timeValue(raw any) time.Time {
	switch typed := raw.(type) {
	case time.Time:
		return typed.UTC()
	case string:
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(typed))
		if err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}
