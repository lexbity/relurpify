package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	runtimesvc "github.com/lexcodex/relurpify/app/relurpish/runtime"
	fauthorization "github.com/lexcodex/relurpify/framework/authorization"
)

// Run bootstraps the TUI. This is the public entrypoint called by cmd/start.go.
func Run(ctx context.Context, rt *runtimesvc.Runtime) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	m := newRootModel(newRuntimeAdapter(rt))
	program := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	// Inject the interaction emitter into the euclo agent after program is created
	emitter := NewTUIFrameEmitter(program)
	if m.runtime != nil {
		m.runtime.SetInteractionEmitter(emitter)
	}
	m.eucloEmitter = emitter

	final, err := program.Run()
	if rm, ok := final.(RootModel); ok {
		rm.cleanup()
	}
	return err
}

// RootModel is the top-level Bubble Tea model. It owns the layout and routes
// messages to focused panes.  Panes are held by pointer so mutations survive
// the value-semantics copy that Bubble Tea makes on every Update call.
type RootModel struct {
	// Components (value types — cheap to copy)
	titleBar TitleBar
	tabBar   TabBar
	notifBar *NotificationBar
	inputBar *InputBar
	notifQ   *NotificationQueue

	// Panes (pointer types — mutations survive tea.Model value copies)
	chat     *ChatPane
	tasks    *TasksPane
	session  *SessionPane
	settings *SettingsPane
	tools    *ToolsPane

	// Shared state
	activeTab    TabID
	titleVisible bool
	searchActive bool
	showHelp     bool
	help         HelpOverlay
	ready        bool
	width        int
	height       int

	// Session-level state shared across panes
	sharedSess *Session
	sharedCtx  *AgentContext
	runtime    RuntimeAdapter
	store      *SessionStore

	// Interaction emitter for euclo agent
	eucloEmitter *TUIFrameEmitter

	// HITL subscription
	hitlCh    <-chan fauthorization.HITLEvent
	hitlUnsub func()

	// Task queue: maps run IDs that originated from the task queue.
	taskRunIDs map[string]bool
}

func newRootModel(rt RuntimeAdapter) RootModel {
	info := SessionInfo{MaxTokens: 100000}
	if rt != nil {
		info = rt.SessionInfo()
	}

	sess := &Session{
		ID:        fmt.Sprintf("session-%d", time.Now().UnixNano()),
		StartTime: time.Now(),
		Workspace: info.Workspace,
		Model:     info.Model,
		Agent:     info.Agent,
		Role:      info.Role,
		Mode:      info.Mode,
		Strategy:  info.Strategy,
	}

	ctx := &AgentContext{
		Files:     []string{},
		MaxTokens: info.MaxTokens,
	}

	notifQ := &NotificationQueue{}

	m := RootModel{
		titleBar:     NewTitleBar(info),
		tabBar:       NewTabBar(TabChat),
		notifBar:     NewNotificationBar(notifQ),
		inputBar:     NewInputBar(),
		notifQ:       notifQ,
		activeTab:    TabChat,
		titleVisible: true,
		sharedSess:   sess,
		sharedCtx:    ctx,
		runtime:      rt,
		taskRunIDs:   make(map[string]bool),
	}

	var store *SessionStore
	if info.Workspace != "" {
		store = NewSessionStore(info.Workspace)
	}
	m.store = store

	m.chat = NewChatPane(rt, ctx, sess, notifQ)
	m.tasks = NewTasksPane(rt, notifQ)
	m.session = NewSessionPane(ctx, sess, rt)
	m.settings = NewSettingsPane(rt, store)
	m.tools = NewToolsPane(rt)

	return m
}

// sessionFoundMsg carries the latest persisted session found at startup.
type sessionFoundMsg struct{ rec SessionRecord }

// hitlSubscribedMsg carries the HITL subscription info from initialization.
type hitlSubscribedMsg struct {
	ch    <-chan fauthorization.HITLEvent
	unsub func()
}

// Init starts the HITL listener, spinner, and text-input blink.
func (m RootModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.chat.Init(),
		m.session.Init(),
		m.settings.Init(),
		m.restorePromptCmd(),
		m.subscribeHITLCmd(),
	)
}

// restorePromptCmd checks for a saved session and emits sessionFoundMsg if one exists.
func (m RootModel) restorePromptCmd() tea.Cmd {
	if m.store == nil {
		return nil
	}
	store := m.store
	return func() tea.Msg {
		rec, ok, _ := store.Latest()
		if !ok || len(rec.Messages) == 0 {
			return nil
		}
		return sessionFoundMsg{rec: rec}
	}
}

// subscribeHITLCmd subscribes to HITL events and returns the subscription info.
func (m RootModel) subscribeHITLCmd() tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		if rt == nil {
			return nil
		}
		ch, unsub := rt.SubscribeHITL()
		return hitlSubscribedMsg{ch: ch, unsub: unsub}
	}
}

// Update is the central message router.
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		return m.handleResize(msg)

	case GlobalKeyMsg:
		return m.handleGlobalKey(msg.Key)

	case tea.KeyMsg:
		// Quit shortcuts bypass the input bar.
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
		}
		// Notification bar captures keys when active.
		if m.notifBar.Active() {
			nb, cmd := m.notifBar.Update(msg)
			m.notifBar = nb
			return m, cmd
		}
		// Route to input bar first.
		ib, ibCmd := m.inputBar.Update(msg, m.activeTab)
		m.inputBar = ib
		return m, ibCmd

	case InputSubmittedMsg:
		return m.handleInputSubmitted(msg.Value)

	case CommandInvokedMsg:
		nm, cmd := executeCommand(&m, msg.Name, msg.Args)
		return *nm, cmd

	// Notification responses
	case NotifHITLApproveMsg:
		cmds := []tea.Cmd{approveHITLRootCmd(m.chat.hitlSvc, msg.ID, msg.Scope)}
		if msg.Scope == fauthorization.GrantScopePersistent {
			cmds = append(cmds, savePolicyCmd(m.runtime, msg.Action))
		}
		return m, tea.Batch(cmds...)
	case NotifHITLDenyMsg:
		return m, denyHITLRootCmd(m.chat.hitlSvc, msg.ID)
	case NotifDismissMsg:
		m.notifQ.Resolve(msg.ID)
		return m, nil
	case NotifRestoreSessionMsg:
		return m.handleRestoreSession(msg.ID)

	// Stream events — always routed to chat pane regardless of active tab.
	case streamDoneMsg:
		m.autoSave()
		m.session.SyncChanges(m.latestChanges())
		m.session.SyncContext(m.sharedCtx)
		if m.taskRunIDs[msg.RunID] {
			m.tasks.MarkComplete(msg.RunID)
			delete(m.taskRunIDs, msg.RunID)
		}
		return m, m.dequeueNextTask()

	// Startup session restore prompt.
	case sessionFoundMsg:
		if m.notifQ != nil && len(msg.rec.Messages) > 0 {
			m.notifQ.Push(NotificationItem{
				ID:   msg.rec.ID,
				Kind: NotifKindRestore,
				Msg:  fmt.Sprintf("Resume session (%s, %d messages)?", msg.rec.Agent, len(msg.rec.Messages)),
			})
		}
		return m, nil

	// HITL subscription initialization.
	case hitlSubscribedMsg:
		m.hitlCh = msg.ch
		m.hitlUnsub = msg.unsub
		if m.hitlCh != nil {
			return m, listenHITLEvents(m.hitlCh)
		}
		return m, nil

	// Settings pane models.
	case settingsModelsMsg:
		sp, cmd := m.settings.Update(msg)
		m.settings = sp
		return m, cmd

	// Settings pane sessions.
	case settingsSessionsMsg:
		sp, cmd := m.settings.Update(msg)
		m.settings = sp
		return m, cmd

	// File index for session pane.
	case fileIndexMsg:
		sp, cmd := m.session.Update(msg)
		m.session = sp
		return m, cmd

	// Chat-specific messages routed always to chat.
	case chatSystemMsg:
		p, cmd := m.chat.Update(msg)
		m.chat = p
		return m, cmd

	// HITL event handling.
	case hitlEventMsg:
		return m.handleHITLEvent(msg)
	case hitlResolvedMsg:
		return m.handleHITLResolved(msg)

	// Euclo interaction frame handling.
	case eucloFrameMsg:
		// Push the frame as an interactive notification
		if m.notifQ != nil {
			m.notifQ.PushInteraction(msg.frame)
		}
		// Add the rendered frame to the chat feed
		if m.chat != nil && m.chat.feed != nil {
			m.chat.feed.AppendMessage(msg.message)
		}
		return m, nil

	// Euclo interaction response handling.
	case eucloResponseMsg:
		if m.eucloEmitter != nil {
			m.eucloEmitter.Resolve(msg.response)
		}
		return m, nil
	}

	// Route to active pane + chat (chat always listens for stream/spinner msgs).
	return m.routeToActivePanes(msg)
}

// routeToActivePanes fans the message to the chat pane (always) and the
// currently visible pane if different.
func (m RootModel) routeToActivePanes(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	chatPane, chatCmd := m.chat.Update(msg)
	m.chat = chatPane
	if chatCmd != nil {
		cmds = append(cmds, chatCmd)
	}

	switch m.activeTab {
	case TabTasks:
		tp, cmd := m.tasks.Update(msg)
		m.tasks = tp
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case TabSession:
		sp, cmd := m.session.Update(msg)
		m.session = sp
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case TabSettings:
		sp, cmd := m.settings.Update(msg)
		m.settings = sp
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case TabTools:
		tp, cmd := m.tools.Update(msg)
		m.tools = tp
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View composes the full terminal screen.
func (m RootModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	parts := []string{}

	if m.titleVisible {
		parts = append(parts, m.titleBar.View())
	}

	// Active pane content.
	parts = append(parts, m.activePaneView())

	// Notification banner (conditional).
	if m.notifBar.Active() {
		parts = append(parts, m.notifBar.View())
	}

	// Input bar (always).
	streaming := m.chat != nil && m.chat.HasActiveRuns()
	parts = append(parts, m.inputBar.View(m.activeTab, streaming))

	// Tab bar (always).
	parts = append(parts, m.tabBar.View())

	base := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Help overlay sits on top of everything.
	if m.showHelp {
		return m.help.View(base)
	}
	return base
}

func (m RootModel) activePaneView() string {
	switch m.activeTab {
	case TabTasks:
		return m.tasks.View()
	case TabSession:
		return m.session.View()
	case TabSettings:
		return m.settings.View()
	case TabTools:
		return m.tools.View()
	default:
		return m.chat.View()
	}
}

// handleResize distributes new terminal dimensions to all components.
func (m RootModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true

	_, paneH, _, _ := m.layoutHeights()

	m.titleBar.SetWidth(msg.Width)
	m.tabBar.SetWidth(msg.Width)
	m.notifBar.SetWidth(msg.Width)
	m.inputBar.SetWidth(msg.Width)
	m.help.SetSize(msg.Width, msg.Height)

	m.chat.SetSize(msg.Width, paneH)
	m.tasks.SetSize(msg.Width, paneH)
	m.session.SetSize(msg.Width, paneH)
	m.settings.SetSize(msg.Width, paneH)
	m.tools.SetSize(msg.Width, paneH)

	return m, nil
}

// layoutHeights computes component heights for the current terminal size.
func (m RootModel) layoutHeights() (title, pane, input, tab int) {
	title = 0
	if m.titleVisible {
		title = 1
	}
	tab = 1
	input = 1
	notif := 0
	if m.notifBar.Active() {
		notif = 1
	}
	pane = m.height - title - tab - input - notif
	if pane < 1 {
		pane = 1
	}
	return
}

// handleGlobalKey processes navigation keys emitted by InputBar when the
// input field is empty.
func (m RootModel) handleGlobalKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c", "ctrl+d":
		return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
	case "1":
		m.activeTab = TabChat
		m.tabBar.SetActive(TabChat)
	case "2":
		m.activeTab = TabTasks
		m.tabBar.SetActive(TabTasks)
	case "3":
		m.activeTab = TabSession
		m.tabBar.SetActive(TabSession)
	case "4":
		m.activeTab = TabSettings
		m.tabBar.SetActive(TabSettings)
	case "5":
		m.activeTab = TabTools
		m.tabBar.SetActive(TabTools)
	case "tab":
		next := TabID(int(m.activeTab)%5 + 1)
		m.activeTab = next
		m.tabBar.SetActive(next)
	case "shift+tab":
		prev := TabID((int(m.activeTab)-2+5)%5 + 1)
		m.activeTab = prev
		m.tabBar.SetActive(prev)
	case "ctrl+t":
		m.titleVisible = !m.titleVisible
		_, paneH, _, _ := m.layoutHeights()
		m.chat.SetSize(m.width, paneH)
		m.tasks.SetSize(m.width, paneH)
		m.session.SetSize(m.width, paneH)
		m.settings.SetSize(m.width, paneH)
		m.tools.SetSize(m.width, paneH)
	case "ctrl+f":
		m.searchActive = !m.searchActive
		m.inputBar.SetSearchMode(m.searchActive)
		if !m.searchActive && m.chat != nil {
			m.chat.feed.SetSearchFilter("")
		}
	case "?":
		m.showHelp = !m.showHelp
	case "esc":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		m.searchActive = false
		m.inputBar.SetSearchMode(false)
		if m.chat != nil {
			m.chat.feed.SetSearchFilter("")
		}
	case "ctrl+z":
		// Undo: remove the last message from chat feed
		if m.chat != nil {
			m.chat.Undo()
		}
	case "ctrl+y":
		// Redo: restore the last undone message
		if m.chat != nil {
			m.chat.Redo()
		}
	case "ctrl+u":
		// Scroll up: scroll the chat feed up
		if m.chat != nil && m.activeTab == TabChat {
			m.chat.feed.ScrollUp()
		}
	case "pagedown":
		// Page down: scroll the chat feed down by page
		if m.chat != nil && m.activeTab == TabChat {
			m.chat.feed.PageDown()
		}
	case "pageup":
		// Page up: scroll the chat feed up by page
		if m.chat != nil && m.activeTab == TabChat {
			m.chat.feed.PageUp()
		}
	case "@":
		// File picker: enable file selection mode in input
		m.inputBar.SetFilePickerMode(true)
	case "ctrl+k":
		// Compact: toggle message compactness in chat feed
		if m.chat != nil {
			m.chat.ToggleCompact()
		}
	}
	return m, nil
}

// handleInputSubmitted routes a submitted value to the active pane.
func (m RootModel) handleInputSubmitted(value string) (tea.Model, tea.Cmd) {
	value = strings.TrimSpace(value)
	if value == "" {
		return m, nil
	}
	// Search mode: filter the chat feed instead of submitting a prompt.
	if m.searchActive && m.chat != nil {
		m.chat.feed.SetSearchFilter(value)
		return m, nil
	}
	switch m.activeTab {
	case TabTasks:
		cmd := m.tasks.HandleInputSubmit(value)
		dequeueCmd := m.dequeueNextTask()
		return m, tea.Batch(cmd, dequeueCmd)
	case TabSession:
		m.session.HandleFilterInput(value)
		return m, nil
	default:
		cmd := m.chat.HandleInputSubmit(value)
		return m, cmd
	}
}

// handleRestoreSession loads a saved session into the chat pane.
func (m RootModel) handleRestoreSession(id string) (tea.Model, tea.Cmd) {
	if m.store == nil {
		return m, nil
	}
	rec, err := m.store.Load(id)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Restore failed: %v", err))
		return m, nil
	}
	m.notifQ.Resolve(id)
	for _, msg := range rec.Messages {
		m.chat.feed.AppendMessage(msg)
	}
	if rec.Context != nil {
		m.sharedCtx.Files = rec.Context.Files
	}
	m.addSystemMessage(fmt.Sprintf("Restored session %s (%d messages)", id, len(rec.Messages)))
	return m, nil
}

// addSystemMessage adds a system line to the chat feed.
func (m *RootModel) addSystemMessage(text string) {
	if m.chat != nil {
		m.chat.addSystemMessage(text)
	}
}

// autoSave persists the current session after each completed run.
func (m RootModel) autoSave() {
	if m.store == nil || m.chat == nil {
		return
	}
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:        m.sharedSess.ID,
			Agent:     m.sharedSess.Agent,
			Workspace: m.sharedSess.Workspace,
			UpdatedAt: time.Now(),
		},
		Messages: m.chat.feed.Messages(),
		Context:  m.sharedCtx,
	}
	_ = m.store.Save(rec) // fire-and-forget; errors are silently dropped
}

// cleanup cancels all runs and unsubscribes HITL.
func (m RootModel) cleanup() {
	if m.hitlUnsub != nil {
		m.hitlUnsub()
	}
	if m.chat != nil {
		m.chat.Cleanup()
	}
}

// handleHITLEvent processes HITL events from the subscription.
func (m RootModel) handleHITLEvent(msg hitlEventMsg) (RootModel, tea.Cmd) {
	var pending []*fauthorization.PermissionRequest
	if m.chat != nil && m.chat.hitlSvc != nil {
		pending = m.chat.hitlSvc.PendingHITL()
	}
	switch msg.event.Type {
	case fauthorization.HITLEventRequested:
		req := msg.event.Request
		if req == nil && len(pending) > 0 {
			req = pending[0]
		}
		if req != nil && m.notifQ != nil {
			m.notifQ.PushHITL(req)
		}
	case fauthorization.HITLEventResolved, fauthorization.HITLEventExpired:
		if msg.event.Request != nil && m.notifQ != nil {
			m.notifQ.Resolve(msg.event.Request.ID)
		}
		if msg.event.Type == fauthorization.HITLEventExpired && msg.event.Request != nil {
			reason := msg.event.Error
			if reason == "" {
				reason = "expired"
			}
			m.addSystemMessage(fmt.Sprintf("Permission %s expired: %s", msg.event.Request.ID, reason))
		}
	}
	// Re-queue the listener to continue draining the channel
	return m, listenHITLEvents(m.hitlCh)
}

// handleHITLResolved processes HITL resolution messages.
func (m RootModel) handleHITLResolved(msg hitlResolvedMsg) (RootModel, tea.Cmd) {
	if m.notifQ != nil {
		m.notifQ.Resolve(msg.requestID)
	}
	if msg.err != nil {
		m.addSystemMessage(fmt.Sprintf("HITL %s failed: %v", msg.requestID, msg.err))
	} else if msg.approved {
		m.addSystemMessage(fmt.Sprintf("Approved %s", msg.requestID))
	} else {
		m.addSystemMessage(fmt.Sprintf("Denied %s", msg.requestID))
	}
	// Re-queue the listener to continue draining the channel
	return m, listenHITLEvents(m.hitlCh)
}

// dequeueNextTask starts the next pending task from the task queue, if any.
// It is a no-op when a run is already active.
func (m *RootModel) dequeueNextTask() tea.Cmd {
	if m.chat == nil || m.chat.HasActiveRuns() {
		return nil
	}
	item, ok := m.tasks.NextPending()
	if !ok {
		return nil
	}
	cmd, runID := m.chat.StartRun(item.Description)
	if runID == "" {
		return cmd
	}
	m.tasks.MarkInProgress(item.ID, runID)
	if m.taskRunIDs == nil {
		m.taskRunIDs = make(map[string]bool)
	}
	m.taskRunIDs[runID] = true
	return cmd
}

// latestChanges extracts FileChange items from the most recent agent message.
func (m RootModel) latestChanges() []FileChange {
	if m.chat == nil {
		return nil
	}
	msgs := m.chat.feed.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleAgent && len(msgs[i].Content.Changes) > 0 {
			return append([]FileChange(nil), msgs[i].Content.Changes...)
		}
	}
	return nil
}
