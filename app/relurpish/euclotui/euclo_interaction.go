package euclotui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"codeburg.org/lexbit/relurpify/app/relurpish/tui"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
)

const NotifKindInteraction tui.NotificationKind = "interaction"

// PushInteraction pushes an interaction notification with the frame's slots.
func PushInteraction(q *tui.NotificationQueue, frame interaction.InteractionFrame) string {
	id := tui.GenerateID()
	if q != nil {
		q.Push(notificationItemFromFrame(id, tui.NotifKindInteraction, frame, nil))
	}
	return id
}

func notificationItemFromFrame(id string, kind tui.NotificationKind, frame interaction.InteractionFrame, extra map[string]string) tui.NotificationItem {
	itemExtra := serializeFrameSlots(frame)
	for key, value := range extra {
		itemExtra[key] = value
	}
	return tui.NotificationItem{
		ID:    id,
		Kind:  kind,
		Msg:   frameLabel(frame),
		Extra: itemExtra,
	}
}

func serializeFrameSlots(frame interaction.InteractionFrame) map[string]string {
	slots := frame.Slots
	if len(slots) == 0 {
		slots = frame.Actions
	}
	frameType := frame.Type
	if frameType == "" {
		frameType = frame.Kind
	}
	extra := map[string]string{
		"frame_id":   frame.ID,
		"task_id":    frame.TaskID,
		"session_id": frame.SessionID,
		"frame_type": string(frameType),
	}
	for i, slot := range slots {
		for _, prefix := range []string{fmt.Sprintf("slot_%d", i), fmt.Sprintf("action_%d", i)} {
			extra[prefix+"_id"] = slot.ID
			extra[prefix+"_label"] = slot.Label
			extra[prefix+"_action"] = slot.Action
			extra[prefix+"_shortcut"] = slot.Shortcut
			extra[prefix+"_kind"] = slot.Kind
			extra[prefix+"_risk"] = slot.Risk
		}
		if slot.Default {
			extra[fmt.Sprintf("slot_%d_default", i)] = "true"
			extra[fmt.Sprintf("action_%d_default", i)] = "true"
			extra["default_slot"] = slot.ID
			extra["default_action"] = slot.ID
		}
	}
	extra["slot_count"] = fmt.Sprintf("%d", len(slots))
	extra["action_count"] = fmt.Sprintf("%d", len(slots))
	return extra
}

func frameLabel(frame interaction.InteractionFrame) string {
	frameType := frame.Type
	if frameType == "" {
		frameType = frame.Kind
	}
	switch frameType {
	case interaction.FrameScopeConfirmation:
		return "scope confirmation"
	case interaction.FrameIntentClarification:
		return "intent clarification"
	case interaction.FrameCandidateSelection:
		return "candidate selection"
	case interaction.FrameThoughtRecipeSelection:
		return "thoughtrecipe selection"
	case interaction.FrameCapabilitySelection:
		return "capability selection"
	case interaction.FrameHITLApproval:
		return "approval required"
	case interaction.FrameSessionResume:
		return "session resume"
	case interaction.FrameBackgroundJobStatus:
		return "background job status"
	case interaction.FrameExecutionSummary:
		return "execution summary"
	case interaction.FrameVerificationEvidence:
		return "verification evidence"
	case interaction.FrameOutcomeFeedback:
		return "outcome feedback"
	default:
		return string(frameType)
	}
}

func notificationAllowsFreetext(item tui.NotificationItem) bool {
	if item.Kind != tui.NotifKindInteraction || item.Extra == nil {
		return false
	}
	if item.Extra["frame_type"] != string(interaction.FrameIntentClarification) {
		return false
	}
	count, _ := strconv.Atoi(strings.TrimSpace(item.Extra["slot_count"]))
	if count == 0 {
		return true
	}
	if count == 1 && strings.TrimSpace(item.Extra["slot_0_id"]) == "answer" {
		return true
	}
	return false
}

// RenderInteractionNotification renders the notification bar for an
// interaction notification.
func RenderInteractionNotification(item tui.NotificationItem) string {
	label := "● " + item.Msg
	rendered := eucloFrameStyle.Render(label)

	countStr := item.Extra["slot_count"]
	count, _ := strconv.Atoi(countStr)
	if count == 0 {
		return rendered + dimStyle.Render("  [d] dismiss")
	}

	var actions []interaction.ActionSlot
	for i := 0; i < count; i++ {
		prefix := fmt.Sprintf("slot_%d", i)
		actions = append(actions, interaction.ActionSlot{
			ID:      item.Extra[prefix+"_id"],
			Label:   item.Extra[prefix+"_label"],
			Action:  item.Extra[prefix+"_action"],
			Risk:    item.Extra[prefix+"_risk"],
			Default: item.Extra[prefix+"_default"] == "true",
		})
	}
	return rendered + RenderActionSlots(actions) + dimStyle.Render("  [enter] default  [d] dismiss")
}

func RenderActionSlots(actions []interaction.ActionSlot) string {
	if len(actions) == 0 {
		return ""
	}
	var parts []string
	for i, action := range actions {
		label := action.Label
		if label == "" {
			label = action.ID
		}
		if action.Default {
			label = "*" + label
		}
		parts = append(parts, fmt.Sprintf("[%d] %s", i+1, label))
	}
	return " " + strings.Join(parts, " ")
}

// EucloEventRouter fans normalized execution events into the projections used
// by the Euclo surfaces.
type EucloEventRouter struct {
	mu      sync.Mutex
	chat    ChatProjection
	graph   GraphProjection
	diff    DiffProjection
	library LibraryProjection
}

// NewEucloEventRouter creates an empty projection router.
func NewEucloEventRouter() *EucloEventRouter {
	return &EucloEventRouter{
		graph: GraphProjection{Nodes: make(map[string]*GraphNodeProjection)},
		library: LibraryProjection{
			Recipes: make(map[string]int),
			Tags:    make(map[string]int),
		},
	}
}

// ApplyExecutionEvent applies a normalized execution event to all projections.
func (r *EucloEventRouter) ApplyExecutionEvent(ev ExecutionEvent) EucloProjectionSnapshot {
	if r == nil {
		return EucloProjectionSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.applyChatEvent(ev)
	r.applyGraphEvent(ev)
	r.applyDiffEvent(ev)
	r.applyLibraryEvent(ev)
	return r.snapshotLocked()
}

// ApplyInteractionFrame projects an interaction frame into the same event
// stream used by execution telemetry.
func (r *EucloEventRouter) ApplyInteractionFrame(frame interaction.InteractionFrame) EucloProjectionSnapshot {
	ev := ExecutionEvent{
		Header: reporting.EventHeader{
			TaskID:     frame.TaskID,
			SessionID:  frame.SessionID,
			OccurredAt: frame.CreatedAt,
		},
		Type:      reporting.EventTypeFrameEmittedEuclo,
		TaskID:    frame.TaskID,
		SessionID: frame.SessionID,
		NodeID:    frame.ID,
		Summary:   frameLabel(frame),
		Milestone: frameLabel(frame),
		Frame:     &frame,
		Payload:   frame.Payload,
	}
	if frame.Response != nil {
		ev.Output = strings.TrimSpace(frame.Response.ChosenSlot)
	}
	return r.ApplyExecutionEvent(ev)
}

// Snapshot returns a copy of the router projections.
func (r *EucloEventRouter) Snapshot() EucloProjectionSnapshot {
	if r == nil {
		return EucloProjectionSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *EucloEventRouter) snapshotLocked() EucloProjectionSnapshot {
	return EucloProjectionSnapshot{
		Chat:    r.chat.clone(),
		Graph:   r.graph.clone(),
		Diff:    r.diff.clone(),
		Library: r.library.clone(),
	}
}

func (r *EucloEventRouter) applyChatEvent(ev ExecutionEvent) {
	if ev.Milestone != "" {
		r.chat.Milestones = append(r.chat.Milestones, ev.Milestone)
	}
	if ev.Summary != "" && ev.Summary != ev.Milestone {
		r.chat.Milestones = append(r.chat.Milestones, ev.Summary)
	}
	if ev.Output != "" {
		r.chat.Outputs = append(r.chat.Outputs, ev.Output)
	}
	if ev.Frame != nil {
		r.chat.Frames = append(r.chat.Frames, *ev.Frame)
	}
}

func (r *EucloEventRouter) applyGraphEvent(ev ExecutionEvent) {
	nodeID := strings.TrimSpace(ev.NodeID)
	if nodeID == "" {
		nodeID = strings.TrimSpace(ev.StepID)
	}
	if nodeID == "" && ev.Frame != nil {
		nodeID = strings.TrimSpace(ev.Frame.ID)
	}
	if nodeID == "" {
		return
	}
	node := r.graph.Nodes[nodeID]
	if node == nil {
		node = &GraphNodeProjection{ID: nodeID}
		r.graph.Nodes[nodeID] = node
		r.graph.Order = append(r.graph.Order, nodeID)
	}
	if ev.StepID != "" {
		node.StepID = ev.StepID
	}
	if ev.RecipeID != "" {
		node.RecipeID = ev.RecipeID
	}
	if ev.Summary != "" {
		node.Label = ev.Summary
	} else if ev.Milestone != "" {
		node.Label = ev.Milestone
	}
	node.LastEvent = ev.Type
	if len(ev.RouteScores) > 0 {
		node.RouteScores = cloneScores(ev.RouteScores)
	}
	switch {
	case strings.Contains(strings.ToLower(string(ev.Type)), "failed"):
		node.Status = "failed"
	case strings.Contains(strings.ToLower(string(ev.Type)), "complete"):
		node.Status = "completed"
	case strings.Contains(strings.ToLower(string(ev.Type)), "select"):
		node.Status = "running"
	default:
		if node.Status == "" {
			node.Status = "running"
		}
	}
	if ev.Type == reporting.EventTypeRouteSelected && node.RouteScores != nil {
		r.graph.ActiveNode = nodeID
	}
}

func (r *EucloEventRouter) applyDiffEvent(ev ExecutionEvent) {
	if len(ev.PatchHunks) == 0 {
		return
	}
	for _, hunk := range ev.PatchHunks {
		if strings.TrimSpace(hunk.File) == "" {
			continue
		}
		projection := DiffHunkProjection{
			File:         hunk.File,
			Summary:      hunk.Summary,
			Body:         hunk.Body,
			StepID:       firstNonEmpty(hunk.StepID, ev.StepID),
			Origin:       firstNonEmpty(hunk.Origin, string(ev.Type)),
			LinesAdded:   hunk.LinesAdded,
			LinesRemoved: hunk.LinesRemoved,
		}
		if strings.Contains(strings.ToLower(string(ev.Type)), "fail") {
			projection.VerificationFailed = true
		}
		if failure := firstNonEmpty(
			stringPayload(ev.Payload, "verification_log"),
			stringPayload(ev.Payload, "compiler_log"),
			stringPayload(ev.Payload, "verification_output"),
			strings.TrimSpace(ev.Output),
		); failure != "" {
			projection.VerificationFailed = true
			projection.VerificationLog = failure
		}
		if deferred := firstNonEmpty(
			stringPayload(ev.Payload, "deferred_markdown"),
			stringPayload(ev.Payload, "deferred_file"),
			stringPayload(ev.Payload, "deferred_path"),
		); deferred != "" {
			projection.DeferredMarkdownPath = deferred
		}
		r.diff.Hunks = append(r.diff.Hunks, projection)
		step := r.ensureDiffStepProjection(projection.StepID, projection.Origin)
		file := r.ensureDiffFileProjection(step, projection.File)
		file.Hunks = append(file.Hunks, projection)
		if projection.VerificationFailed {
			step.VerificationFailed = true
			if projection.VerificationLog != "" {
				step.VerificationLog = projection.VerificationLog
			}
		}
		if projection.VerificationLog != "" && step.VerificationLog == "" {
			step.VerificationLog = projection.VerificationLog
		}
		if projection.DeferredMarkdownPath != "" {
			step.DeferredMarkdownPath = projection.DeferredMarkdownPath
		}
		if projection.VerificationFailed {
			file.VerificationFailed = true
			if projection.VerificationLog != "" {
				file.VerificationLog = projection.VerificationLog
			}
		}
		if projection.DeferredMarkdownPath != "" {
			file.DeferredMarkdownPath = projection.DeferredMarkdownPath
		}
	}
}

func (r *EucloEventRouter) applyLibraryEvent(ev ExecutionEvent) {
	if ev.RecipeID != "" {
		r.library.Recipes[ev.RecipeID]++
	}
	for _, hunk := range ev.PatchHunks {
		if tag := strings.TrimSpace(hunk.Origin); tag != "" {
			r.library.Tags[tag]++
		}
	}
}

// ChatProjection is the human-sized milestone feed for the chat surface.
type ChatProjection struct {
	Milestones []string
	Outputs    []string
	Frames     []interaction.InteractionFrame
}

func (p ChatProjection) clone() ChatProjection {
	return ChatProjection{
		Milestones: append([]string(nil), p.Milestones...),
		Outputs:    append([]string(nil), p.Outputs...),
		Frames:     append([]interaction.InteractionFrame(nil), p.Frames...),
	}
}

// GraphNodeProjection captures one DAG node state.
type GraphNodeProjection struct {
	ID          string
	RecipeID    string
	StepID      string
	Label       string
	Status      string
	RouteScores map[string]float64
	LastEvent   reporting.EventType
}

// GraphProjection is the execution DAG view.
type GraphProjection struct {
	Nodes      map[string]*GraphNodeProjection
	Order      []string
	ActiveNode string
}

func (p GraphProjection) clone() GraphProjection {
	out := GraphProjection{
		Nodes:      make(map[string]*GraphNodeProjection, len(p.Nodes)),
		Order:      append([]string(nil), p.Order...),
		ActiveNode: p.ActiveNode,
	}
	for id, node := range p.Nodes {
		if node == nil {
			continue
		}
		cloned := *node
		cloned.RouteScores = cloneScores(node.RouteScores)
		out.Nodes[id] = &cloned
	}
	return out
}

// DiffHunkProjection captures causal diff state.
type DiffHunkProjection struct {
	File                 string
	Summary              string
	Body                 string
	StepID               string
	Origin               string
	LinesAdded           int
	LinesRemoved         int
	VerificationFailed   bool
	VerificationLog      string
	DeferredMarkdownPath string
}

// DiffFileProjection groups causal hunks for a single file.
type DiffFileProjection struct {
	File                 string
	Hunks                []DiffHunkProjection
	VerificationFailed   bool
	VerificationLog      string
	DeferredMarkdownPath string
}

func (p DiffFileProjection) clone() DiffFileProjection {
	return DiffFileProjection{
		File:                 p.File,
		Hunks:                append([]DiffHunkProjection(nil), p.Hunks...),
		VerificationFailed:   p.VerificationFailed,
		VerificationLog:      p.VerificationLog,
		DeferredMarkdownPath: p.DeferredMarkdownPath,
	}
}

// DiffStepProjection groups causal file groups by execution step.
type DiffStepProjection struct {
	StepID               string
	Origin               string
	Files                map[string]*DiffFileProjection
	Order                []string
	VerificationFailed   bool
	VerificationLog      string
	DeferredMarkdownPath string
}

func (p DiffStepProjection) clone() DiffStepProjection {
	out := DiffStepProjection{
		StepID:               p.StepID,
		Origin:               p.Origin,
		Files:                make(map[string]*DiffFileProjection, len(p.Files)),
		Order:                append([]string(nil), p.Order...),
		VerificationFailed:   p.VerificationFailed,
		VerificationLog:      p.VerificationLog,
		DeferredMarkdownPath: p.DeferredMarkdownPath,
	}
	for k, v := range p.Files {
		if v == nil {
			continue
		}
		cloned := v.clone()
		out.Files[k] = &cloned
	}
	return out
}

// DiffProjection groups all hunks for review.
type DiffProjection struct {
	Hunks []DiffHunkProjection
	Steps map[string]*DiffStepProjection
	Order []string
}

func (p DiffProjection) clone() DiffProjection {
	out := DiffProjection{
		Hunks: append([]DiffHunkProjection(nil), p.Hunks...),
		Steps: make(map[string]*DiffStepProjection, len(p.Steps)),
		Order: append([]string(nil), p.Order...),
	}
	for k, v := range p.Steps {
		if v == nil {
			continue
		}
		cloned := v.clone()
		out.Steps[k] = &cloned
	}
	return out
}

// LibraryProjection records historical recipe activity.
type LibraryProjection struct {
	Recipes map[string]int
	Tags    map[string]int
}

func (p LibraryProjection) clone() LibraryProjection {
	out := LibraryProjection{
		Recipes: make(map[string]int, len(p.Recipes)),
		Tags:    make(map[string]int, len(p.Tags)),
	}
	for k, v := range p.Recipes {
		out.Recipes[k] = v
	}
	for k, v := range p.Tags {
		out.Tags[k] = v
	}
	return out
}

// EucloProjectionSnapshot is the immutable view of all Euclo surfaces.
type EucloProjectionSnapshot struct {
	Chat    ChatProjection
	Graph   GraphProjection
	Diff    DiffProjection
	Library LibraryProjection
}

func cloneScores(in map[string]float64) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedScoreKeys(scores map[string]float64) []string {
	keys := make([]string, 0, len(scores))
	for k := range scores {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringPayload(payload map[string]any, key string) string {
	if len(payload) == 0 {
		return ""
	}
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func (r *EucloEventRouter) ensureDiffStepProjection(stepID, origin string) *DiffStepProjection {
	if r.diff.Steps == nil {
		r.diff.Steps = make(map[string]*DiffStepProjection)
	}
	stepID = firstNonEmpty(stepID, "unscoped-step")
	step := r.diff.Steps[stepID]
	if step == nil {
		step = &DiffStepProjection{
			StepID: stepID,
			Origin: origin,
			Files:  make(map[string]*DiffFileProjection),
		}
		r.diff.Steps[stepID] = step
		r.diff.Order = append(r.diff.Order, stepID)
	}
	if step.Origin == "" {
		step.Origin = origin
	}
	return step
}

func (r *EucloEventRouter) ensureDiffFileProjection(step *DiffStepProjection, file string) *DiffFileProjection {
	if step == nil {
		return nil
	}
	if step.Files == nil {
		step.Files = make(map[string]*DiffFileProjection)
	}
	file = strings.TrimSpace(file)
	node := step.Files[file]
	if node == nil {
		node = &DiffFileProjection{File: file}
		step.Files[file] = node
		step.Order = append(step.Order, file)
	}
	return node
}
