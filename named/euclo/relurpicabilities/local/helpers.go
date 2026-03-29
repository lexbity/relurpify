package local

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
)

func instructionFromArtifacts(artifacts euclotypes.ArtifactState) string {
	for _, artifact := range artifacts.All() {
		if artifact.Kind != euclotypes.ArtifactKindIntake {
			continue
		}
		if summary := strings.TrimSpace(artifact.Summary); summary != "" {
			return summary
		}
		if payload, ok := artifact.Payload.(map[string]any); ok {
			if instruction := strings.TrimSpace(stringValue(payload["instruction"])); instruction != "" {
				return instruction
			}
		}
	}
	return ""
}

func taskInstruction(task *core.Task) string {
	if task == nil || strings.TrimSpace(task.Instruction) == "" {
		return "the requested change"
	}
	return strings.TrimSpace(task.Instruction)
}

func capTaskInstruction(task *core.Task) string {
	return taskInstruction(task)
}

func taskContextString(task *core.Task, key string) string {
	if task == nil || task.Context == nil {
		return ""
	}
	value, ok := task.Context[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func mergeStateArtifactsToContext(state *core.Context, artifacts []euclotypes.Artifact) {
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

func summarizePayload(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return encodePayload(value)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func resultSummary(result *core.Result) string {
	if result == nil {
		return ""
	}
	if result.Data != nil {
		if summary, ok := result.Data["summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
	}
	if result.Error != nil {
		return result.Error.Error()
	}
	return "task completed"
}

func errMsg(err error, result *core.Result) string {
	if err != nil {
		return err.Error()
	}
	if result != nil && result.Error != nil {
		return result.Error.Error()
	}
	return "unknown error"
}

func uniqueStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func encodePayload(value any) string {
	data, err := json.Marshal(value)
	if err == nil {
		return string(data)
	}
	return fmt.Sprint(value)
}
