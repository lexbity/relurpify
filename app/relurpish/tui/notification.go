package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	tea "github.com/charmbracelet/bubbletea"
)

// Notification tea messages emitted by NotificationBar.
type NotifHITLApproveMsg struct {
	ID     string
	Scope  fauthorization.GrantScope // OneTime, Session, or Persistent (always)
	Action string                    // raw HITL action, e.g. "tool:cli_mkdir"
}
type NotifHITLDenyMsg struct{ ID string }
type NotifDismissMsg struct{ ID string }
type NotifReviewTaskMsg struct{ ID string }
type NotifReviewDeferredMsg struct{}
type NotifRunTestsMsg struct{}
type NotifRestoreSessionMsg struct{ ID string }

// NotificationQueue is a goroutine-safe FIFO queue of NotificationItems.
type NotificationQueue struct {
	mu    sync.Mutex
	items []NotificationItem
}

// Push adds a notification. A unique ID is generated if missing.
func (q *NotificationQueue) Push(n NotificationItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if n.ID == "" {
		n.ID = generateID()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now()
	}
	q.items = append(q.items, n)
}

// PushHITL is a convenience helper to push a HITL notification.
func (q *NotificationQueue) PushHITL(req *fauthorization.PermissionRequest) {
	if req == nil {
		return
	}
	kind := approvalKindFromRequest(req)
	msg := fmt.Sprintf("%s approval: %s", kind, req.Permission.Action)
	if target := hitlTarget(req); target != "" {
		msg += " -> " + target
	}
	q.Push(NotificationItem{
		ID:   req.ID,
		Kind: NotifKindHITL,
		Msg:  msg,
		Extra: map[string]string{
			"request_id": req.ID,
			"action":     req.Permission.Action,
			"kind":       kind,
			"resource":   req.Permission.Resource,
			"risk":       string(req.Risk),
			"target":     hitlTarget(req),
			"reason":     req.Justification,
		},
	})
}

// Resolve removes a notification by ID.
func (q *NotificationQueue) Resolve(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, item := range q.items {
		if item.ID == id {
			q.items = append(q.items[:i], q.items[i+1:]...)
			return
		}
	}
}

// Current returns the head of the queue without removing it.
func (q *NotificationQueue) Current() (NotificationItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return NotificationItem{}, false
	}
	return q.items[0], true
}

// Len returns the number of queued notifications.
func (q *NotificationQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// NotificationBar renders the current notification as a one-line banner and
// handles key presses for HITL approve/deny/dismiss.
type NotificationBar struct {
	queue               *NotificationQueue
	width               int
	interactionRenderer func(NotificationItem) string
}

// NewNotificationBar creates a NotificationBar backed by the given queue.
func NewNotificationBar(q *NotificationQueue) *NotificationBar {
	return &NotificationBar{queue: q, interactionRenderer: renderGenericNotification}
}

// SetInteractionRenderer updates the renderer used for surface-specific
// interaction notifications.
func (nb *NotificationBar) SetInteractionRenderer(fn func(NotificationItem) string) {
	if nb == nil || fn == nil {
		return
	}
	nb.interactionRenderer = fn
}

// SetWidth updates the bar width (called on every WindowSizeMsg).
func (nb *NotificationBar) SetWidth(w int) {
	nb.width = w
}

// Active returns true when there is at least one notification to show.
func (nb *NotificationBar) Active() bool {
	return nb != nil && nb.queue != nil && nb.queue.Len() > 0
}

// Update handles key events relevant to the current notification.
func (nb *NotificationBar) Update(msg tea.Msg) (*NotificationBar, tea.Cmd) {
	if nb.queue == nil {
		return nb, nil
	}
	current, ok := nb.queue.Current()
	if !ok {
		return nb, nil
	}
	kMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return nb, nil
	}
	// Interaction notifications resolve numbered slots directly. Freetext
	// clarification frames open the guidance panel and resolve there.
	if current.Kind == NotifKindInteraction {
		switch kMsg.String() {
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			slotID, ok := interactionSlotID(current, kMsg.String())
			if !ok {
				return nb, nil
			}
			id := current.ID
			nb.queue.Resolve(id)
			return nb, func() tea.Msg {
				return NotifInteractionResolveMsg{
					NotificationID: id,
					FrameID:        current.Extra["frame_id"],
					TaskID:         current.Extra["task_id"],
					ChoiceID:       slotID,
				}
			}
		case "enter":
			slotID, ok := interactionDefaultSlotID(current)
			if !ok {
				return nb, nil
			}
			id := current.ID
			nb.queue.Resolve(id)
			return nb, func() tea.Msg {
				return NotifInteractionResolveMsg{
					NotificationID: id,
					FrameID:        current.Extra["frame_id"],
					TaskID:         current.Extra["task_id"],
					ChoiceID:       slotID,
				}
			}
		case "d", "esc":
			id := current.ID
			nb.queue.Resolve(id)
			return nb, func() tea.Msg { return NotifDismissMsg{ID: id} }
		default:
			return nb, nil
		}
	}
	switch kMsg.String() {
	case "y", "Y":
		if current.Kind == NotifKindHITL {
			id, action := current.ID, current.Extra["action"]
			return nb, func() tea.Msg {
				return NotifHITLApproveMsg{ID: id, Scope: fauthorization.GrantScopeOneTime, Action: action}
			}
		}
	case "s", "S":
		if current.Kind == NotifKindHITL {
			id, action := current.ID, current.Extra["action"]
			return nb, func() tea.Msg {
				return NotifHITLApproveMsg{ID: id, Scope: fauthorization.GrantScopeSession, Action: action}
			}
		}
	case "a", "A":
		if current.Kind == NotifKindHITL {
			id, action := current.ID, current.Extra["action"]
			return nb, func() tea.Msg {
				return NotifHITLApproveMsg{ID: id, Scope: fauthorization.GrantScopePersistent, Action: action}
			}
		}
	case "n", "N":
		if current.Kind == NotifKindHITL {
			id := current.ID
			return nb, func() tea.Msg { return NotifHITLDenyMsg{ID: id} }
		}
	case "enter":
		if current.Kind == NotifKindRestore {
			id := current.ID
			return nb, func() tea.Msg { return NotifRestoreSessionMsg{ID: id} }
		}
		if current.Kind == NotifKindDeferred {
			id := current.ID
			nb.queue.Resolve(id)
			return nb, func() tea.Msg { return NotifReviewDeferredMsg{} }
		}
		if current.Kind == NotifKindTaskDone {
			id := current.ID
			return nb, func() tea.Msg { return NotifReviewTaskMsg{ID: id} }
		}
	case "d", "esc":
		id := current.ID
		nb.queue.Resolve(id)
		return nb, func() tea.Msg { return NotifDismissMsg{ID: id} }
	}
	return nb, nil
}

// View renders the notification bar. Returns empty string when no notification is active.
func (nb *NotificationBar) View() string {
	if nb == nil || nb.queue == nil {
		return ""
	}
	current, ok := nb.queue.Current()
	if !ok {
		return ""
	}
	hint := ""
	switch current.Kind {
	case NotifKindHITL:
		hint = dimStyle.Render("  [y] once  [s] session  [a] always  [n] deny  [d] dismiss")
	case NotifKindRestore:
		hint = dimStyle.Render("  [enter] restore  [d] dismiss")
	case NotifKindDeferred:
		hint = dimStyle.Render("  [enter] review  [d] dismiss")
	case NotifKindInteraction, NotifKindGuidance:
		rendered := nb.interactionRenderer(current)
		if nb.queue.Len() > 1 {
			rendered += dimStyle.Render(fmt.Sprintf("  (+%d more)", nb.queue.Len()-1))
		}
		return rendered
	default:
		hint = dimStyle.Render("  [d] dismiss")
	}
	more := ""
	if nb.queue.Len() > 1 {
		more = dimStyle.Render(fmt.Sprintf("  (+%d more)", nb.queue.Len()-1))
	}
	label := "● " + current.Msg
	if current.Kind == NotifKindHITL {
		label = "● " + renderApprovalNotification(current)
	}
	var rendered string
	switch current.Kind {
	case NotifKindHITL:
		rendered = notifHITLStyle.Render(label)
	case NotifKindError:
		rendered = notifErrorStyle.Render(label)
	case NotifKindTaskDone:
		rendered = notifSuccessStyle.Render(label)
	case NotifKindDeferred:
		rendered = notifInfoStyle.Render(label)
	default:
		rendered = notifInfoStyle.Render(label)
	}
	return rendered + hint + more
}

func renderGenericNotification(item NotificationItem) string {
	label := "● " + item.Msg
	var rendered string
	switch item.Kind {
	case NotifKindHITL:
		rendered = notifHITLStyle.Render(label)
	case NotifKindError:
		rendered = notifErrorStyle.Render(label)
	case NotifKindTaskDone:
		rendered = notifSuccessStyle.Render(label)
	case NotifKindDeferred:
		rendered = notifInfoStyle.Render(label)
	default:
		rendered = notifInfoStyle.Render(label)
	}
	if item.Kind == NotifKindHITL {
		rendered = rendered + dimStyle.Render("  [y] once  [s] session  [a] always  [n] deny  [d] dismiss")
	}
	return rendered
}

// PromptOverlay returns a modal overlay for notifications that require
// explicit confirmation rather than a banner-only acknowledgement.
func (nb *NotificationBar) PromptOverlay() (Overlay, bool) {
	if nb == nil || nb.queue == nil {
		return nil, false
	}
	current, ok := nb.queue.Current()
	if !ok {
		return nil, false
	}
	switch current.Kind {
	case NotifKindRestore, NotifKindDeferred:
		return &notificationPromptOverlay{queue: nb.queue, item: current}, true
	default:
		return nil, false
	}
}

type notificationPromptOverlay struct {
	queue *NotificationQueue
	item  NotificationItem
}

func (o *notificationPromptOverlay) Render(width, height int) string {
	if o == nil {
		return ""
	}
	_ = height
	title := "Notification"
	if o.item.Kind == NotifKindRestore {
		title = "Restore Session"
	} else if o.item.Kind == NotifKindDeferred {
		title = "Deferred Review"
	}
	lines := []string{
		panelHeaderStyle.Render(title),
		o.item.Msg,
	}
	switch o.item.Kind {
	case NotifKindRestore:
		lines = append(lines, dimStyle.Render("[enter] restore  [d] dismiss"))
	case NotifKindDeferred:
		lines = append(lines, dimStyle.Render("[enter] review  [d] dismiss"))
	}
	w := width
	if w < 1 {
		w = 1
	}
	return panelStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func (o *notificationPromptOverlay) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if o == nil || o.queue == nil {
		return nil, false
	}
	switch msg.String() {
	case "enter":
		id := o.item.ID
		o.queue.Resolve(id)
		switch o.item.Kind {
		case NotifKindRestore:
			return func() tea.Msg { return NotifRestoreSessionMsg{ID: id} }, true
		case NotifKindDeferred:
			return func() tea.Msg { return NotifReviewDeferredMsg{} }, true
		}
	case "d", "esc":
		id := o.item.ID
		o.queue.Resolve(id)
		return func() tea.Msg { return NotifDismissMsg{ID: id} }, true
	}
	return nil, false
}

func renderApprovalNotification(item NotificationItem) string {
	parts := []string{item.Msg}
	if kind := item.Extra["kind"]; kind != "" {
		parts = append(parts, "kind="+kind)
	}
	if risk := item.Extra["risk"]; risk != "" {
		parts = append(parts, "risk="+risk)
	}
	if target := item.Extra["target"]; target != "" {
		parts = append(parts, "target="+target)
	}
	if reason := item.Extra["reason"]; reason != "" {
		parts = append(parts, "why="+reason)
	}
	return strings.Join(parts, " | ")
}

func interactionDefaultSlotID(item NotificationItem) (string, bool) {
	if item.Extra == nil {
		return "", false
	}
	if slot := strings.TrimSpace(item.Extra["default_slot"]); slot != "" {
		return slot, true
	}
	if slot := strings.TrimSpace(item.Extra["slot_0_id"]); slot != "" {
		return slot, true
	}
	return "", false
}

func interactionSlotID(item NotificationItem, key string) (string, bool) {
	if item.Extra == nil {
		return "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	index := int(key[0] - '1')
	if index < 0 {
		return "", false
	}
	slotKey := fmt.Sprintf("slot_%d_id", index)
	if slotID := strings.TrimSpace(item.Extra[slotKey]); slotID != "" {
		return slotID, true
	}
	return "", false
}

func approvalKindFromRequest(req *fauthorization.PermissionRequest) string {
	if req == nil {
		return "execution"
	}
	for _, key := range []string{"approval_kind", "kind", "operation_kind"} {
		if value := req.Permission.Metadata[key]; value != "" {
			return value
		}
	}
	action := req.Permission.Action
	switch {
	case strings.Contains(action, "insert"):
		return "insertion"
	case strings.Contains(action, "admission"):
		return "admission"
	case strings.Contains(action, "provider"), strings.Contains(action, "session"):
		return "provider_operation"
	default:
		return "execution"
	}
}

func hitlTarget(req *fauthorization.PermissionRequest) string {
	if req == nil {
		return ""
	}
	for _, key := range []string{"target_resource", "capability_id", "provider_id", "session_id"} {
		if value := req.Permission.Metadata[key]; value != "" {
			return value
		}
	}
	return req.Permission.Resource
}
