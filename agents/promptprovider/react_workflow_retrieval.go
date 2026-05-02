package promptprovider

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/prompt"
)

type reactWorkflowRetrievalProvider struct{}

func (reactWorkflowRetrievalProvider) Provide(ctx prompt.RuntimeContext) prompt.ContextChunk {
	if ctx.Task == nil || ctx.Task.Context == nil {
		return prompt.ContextChunk{}
	}
	raw, ok := ctx.Task.Context["workflow_retrieval"]
	if !ok || raw == nil {
		return prompt.ContextChunk{}
	}
	// Try byte payload first.
	if payload := extractTaskBytes(ctx.Task, "workflow_retrieval"); len(payload) > 0 {
		var data map[string]any
		if err := json.Unmarshal(payload, &data); err == nil {
			if s := formatWorkflowRetrieval(data); s != "" {
				return prompt.ContextChunk{Content: s}
			}
		}
	}
	// Fallback to direct marshal.
	encoded, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return prompt.ContextChunk{Content: strings.TrimSpace(fmt.Sprint(raw))}
	}
	return prompt.ContextChunk{Content: string(encoded)}
}

func (reactWorkflowRetrievalProvider) Describe() prompt.ProviderMetadata {
	return prompt.ProviderMetadata{
		Name:        "react.workflow_retrieval",
		Description: "Supplies workflow retrieval evidence from task.Context[\"workflow_retrieval\"].",
		Paradigms:   []string{"react"},
	}
}

func formatWorkflowRetrieval(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}
	var sections []string
	if q := trimField(payload, "query"); q != "" {
		sections = append(sections, "Query: "+q)
	}
	if s := trimField(payload, "scope"); s != "" {
		sections = append(sections, "Scope: "+s)
	}
	if ct := trimField(payload, "cache_tier"); ct != "" {
		sections = append(sections, "Cache tier: "+ct)
	}
	results := toSliceOfAny(payload["results"])
	var lines []string
	for i, result := range results {
		m, ok := result.(map[string]any)
		if !ok {
			continue
		}
		text := trimField(m, "text")
		if text == "" {
			text = trimField(m, "summary")
		}
		if text == "" {
			text = "reference only"
		}
		line := fmt.Sprintf("%d. %s", i+1, truncate(text, 240))
		if ref := workflowRef(m); ref != "" {
			line += "\n   Reference: " + ref
		}
		if anchors := toSliceOfAny(m["anchors"]); len(anchors) > 0 {
			if notices := anchorNotices(anchors); notices != "" {
				line += "\n" + notices
			}
		}
		if deriv, ok := m["derivation"].(map[string]any); ok {
			if w := derivationWarning(deriv); w != "" {
				line += "\n" + w
			}
		}
		lines = append(lines, line)
	}
	if len(lines) > 0 {
		sections = append(sections, "Evidence:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n")
}

func workflowRef(result map[string]any) string {
	ref, ok := result["reference"].(map[string]any)
	if !ok || len(ref) == 0 {
		return ""
	}
	for _, k := range []string{"uri", "id", "detail"} {
		if v := trimField(ref, k); v != "" {
			return v
		}
	}
	return ""
}

func anchorNotices(anchors []any) string {
	var notices []string
	for _, a := range anchors {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		status := fmt.Sprint(m["status"])
		term := fmt.Sprint(m["term"])
		def := fmt.Sprint(m["definition"])
		switch status {
		case "drifted":
			notices = append(notices, fmt.Sprintf("   ⚠ ANCHOR DRIFT: %q was %q when captured; context has changed.", term, def))
		case "superseded":
			notices = append(notices, fmt.Sprintf("   ⚠ ANCHOR SUPERSEDED: %q is no longer the active definition.", term))
		}
	}
	return strings.Join(notices, "\n")
}

func derivationWarning(d map[string]any) string {
	depth := toAnyInt(d["depth"])
	loss := toAnyFloat(d["total_loss"])
	if depth <= 4 && loss <= 0.5 {
		return ""
	}
	origin := fmt.Sprint(d["origin_system"])
	return fmt.Sprintf("   ⚠ CONFIDENCE: %d transformations, ~%d%% information loss. Origin: %s", depth, int(loss*100), origin)
}

func trimField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func toAnyInt(v any) int {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

func toAnyFloat(v any) float64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

// extractTaskBytes extracts a []byte value from task context by key.
func extractTaskBytes(task *core.Task, key string) []byte {
	if task == nil || task.Context == nil {
		return nil
	}
	raw, ok := task.Context[key]
	if !ok || raw == nil {
		return nil
	}
	if b, ok := raw.([]byte); ok {
		return b
	}
	return nil
}
