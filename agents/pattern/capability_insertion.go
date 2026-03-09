package pattern

import (
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
)

func resolveInsertionDecision(agent *ReActAgent, task *core.Task, envelope *core.CapabilityResultEnvelope) core.InsertionDecision {
	if agent == nil {
		return core.EffectiveInsertionDecision(nil, envelope)
	}
	return core.EffectiveInsertionDecision(agent.effectiveAgentSpec(task), envelope)
}

func renderInsertionFilteredSummary(agent *ReActAgent, task *core.Task, toolName string, payload *core.ToolResult, envelope *core.CapabilityResultEnvelope) (string, bool) {
	if payload == nil {
		return "", false
	}
	decision := resolveInsertionDecision(agent, task, envelope)
	switch decision.Action {
	case core.InsertionActionDirect, core.InsertionActionSummarized:
		if summary, ok := visibleBlockSummary(envelope); ok {
			return summary, true
		}
		return summarizeToolPayload(payload), true
	case core.InsertionActionMetadataOnly:
		return metadataOnlyInsertionText(toolName, envelope), true
	case core.InsertionActionHITLRequired:
		return fmt.Sprintf("output withheld pending approval for capability %s", capabilityDisplayName(toolName, envelope)), true
	case core.InsertionActionDenied:
		return "", false
	default:
		return "", false
	}
}

func visibleBlockSummary(envelope *core.CapabilityResultEnvelope) (string, bool) {
	if envelope == nil || len(envelope.ContentBlocks) == 0 || len(envelope.BlockInsertions) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(envelope.ContentBlocks))
	for i, block := range envelope.ContentBlocks {
		if i >= len(envelope.BlockInsertions) {
			break
		}
		switch envelope.BlockInsertions[i].Decision.Action {
		case core.InsertionActionDirect, core.InsertionActionSummarized:
		default:
			continue
		}
		switch typed := block.(type) {
		case core.TextContentBlock:
			text := truncateForPrompt(strings.TrimSpace(typed.Text), 220)
			if text != "" {
				parts = append(parts, text)
			}
		case core.StructuredContentBlock:
			text := truncateForPrompt(fmt.Sprint(typed.Data), 220)
			if text != "" && text != "<nil>" {
				parts = append(parts, text)
			}
		case core.ErrorContentBlock:
			text := truncateForPrompt(strings.TrimSpace(typed.Message), 220)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

func metadataOnlyInsertionText(toolName string, envelope *core.CapabilityResultEnvelope) string {
	name := capabilityDisplayName(toolName, envelope)
	if envelope == nil {
		return fmt.Sprintf("metadata-only result retained for %s", name)
	}
	trust := strings.TrimSpace(string(envelope.Descriptor.TrustClass))
	provider := strings.TrimSpace(envelope.Descriptor.Source.ProviderID)
	var attrs []string
	if trust != "" {
		attrs = append(attrs, "trust="+trust)
	}
	if provider != "" {
		attrs = append(attrs, "provider="+provider)
	}
	if len(attrs) == 0 {
		return fmt.Sprintf("metadata-only result retained for %s", name)
	}
	return fmt.Sprintf("metadata-only result retained for %s (%s)", name, strings.Join(attrs, ", "))
}

func capabilityDisplayName(toolName string, envelope *core.CapabilityResultEnvelope) string {
	if envelope != nil {
		if name := strings.TrimSpace(envelope.Descriptor.Name); name != "" {
			return name
		}
		if id := strings.TrimSpace(envelope.Descriptor.ID); id != "" {
			return id
		}
	}
	if strings.TrimSpace(toolName) != "" {
		return strings.TrimSpace(toolName)
	}
	return "capability"
}
