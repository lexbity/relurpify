package debug

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func instructionFromArtifacts(artifacts euclotypes.ArtifactState) string {
	for _, artifact := range artifacts.OfKind(euclotypes.ArtifactKindIntake) {
		if instruction := extractInstruction(artifact.Payload); instruction != "" {
			return instruction
		}
	}
	return ""
}

func extractInstruction(payload any) string {
	switch typed := payload.(type) {
	case map[string]any:
		if value, ok := typed["instruction"].(string); ok {
			return strings.TrimSpace(value)
		}
	case string:
		return strings.TrimSpace(typed)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return strings.TrimSpace(fmt.Sprint(payload))
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err == nil {
		if value, ok := decoded["instruction"].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return strings.TrimSpace(string(data))
}

func taskInstruction(task *core.Task) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.Instruction)
}

func mergeArtifacts(state *core.Context, artifacts []euclotypes.Artifact) {
	if state == nil || len(artifacts) == 0 {
		return
	}
	existing := euclotypes.ArtifactStateFromContext(state).All()
	merged := append(existing, artifacts...)
	state.Set("euclo.artifacts", merged)
	for _, artifact := range artifacts {
		if key := euclotypes.StateKeyForArtifactKind(artifact.Kind); key != "" && artifact.Payload != nil {
			state.Set(key, artifact.Payload)
		}
	}
}

func summarize(value any) string {
	data, err := json.Marshal(value)
	if err == nil {
		return string(data)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func defaultValue(value any, fallback any) any {
	if value == nil {
		return fallback
	}
	return value
}

func decodeMaybeJSON(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err == nil {
		return payload
	}
	return raw
}
