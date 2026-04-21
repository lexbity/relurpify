package interaction

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
)

type RecordedArtifact struct {
	Kind    string          `json:"kind"`
	Summary string          `json:"summary,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type InteractionRecord struct {
	Kind              string             `json:"kind"`
	Mode              string             `json:"mode"`
	Phase             string             `json:"phase"`
	Content           json.RawMessage    `json:"content"`
	Actions           json.RawMessage    `json:"actions"`
	Response          json.RawMessage    `json:"response,omitempty"`
	Timestamp         string             `json:"timestamp"`
	Duration          string             `json:"duration,omitempty"`
	ArtifactsProduced []string           `json:"artifacts_produced,omitempty"`
	ArtifactsConsumed []string           `json:"artifacts_consumed,omitempty"`
	ProducedArtifacts []RecordedArtifact `json:"produced_artifacts,omitempty"`
}

// InteractionEvent is a single recorded event in an interaction session.
type InteractionEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	Type      string            `json:"type"` // frame, response, transition, phase_skip
	Frame     *InteractionFrame `json:"frame,omitempty"`
	Response  *UserResponse     `json:"response,omitempty"`
	Phase     string            `json:"phase,omitempty"`
	Mode      string            `json:"mode,omitempty"`
	Detail    string            `json:"detail,omitempty"`
}

// InteractionRecording captures the full sequence of interaction events
// during an interactive session. It can be persisted for replay.
type InteractionRecording struct {
	mu     sync.Mutex
	events []InteractionEvent
	full   []InteractionRecord
}

// NewInteractionRecording creates an empty recording.
func NewInteractionRecording() *InteractionRecording {
	return &InteractionRecording{}
}

// RecordFrame adds a frame emission event.
func (r *InteractionRecording) RecordFrame(frame InteractionFrame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.events = append(r.events, InteractionEvent{
		Timestamp: now,
		Type:      "frame",
		Frame:     &frame,
		Phase:     frame.Phase,
		Mode:      frame.Mode,
	})
	r.full = append(r.full, InteractionRecord{
		Kind:      string(frame.Kind),
		Mode:      frame.Mode,
		Phase:     frame.Phase,
		Content:   mustJSON(frame.Content),
		Actions:   mustJSON(frame.Actions),
		Timestamp: now.Format(time.RFC3339Nano),
	})
}

// RecordResponse adds a user response event.
func (r *InteractionRecording) RecordResponse(resp UserResponse, phase, mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	r.events = append(r.events, InteractionEvent{
		Timestamp: now,
		Type:      "response",
		Response:  &resp,
		Phase:     phase,
		Mode:      mode,
	})
	for i := len(r.full) - 1; i >= 0; i-- {
		record := &r.full[i]
		if record.Phase != phase || record.Mode != mode || len(record.Response) != 0 {
			continue
		}
		record.Response = mustJSON(resp)
		record.Duration = durationSince(record.Timestamp, now)
		break
	}
}

// RecordPhaseSkip records a phase that was auto-skipped.
func (r *InteractionRecording) RecordPhaseSkip(phase, mode, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, InteractionEvent{
		Timestamp: time.Now(),
		Type:      "phase_skip",
		Phase:     phase,
		Mode:      mode,
		Detail:    reason,
	})
}

// RecordTransition records a mode transition event.
func (r *InteractionRecording) RecordTransition(fromMode, toMode, trigger string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, InteractionEvent{
		Timestamp: time.Now(),
		Type:      "transition",
		Mode:      fromMode,
		Detail:    toMode + ":" + trigger,
	})
}

func (r *InteractionRecording) RecordPhaseArtifacts(phase, mode string, produced []euclotypes.Artifact, consumed []euclotypes.ArtifactKind) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record := r.ensurePhaseRecordLocked(phase, mode)
	record.ArtifactsConsumed = artifactKindStrings(consumed)
	record.ArtifactsProduced = producedArtifactKindStrings(produced)
	record.ProducedArtifacts = recordedArtifacts(produced)
}

// Events returns all recorded events.
func (r *InteractionRecording) Events() []InteractionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]InteractionEvent, len(r.events))
	copy(out, r.events)
	return out
}

func (r *InteractionRecording) Records() []InteractionRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]InteractionRecord, len(r.full))
	copy(out, r.full)
	return out
}

// FrameEvents returns only frame emission events.
func (r *InteractionRecording) FrameEvents() []InteractionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []InteractionEvent
	for _, e := range r.events {
		if e.Type == "frame" {
			out = append(out, e)
		}
	}
	return out
}

// TransitionEvents returns only transition events.
func (r *InteractionRecording) TransitionEvents() []InteractionEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []InteractionEvent
	for _, e := range r.events {
		if e.Type == "transition" {
			out = append(out, e)
		}
	}
	return out
}

// MarshalJSON serializes the recording to JSON.
func (r *InteractionRecording) MarshalJSON() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return json.Marshal(struct {
		Events  []InteractionEvent  `json:"events"`
		Records []InteractionRecord `json:"records"`
	}{Events: r.events, Records: r.full})
}

// ToStateMap returns a map suitable for persisting in core.Context state.
func (r *InteractionRecording) ToStateMap() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()

	frames := make([]map[string]any, 0)
	transitions := make([]map[string]any, 0)

	for _, e := range r.events {
		switch e.Type {
		case "frame":
			if e.Frame != nil {
				frames = append(frames, map[string]any{
					"kind":  string(e.Frame.Kind),
					"mode":  e.Frame.Mode,
					"phase": e.Frame.Phase,
				})
			}
		case "transition":
			transitions = append(transitions, map[string]any{
				"mode":   e.Mode,
				"detail": e.Detail,
			})
		}
	}

	records := make([]map[string]any, 0, len(r.full))
	for _, record := range r.full {
		entry := map[string]any{
			"kind":  record.Kind,
			"mode":  record.Mode,
			"phase": record.Phase,
		}
		if len(record.ArtifactsProduced) > 0 {
			entry["artifacts_produced"] = append([]string{}, record.ArtifactsProduced...)
		}
		if len(record.ArtifactsConsumed) > 0 {
			entry["artifacts_consumed"] = append([]string{}, record.ArtifactsConsumed...)
		}
		records = append(records, entry)
	}

	return map[string]any{
		"frames":      frames,
		"transitions": transitions,
		"records":     records,
		"event_count": len(r.events),
	}
}

func (r *InteractionRecording) ToJSONLines() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []byte
	for _, record := range r.full {
		line, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out, nil
}

// RecordingEmitter wraps a FrameEmitter and records all interactions.
type RecordingEmitter struct {
	Inner     FrameEmitter
	Recording *InteractionRecording
}

// NewRecordingEmitter creates an emitter that records all frames and responses.
func NewRecordingEmitter(inner FrameEmitter) *RecordingEmitter {
	return &RecordingEmitter{
		Inner:     inner,
		Recording: NewInteractionRecording(),
	}
}

// Emit delegates to the inner emitter and records the frame.
func (e *RecordingEmitter) Emit(ctx context.Context, frame InteractionFrame) error {
	e.Recording.RecordFrame(frame)
	return e.Inner.Emit(ctx, frame)
}

// AwaitResponse delegates to the inner emitter and records the response.
func (e *RecordingEmitter) AwaitResponse(ctx context.Context) (UserResponse, error) {
	resp, err := e.Inner.AwaitResponse(ctx)
	if err == nil {
		// Determine phase from last recorded frame.
		events := e.Recording.FrameEvents()
		phase, mode := "", ""
		if len(events) > 0 {
			last := events[len(events)-1]
			phase = last.Phase
			mode = last.Mode
		}
		e.Recording.RecordResponse(resp, phase, mode)
	}
	return resp, err
}

func (r *InteractionRecording) ensurePhaseRecordLocked(phase, mode string) *InteractionRecord {
	for i := len(r.full) - 1; i >= 0; i-- {
		record := &r.full[i]
		if record.Phase == phase && record.Mode == mode {
			return record
		}
	}
	now := time.Now().UTC()
	r.full = append(r.full, InteractionRecord{
		Kind:      "phase",
		Mode:      mode,
		Phase:     phase,
		Timestamp: now.Format(time.RFC3339Nano),
	})
	return &r.full[len(r.full)-1]
}

func artifactKindStrings(kinds []euclotypes.ArtifactKind) []string {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]string, 0, len(kinds))
	seen := map[string]struct{}{}
	for _, kind := range kinds {
		value := string(kind)
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

func producedArtifactKindStrings(artifacts []euclotypes.Artifact) []string {
	if len(artifacts) == 0 {
		return nil
	}
	out := make([]string, 0, len(artifacts))
	seen := map[string]struct{}{}
	for _, artifact := range artifacts {
		value := string(artifact.Kind)
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

func recordedArtifacts(artifacts []euclotypes.Artifact) []RecordedArtifact {
	if len(artifacts) == 0 {
		return nil
	}
	out := make([]RecordedArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, RecordedArtifact{
			Kind:    string(artifact.Kind),
			Summary: artifact.Summary,
			Payload: mustJSON(artifact.Payload),
		})
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
}

func durationSince(raw string, now time.Time) string {
	if raw == "" {
		return ""
	}
	start, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return ""
	}
	return now.Sub(start).String()
}
