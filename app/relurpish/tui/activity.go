package tui

import (
	"fmt"
	"strconv"
	"strings"

	"codeburg.org/lexbit/relurpify/model"
	"codeburg.org/lexbit/relurpify/telemetry"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	activityLimit     = 512
	activityAll       = "all"
	activityCalls     = "calls"
	activitySystem    = "system"
	activityMetaKind  = "kind"
	activityMetaModel = "model"
)

type activityEntry struct {
	group string
	entry InspectionEntry
}

// activityCanvas is a bounded projection of the events already emitted to the
// OTel chain. Its local row identities are navigation anchors, never journal
// cursors. No payloads, prompts, tool arguments, or output previews are retained.
type activityCanvas struct {
	rows      []activityEntry
	head      int
	next      uint64
	evicted   uint64
	filter    string
	inspector documentInspector
	demo      bool
}

func activityGroup(kind telemetry.EventType) string {
	name := string(kind)
	switch {
	case strings.HasPrefix(name, "llm_"), strings.HasPrefix(name, "model."), strings.HasPrefix(name, "inference_"):
		return activityCalls
	case strings.HasPrefix(name, "prompt."):
		return "prompts"
	case strings.HasPrefix(name, "compiler."), strings.HasPrefix(name, "chunk."), strings.HasPrefix(name, "knowledge."), strings.HasPrefix(name, "contextstream."), strings.HasPrefix(name, "capture."), strings.HasPrefix(name, "summary."):
		return "knowledge"
	case strings.HasPrefix(name, "graph_"), strings.HasPrefix(name, "graph."), strings.HasPrefix(name, "node_"), strings.HasPrefix(name, "euclo."), strings.HasPrefix(name, "tool_"), strings.HasPrefix(name, "capability_"):
		return "graph"
	default:
		return activitySystem
	}
}

func (p *activityCanvas) apply(events []telemetry.Event) {
	for _, event := range events {
		p.next++
		row := activityEntry{group: activityGroup(event.Type), entry: activityInspection(event, p.next, p.demo)}
		if len(p.rows) == activityLimit {
			p.rows[p.head] = row
			p.head = (p.head + 1) % activityLimit
			p.evicted++
		} else {
			p.rows = append(p.rows, row)
		}
	}
	p.rebuild()
}

func (p *activityCanvas) rebuild() {
	filter := p.filter
	if filter == "" {
		filter = activityAll
	}
	document := InspectionDocument{Title: "Activity / " + filter, Notice: "OTel pipeline observations · latest 512 retained locally · best effort; not a durable journal. Raw content is omitted."}
	if p.demo {
		document.Notice = "DEMO: simulated telemetry observations; no model, tool or compiler ran. Raw content is omitted."
	}
	if p.evicted > 0 {
		document.Notice += fmt.Sprintf(" %d older local rows evicted.", p.evicted)
	}
	for i := range len(p.rows) {
		row := p.rows[(p.head+i)%len(p.rows)]
		if filter == activityAll || row.group == filter {
			document.Entries = append(document.Entries, row.entry)
		}
	}
	// Preserve an open detail even when the ring evicts its row. At most one
	// detached row is pinned; subsequent activity cannot move the reader.
	if p.inspector.detail && p.inspector.index() >= 0 {
		pinned := p.inspector.document.Entries[p.inspector.index()]
		found := false
		for _, entry := range document.Entries {
			if entry.ID == pinned.ID {
				found = true
				break
			}
		}
		if !found {
			pinned.Summary = "Retained inspection; this row has left the local activity window."
			document.Entries = append(document.Entries, pinned)
		}
	}
	p.inspector.apply(document)
}

func activityInspection(event telemetry.Event, local uint64, demo bool) InspectionEntry {
	kind := boundedActivityText(string(event.Type))
	entry := InspectionEntry{ID: strconv.FormatUint(local, 10), Label: fmt.Sprintf("%s  %s", event.Timestamp.Format("15:04:05.000"), kind), Origin: "Live observation from the existing OTel telemetry pipeline", Summary: "Observed event only; no missing transitions or provenance are inferred."}
	if demo {
		entry.Origin = "DEMO: simulated telemetry fixture"
	}
	add := func(name, value string) {
		if value != "" {
			entry.Fields = append(entry.Fields, InspectionField{Name: name, Value: boundedActivityText(value)})
		}
	}
	add("Event", kind)
	add("Time", event.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"))
	add("Run", event.RunID)
	add("Task", event.TaskID)
	add("Node", event.NodeID)
	add("Trace", event.TraceID)
	add("Span", event.SpanID)
	add("Session", event.SessionID)
	if event.Seq != 0 {
		add("Producer sequence", strconv.FormatUint(event.Seq, 10))
	}
	// Strict metadata allowlist. Unknown/debug metadata and Event.Message can
	// contain secrets or arbitrary object graphs and MUST NOT enter the canvas.
	for _, key := range []string{activityMetaKind, activityMetaModel, "call_id", "attempt_id", "step_id", "recipe_id", "paradigm", "capability_id", "tool_name", "finish_reason", "message_count", "prompt_chars", "tool_count", "prompt_id", "prompt_revision", "source_version", "blocks_included", "blocks_excluded", "output_length", "cache_hit", "duration_ms", "chunk_id", "content_hash", "token_estimate", "input_tokens", "output_tokens", "total_tokens", "indexed_files", "failure_count", "success"} {
		if value, ok := activityScalar(event.Metadata[key]); ok {
			add(key, value)
		}
	}
	if usage, ok := event.Metadata["usage"].(model.TokenUsage); ok {
		switch {
		case usage.PromptTokens < 0 || usage.CompletionTokens < 0 || usage.TotalTokens < 0:
			add("Token usage", "Invalid negative counters; usage is not established.")
		case !usage.Estimated && usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0:
			add("Token usage", "Not established by this event (zero-valued counters).")
		default:
			source := "Reported counters"
			if usage.Estimated {
				source = "Estimated counters"
				if usage.EstimationMethod != "" {
					source += " / " + usage.EstimationMethod
				}
			}
			add("Token usage", source)
			add("Input tokens", strconv.Itoa(usage.PromptTokens))
			add("Output tokens", strconv.Itoa(usage.CompletionTokens))
			add("Total tokens", strconv.Itoa(usage.TotalTokens))
		}
	}
	if event.Metadata["error"] != nil {
		add("Failure", "An error was reported. Raw error payload is omitted from this view.")
	}
	if activityGroup(event.Type) == activityCalls {
		add("Scope", "Logical model instrumentation; physical-attempt identity is unavailable unless explicitly recorded.")
		if event.Metadata["finish_reason"] == "stream" {
			add("Stream", "Stream opened; this event does not establish stream completion.")
		}
		if event.Metadata["finish_reason"] == "stream_closed" {
			add("Stream", "Token source closed; protocol completion and usage are not established by this transport.")
		}
		add("Prompt evidence", "Actual provider request bytes, bindings, and prompt revisions are not captured by this event contract.")
	}
	return entry
}

func boundedActivityText(value string) string {
	const maximum = 512
	// Stop at a UTF-8 boundary without copying or decoding an unbounded payload.
	for i := range value {
		if i >= maximum {
			value = value[:i]
			break
		}
	}
	return terminalText(strings.Clone(value))
}

func activityScalar(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		return boundedActivityText(value), true
	case bool:
		return strconv.FormatBool(value), true
	case int:
		return strconv.Itoa(value), true
	case int64:
		return strconv.FormatInt(value, 10), true
	case uint64:
		return strconv.FormatUint(value, 10), true
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64), true
	default:
		return "", false
	}
}

func rootHandleActivity(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	filter := activityAll
	if len(args) > 0 {
		filter = args[0]
	}
	switch filter {
	case activityAll, activityCalls, "graph", "knowledge", "prompts", activitySystem:
	default:
		m.addSystemMessage("Usage: /activity [all|calls|graph|knowledge|prompts|system]")
		return m, nil
	}
	if len(args) > 1 {
		m.addSystemMessage("Usage: /activity [all|calls|graph|knowledge|prompts|system]")
		return m, nil
	}
	if filter != m.activity.filter {
		m.activity.inspector = documentInspector{}
	}
	m.activity.filter = filter
	m.activity.rebuild()
	m.setActiveTab(TabActivity)
	m.setFocus(FocusRegionRegion1)
	return m, nil
}
