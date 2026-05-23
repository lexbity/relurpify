package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Run bootstraps the TUI with the default interaction surface factory.
func Run(ctx context.Context, rt *runtimesvc.Runtime) error {
	return RunWithSurface(ctx, rt, NewDefaultSurfaceFactory())
}

// RunWithSurface bootstraps the TUI with an agent-surface factory.
func RunWithSurface(ctx context.Context, rt *runtimesvc.Runtime, factory SurfaceFactory) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	adapter := newRuntimeAdapter(rt)
	m := newRootModel(adapter, factory)
	program := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
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
	// Layout and tab registry (Phase A infrastructure).
	layout    ChromeLayout
	tabs      *TabRegistry
	subTabBar SubTabBar

	// Components (value types — cheap to copy)
	tabBar     TabBar
	notifBar   *NotificationBar
	inputBar   *InputBar
	cmdPalette *CommandPalette
	notifQ     *NotificationQueue
	overlays   *OverlayStack

	// Panes (pointer types — mutations survive tea.Model value copies)
	chat          ChatPaner
	tasks         *TasksPane
	session       *SessionPane
	sandbox       *SandboxPane
	securityguard *SecurityGuardPane
	aiprovider    *AIProviderPane
	library       *LibraryPane
	welcome       *WelcomePane
	keybindings   *KeybindingPane
	doctor        *DoctorPane

	// Shared state
	activeTab     TabID
	searchActive  bool
	showHelp      bool
	help          HelpOverlay
	ready         bool
	width         int
	height        int
	focus         FocusRouter
	activeAgent   string
	agentPicker   *AgentPicker
	startupLocked bool

	// Session-level state shared across panes
	sharedSess     *Session
	sharedCtx      *AgentContext
	runtime        RuntimeAdapter
	store          *SessionStore
	surfaceFactory SurfaceFactory
	surfaceCache   map[string]*surfaceState
	activeSurface  AgentSurface

	// HITL subscription
	hitlCh    <-chan fauthorization.HITLEvent
	hitlUnsub func()

	// Interaction frames keyed by notification ID and frame ID so the host can
	// resolve slot selections and freetext answers back into the pending frame.
	interactionFrames map[string]*interaction.InteractionFrame

	// Task queue: maps run IDs that originated from the task queue.
	taskRunIDs map[string]bool

	// Guidance panel (Phase B): renders above input bar when open.
	hitlPanel GuidancePanel

	// Phase G: instance-based command registry and corpus scope.
	cmdReg *CommandRegistry
	scope  string
}

type surfaceState struct {
	surface AgentSurface
	tabs    *TabRegistry
	cmdReg  *CommandRegistry
	chat    ChatPaner
}

func newRootModel(rt RuntimeAdapter, factory SurfaceFactory) RootModel {
	info := SessionInfo{MaxTokens: 100000}
	if rt != nil {
		info = rt.SessionInfo()
	}

	sess := &Session{
		ID:            fmt.Sprintf("session-%d", time.Now().UnixNano()),
		StartTime:     time.Now(),
		Workspace:     info.Workspace,
		Provider:      info.Provider,
		BackendState:  info.BackendState,
		Model:         info.Model,
		Agent:         info.Agent,
		Role:          info.Role,
		Mode:          info.Mode,
		Strategy:      info.Strategy,
		Profile:       info.Profile,
		ProfileReason: info.ProfileReason,
		ProfileSource: info.ProfileSource,
	}

	ctx := &AgentContext{
		Files:     []string{},
		MaxTokens: info.MaxTokens,
	}

	notifQ := &NotificationQueue{}
	if factory == nil {
		factory = NewDefaultSurfaceFactory()
	}

	inputBar := NewInputBar()
	if info.Workspace != "" {
		inputBar.SetWorkspace(info.Workspace)
	}
	if rt != nil {
		inputBar.SetRuntime(rt)
	}
	if strings.TrimSpace(sess.Agent) == "" {
		sess.Agent = "none"
	}
	initialAgent := normalizeSurfaceKey(info.Agent)
	if initialAgent == "" {
		initialAgent = "none"
	}
	state := buildSurfaceState(factory, initialAgent, rt, ctx, sess, notifQ)
	inputBar.SetCommandRegistry(state.cmdReg)
	inputBar.SetContext(state.tabs.ActiveTab().ID, state.tabs.ActiveSubTab())

	tabBar := NewTabBar(state.tabs.ActiveTab().ID)
	tabBar.SetRegistry(state.tabs)

	m := RootModel{
		tabs:              state.tabs,
		subTabBar:         NewSubTabBar(state.tabs.ActiveTab()),
		hitlPanel:         newGuidancePanel(),
		tabBar:            tabBar,
		notifBar:          NewNotificationBar(notifQ),
		inputBar:          inputBar,
		cmdPalette:        NewCommandPalette(),
		notifQ:            notifQ,
		overlays:          NewOverlayStack(),
		activeAgent:       initialAgent,
		agentPicker:       NewAgentPicker(),
		activeTab:         state.tabs.ActiveTab().ID,
		focus:             NewFocusRouter(),
		sharedSess:        sess,
		sharedCtx:         ctx,
		runtime:           rt,
		interactionFrames: make(map[string]*interaction.InteractionFrame),
		taskRunIDs:        make(map[string]bool),
		cmdReg:            state.cmdReg,
		scope:             info.Workspace,
		surfaceFactory:    factory,
		surfaceCache:      map[string]*surfaceState{initialAgent: state},
		activeSurface:     state.surface,
	}
	m.notifBar.SetInteractionRenderer(state.surface.RenderNotification)

	var store *SessionStore
	if info.Workspace != "" {
		store = NewSessionStore(info.Workspace)
	}
	m.store = store

	m.chat = state.chat
	m.tasks = NewTasksPane(rt, notifQ)
	m.session = NewSessionPane(ctx, sess, rt)
	m.session.SyncQueuedTasks(m.tasks.Items())
	m.sandbox = NewSandboxPane(rt)
	m.securityguard = NewSecurityGuardPane(rt)
	m.aiprovider = NewAIProviderPane(rt)
	if rt != nil {
		m.library = NewLibraryPane(rt)
	} else {
		m.library = &LibraryPane{
			tagFilters: make(map[string]bool),
			lastUsed:   make(map[string]time.Time),
		}
	}
	m.welcome = NewWelcomePane(sess, m.store)
	m.keybindings = NewKeybindingPane(rt)
	m.doctor = NewDoctorPane(rt)
	if rt != nil {
		m.applyStartupGate()
	}
	m.setFocus(FocusRegionInput)
	m.setActiveTab(m.activeTab)
	m.syncActivePaneFilter(m.inputBar.Value())

	return m
}

func buildSurfaceState(factory SurfaceFactory, agentName string, rt RuntimeAdapter, ctx *AgentContext, sess *Session, notifQ *NotificationQueue) *surfaceState {
	surface := factory.Resolve(agentName)
	if surface == nil {
		surface = newGenericSurface()
	}
	tabs := NewTabRegistry()
	surface.RegisterTabs(tabs)
	initialTab := surface.InitialTab()
	if initialTab == "" {
		initialTab = tabs.ActiveTab().ID
	}
	if initialTab == "" && tabs.Len() > 0 {
		initialTab = tabs.All()[0].ID
	}
	if initialTab == "" {
		initialTab = TabWelcome
	}
	tabs.SetActive(initialTab)
	initialSub := surface.InitialSubTab(initialTab)
	if initialSub != "" {
		tabs.SetSubActive(initialTab, initialSub)
	}
	cmdReg := NewCommandRegistry()
	registerUniversalCommands(cmdReg)
	surface.RegisterCommands(cmdReg)
	chat := surface.NewChat(rt, ctx, sess, notifQ)
	if tabAware, ok := chat.(TabAwarePane); ok {
		tabAware.SetActiveTab(initialTab)
	}
	return &surfaceState{
		surface: surface,
		tabs:    tabs,
		cmdReg:  cmdReg,
		chat:    chat,
	}
}

func (m *RootModel) activateSurface(agentName string) {
	if m == nil {
		return
	}
	if m.surfaceCache == nil {
		m.surfaceCache = make(map[string]*surfaceState)
	}
	key := normalizeSurfaceKey(agentName)
	if key == "" {
		key = normalizeSurfaceKey(m.activeAgent)
	}
	if key == "" {
		key = "none"
	}
	state, ok := m.surfaceCache[key]
	if !ok || state == nil {
		state = buildSurfaceState(m.surfaceFactory, agentName, m.runtime, m.sharedCtx, m.sharedSess, m.notifQ)
		m.surfaceCache[key] = state
	}
	m.activeAgent = key
	m.activeSurface = state.surface
	m.tabs = state.tabs
	m.cmdReg = state.cmdReg
	m.chat = state.chat
	if m.inputBar != nil {
		m.inputBar.SetCommandRegistry(state.cmdReg)
		m.inputBar.SetContext(m.tabs.ActiveTab().ID, m.tabs.ActiveSubTab())
	}
	m.tabBar.SetRegistry(state.tabs)
	m.tabBar.SetActive(m.tabs.ActiveTab().ID)
	m.subTabBar.SetSubTabs(m.tabs.ActiveTab())
	m.subTabBar.SetActive(m.tabs.ActiveSubTab())
	if m.notifBar != nil && state.surface != nil {
		m.notifBar.SetInteractionRenderer(state.surface.RenderNotification)
	}
	m.activeTab = m.tabs.ActiveTab().ID
	m.setActiveTab(m.activeTab)
}

func (m *RootModel) switchActiveAgent(agentName string) error {
	if m == nil {
		return nil
	}
	name := normalizeSurfaceKey(agentName)
	if name == "" {
		name = "none"
	}
	if m.startupLocked && name != "none" {
		return fmt.Errorf("startup checks failed; agent switching is locked")
	}
	if m.runtime != nil && name != "none" {
		if err := m.runtime.SwitchAgent(name); err != nil {
			return err
		}
	}
	if m.sharedSess != nil {
		m.sharedSess.Agent = name
	}
	m.activateSurface(name)
	return nil
}

func (m *RootModel) availableAgents() []string {
	seen := map[string]struct{}{"none": struct{}{}}
	if m.startupLocked {
		return []string{"none"}
	}
	if m.runtime != nil {
		for _, name := range m.runtime.AvailableAgents() {
			name = normalizeSurfaceKey(name)
			if name == "" {
				continue
			}
			seen[name] = struct{}{}
		}
	}
	if current := normalizeSurfaceKey(m.activeAgentName()); current != "" && current != "none" {
		seen[current] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	if _, ok := seen["none"]; ok {
		out = append(out, "none")
		delete(seen, "none")
	}
	var rest []string
	for name := range seen {
		rest = append(rest, name)
	}
	sort.Strings(rest)
	out = append(out, rest...)
	return out
}

func (m *RootModel) openAgentPicker() {
	if m == nil || m.agentPicker == nil {
		return
	}
	if m.startupLocked {
		return
	}
	if m.overlays != nil && m.overlays.Len() > 0 {
		return
	}
	items := m.availableAgents()
	current := m.activeAgentName()
	m.agentPicker.Open(items, current)
	m.syncOverlayStack()
}

func (m *RootModel) closeAgentPicker() {
	if m == nil || m.agentPicker == nil {
		return
	}
	m.agentPicker.Close()
}

func (m *RootModel) syncCommandPalette() {
	if m.inputBar == nil || m.cmdPalette == nil {
		return
	}
	open, items, sel, label := m.inputBar.PaletteState()
	m.cmdPalette.Sync(open, items, sel, m.width, label)
}

func (m *RootModel) syncOverlayStack() {
	if m.overlays == nil {
		m.overlays = NewOverlayStack()
	}
	m.overlays.Clear()
	if m.agentPicker != nil && m.agentPicker.IsOpen() {
		picker := m.agentPicker
		m.overlays.Push(overlayFunc{
			render: func(width, height int) string {
				return picker.Render(width, height)
			},
			handle: func(msg tea.KeyMsg) (tea.Cmd, bool) {
				selected, handled := picker.HandleKey(msg)
				if !handled {
					return nil, false
				}
				if selected != "" {
					if err := m.switchActiveAgent(selected); err != nil {
						m.addSystemMessage(fmt.Sprintf("Agent switch failed: %v", err))
					} else {
						m.setFocus(FocusRegionInput)
					}
				}
				if msg.String() == "esc" || selected != "" {
					m.closeAgentPicker()
				}
				m.syncOverlayStack()
				return nil, true
			},
		})
		return
	}
	if m.cmdPalette != nil && m.cmdPalette.IsOpen() {
		palette := m.cmdPalette
		m.overlays.Push(overlayFunc{
			render: func(width, height int) string {
				_ = height
				_ = width
				return palette.View()
			},
			handle: func(msg tea.KeyMsg) (tea.Cmd, bool) {
				if m.inputBar == nil {
					return nil, false
				}
				ib, cmd := m.inputBar.Update(msg, m.activeTab)
				m.inputBar = ib
				m.syncCommandPalette()
				m.syncOverlayStack()
				return cmd, true
			},
		})
	}
	if m.inputBar != nil {
		if m.inputBar.PickerView() != "" {
			ib := m.inputBar
			m.overlays.Push(overlayFunc{
				render: func(width, height int) string {
					_ = width
					_ = height
					return ib.PickerView()
				},
				handle: func(msg tea.KeyMsg) (tea.Cmd, bool) {
					updated, cmd := ib.Update(msg, m.activeTab)
					m.inputBar = updated
					m.syncCommandPalette()
					m.syncOverlayStack()
					return cmd, true
				},
			})
		}
	}
	if m.notifBar != nil {
		if overlay, ok := m.notifBar.PromptOverlay(); ok {
			m.overlays.Push(overlay)
		}
	}
	if m.hitlPanel.IsOpen() {
		m.overlays.Push(&m.hitlPanel)
	}
}

func (m RootModel) refreshActiveSurfaceCmd() tea.Cmd {
	m.refreshActivePane()
	return nil
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
	cmds := []tea.Cmd{
		textinput.Blink,
		m.session.Init(),
		m.restorePromptCmd(),
		m.subscribeHITLCmd(),
	}
	if m.chat != nil {
		cmds = append(cmds, m.chat.Init())
	}
	return tea.Batch(cmds...)
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

	case tea.MouseMsg:
		if handled, cmd := m.handleMouse(msg); handled {
			return m, cmd
		}

	case GlobalKeyMsg:
		return m.handleGlobalKey(msg.Key)

	case tea.KeyMsg:
		// Quit shortcuts bypass everything.
		switch {
		case keyMatchesBinding(GlobalKeys.Quit, msg.String()):
			return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
		}
		if keyMatchesBinding(GlobalKeys.AgentPicker, msg.String()) {
			m.openAgentPicker()
			return m, nil
		}
		m.syncOverlayStack()
		if m.overlays != nil {
			if cmd, handled := m.overlays.HandleKey(msg); handled {
				m.syncCommandPalette()
				m.syncOverlayStack()
				return m, cmd
			}
		}
		// Notification bar captures keys when active unless the current guidance
		// request expects freetext input through the input bar.
		if m.notifBar.Active() {
			nb, cmd := m.notifBar.Update(msg)
			m.notifBar = nb
			m.syncOverlayStack()
			return m, cmd
		}
		if handled, cmd := m.routeFocusKey(msg); handled {
			return m, cmd
		}
		// Route to input bar first.
		ib, ibCmd := m.inputBar.Update(msg, m.activeTab)
		m.inputBar = ib
		m.syncCommandPalette()
		return m, ibCmd

	case InputSubmittedMsg:
		if m.cmdPalette != nil {
			m.cmdPalette.Close()
		}
		m.syncOverlayStack()
		return m.handleInputSubmitted(msg)

	case CommandInvokedMsg:
		if m.cmdPalette != nil {
			m.cmdPalette.Close()
		}
		m.syncOverlayStack()
		nm, cmd := executeCommand(&m, msg.Name, msg.Args)
		return *nm, cmd

	case libraryRunRequestedMsg:
		if m.inputBar != nil {
			m.inputBar.SetValue(msg.Prompt)
		}
		m.setFocus(FocusRegionInput)
		m.addSystemMessage(fmt.Sprintf("Preparing parameters for recipe %s", msg.RecipeID))
		return m, nil

	// Notification responses
	case NotifHITLApproveMsg:
		var hitlSvc HITLServiceIface
		if m.chat != nil {
			hitlSvc = m.chat.HITLService()
		}
		cmds := []tea.Cmd{approveHITLRootCmd(hitlSvc, msg.ID, msg.Scope)}
		if msg.Scope == fauthorization.GrantScopePersistent {
			cmds = append(cmds, savePolicyCmd(m.runtime, msg.Action))
		}
		return m, tea.Batch(cmds...)
	case NotifHITLDenyMsg:
		var hitlSvc HITLServiceIface
		if m.chat != nil {
			hitlSvc = m.chat.HITLService()
		}
		return m, denyHITLRootCmd(hitlSvc, msg.ID)
	case NotifDismissMsg:
		m.notifQ.Resolve(msg.ID)
		m.syncCommandPalette()
		m.syncOverlayStack()
		return m, nil
	case NotifRestoreSessionMsg:
		m.syncOverlayStack()
		return m.handleRestoreSession(msg.ID)
	case NotifReviewDeferredMsg:
		m.syncOverlayStack()
		return m, nil

	// Stream events — always routed to chat pane regardless of active tab.
	case streamDoneMsg:
		m.autoSave()
		m.session.SyncChanges(m.latestChanges())
		m.session.SyncContext(m.sharedCtx)
		if m.taskRunIDs[msg.RunID] {
			m.tasks.MarkComplete(msg.RunID)
			m.session.SyncQueuedTasks(m.tasks.Items())
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

	case workspaceSelectedMsg:
		return m.handleWorkspaceSelected(msg.Workspace)

	// HITL subscription initialization.
	case hitlSubscribedMsg:
		m.hitlCh = msg.ch
		m.hitlUnsub = msg.unsub
		if m.hitlCh != nil {
			return m, listenHITLEvents(m.hitlCh)
		}
		return m, nil

	// Diagnostics snapshot — route to session pane regardless of active tab.
	case DiagnosticsUpdatedMsg:
		if m.session != nil {
			m.session.SetDiagnostics(msg.Info)
		}
		return m, nil
	case SessionLiveSnapshotMsg:
		if m.session != nil {
			m.session.SetLiveSnapshot(msg.Info, msg.Workflows, msg.Providers, msg.Approvals)
		}
		return m, nil

	// Config refresh — forward to config pane regardless of active tab.
	case configRefreshMsg:
		if m.securityguard != nil {
			m.securityguard.Refresh()
		}
		if m.aiprovider != nil {
			m.aiprovider.Refresh()
		}
		if m.keybindings != nil {
			m.keybindings.Refresh()
		}
		return m, nil

	case doctorStatusMsg:
		if m.doctor != nil {
			m.doctor.report = msg.Report
			m.doctor.working = false
			m.doctor.progress = 1
			if msg.Err != nil {
				m.doctor.status = fmt.Sprintf("%s failed: %v", msg.Action, msg.Err)
				m.addSystemMessage(m.doctor.status)
				return m, nil
			}
			if msg.Action != "" {
				m.doctor.status = msg.Message
			}
		}
		m.applyDoctorReport(msg.Report)
		return m, nil

	case sandboxPersistedMsg:
		if msg.Err != nil {
			m.addSystemMessage(fmt.Sprintf("Sandbox save failed: %v", msg.Err))
			return m, nil
		}
		if m.sandbox != nil {
			m.sandbox.Refresh()
		}
		if msg.Workspace != "" && m.sharedSess != nil {
			m.sharedSess.Workspace = msg.Workspace
		}
		if msg.Backup != "" {
			m.addSystemMessage(fmt.Sprintf("Sandbox saved with backup %s", msg.Backup))
		} else {
			m.addSystemMessage("Sandbox saved")
		}
		m.syncActivePaneFilter(m.inputBar.Value())
		return m, nil

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

	// Guidance panel responses.
	case GuidancePanelSubmitMsg:
		if m.resolvePendingInteraction(msg.RequestID, msg.Response, "") {
			m.syncOverlayStack()
			return m, nil
		}
		m.syncCommandPalette()
		m.syncOverlayStack()
		return m, nil
	case GuidancePanelDeferMsg:
		if m.deferPendingInteraction(msg.RequestID) {
			m.syncOverlayStack()
			return m, nil
		}
		m.syncCommandPalette()
		m.syncOverlayStack()
		return m, nil
	case GuidancePanelAnnotateMsg:
		// Annotation saved; panel stays open — no further model action needed here.
		return m, nil
	case GuidancePanelJumpExploreMsg:
		m.addSystemMessage("expanded view is no longer available")
		return m, nil

	case NotifInteractionResolveMsg:
		if m.resolvePendingInteraction(msg.NotificationID, msg.ChoiceID, msg.Freetext) {
			m.syncOverlayStack()
			return m, nil
		}
		return m, nil

	// Surface interaction frame handling.
	case SurfaceFrameMsg:
		if m.activeSurface != nil {
			m.activeSurface.HandleFrame(context.Background(), &m, msg)
		}
		return m, nil

	// Git operations
	case gitStatusMsg:
		if msg.Err != nil {
			m.addSystemMessage(fmt.Sprintf("Error: %v", msg.Err))
			return m, nil
		}
		if len(msg.Modified) == 0 {
			m.addSystemMessage("nothing to commit")
			return m, nil
		}
		// Show files and prompt for message
		filesStr := strings.Join(msg.Modified, "\n")
		m.addSystemMessage(fmt.Sprintf("Modified files:\n%s\n\nUse /commit \"message here\" to commit", filesStr))
		return m, nil

	case gitCommitMsg:
		if msg.Err != nil {
			m.addSystemMessage(fmt.Sprintf("Commit failed: %v", msg.Err))
			return m, nil
		}
		m.addSystemMessage(fmt.Sprintf("✓ committed: %s", msg.Message))
		return m, nil

	case gitDiffStatMsg:
		if msg.Err != nil {
			m.addSystemMessage(fmt.Sprintf("Review failed: %v", msg.Err))
			return m, nil
		}
		if msg.Output == "" {
			m.addSystemMessage("no changes since last commit")
			return m, nil
		}
		m.addSystemMessage(fmt.Sprintf("Changes since last commit:\n\n%s", msg.Output))
		return m, nil

	case compactResultMsg:
		if msg.Err != nil {
			// Roll back the undo snapshot we pushed before the call.
			if m.chat != nil {
				m.chat.RollbackLastUndo()
			}
			m.addSystemMessage(fmt.Sprintf("Compact failed: %v", msg.Err))
			return m, nil
		}
		if m.chat != nil {
			m.chat.ClearMessages()
			m.chat.AddSystemMessage(fmt.Sprintf(
				"Session compacted — %d messages → 1 summary. [ctrl+z to undo]",
				msg.OriginalCount,
			))
			m.chat.AppendMessage(Message{
				ID:        fmt.Sprintf("compact-%d", time.Now().UnixNano()),
				Role:      RoleAgent,
				Timestamp: time.Now(),
				Content:   MessageContent{Text: msg.Summary},
			})
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

	if m.chat != nil {
		chatPane, chatCmd := m.chat.Update(msg)
		m.chat = chatPane
		if chatCmd != nil {
			cmds = append(cmds, chatCmd)
		}
	}

	switch m.activeTab {
	case TabWelcome:
		if m.welcome != nil {
			wp, cmd := m.welcome.Update(msg)
			m.welcome = wp
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case TabSandbox:
		if m.sandbox != nil {
			sp, cmd := m.sandbox.Update(msg)
			m.sandbox = sp
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case TabSecurityGuard:
		if m.securityguard != nil {
			cp, cmd := m.securityguard.Update(msg)
			m.securityguard = cp
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case TabAIProvider:
		if m.aiprovider != nil {
			pa, cmd := m.aiprovider.Update(msg)
			m.aiprovider = pa
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case TabKeybindings:
		if m.keybindings != nil {
			kp, cmd := m.keybindings.Update(msg)
			m.keybindings = kp
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case TabDoctor:
		if m.doctor != nil {
			dp, cmd := m.doctor.Update(msg)
			m.doctor = dp
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	case TabLibrary:
		if m.library != nil {
			lp, cmd := m.library.Update(msg)
			m.library = lp
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// View composes the full terminal screen.
func (m RootModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	parts := []string{
		lipgloss.JoinVertical(lipgloss.Left, m.subTabBar.View(), m.activePaneView()),
	}

	if m.overlays != nil {
		if overlay := m.overlays.Render(m.width, m.height); overlay != "" {
			parts = append(parts, overlay)
		}
	}

	if m.notifBar != nil && m.notifBar.Active() {
		parts = append(parts, m.notifBar.View())
	}

	streaming := m.chat != nil && m.chat.HasActiveRuns()
	bottom := lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderAgentCell(m.activeAgentName(), m.layout.Region2Width()),
		m.inputBar.View(m.activeTab, streaming),
	)
	parts = append(parts, bottom)
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
	case TabWelcome:
		if m.welcome != nil {
			return m.welcome.View()
		}
	case TabSandbox:
		if m.sandbox != nil {
			return m.sandbox.View()
		}
	case TabSecurityGuard:
		if m.securityguard != nil {
			return m.securityguard.View()
		}
	case TabAIProvider:
		if m.aiprovider != nil {
			return m.aiprovider.View()
		}
	case TabKeybindings:
		if m.keybindings != nil {
			return m.keybindings.View()
		}
	case TabDoctor:
		if m.doctor != nil {
			return m.doctor.View()
		}
	case TabLibrary:
		if m.library != nil {
			return m.library.View()
		}
	default:
		if m.chat != nil {
			return m.chat.View()
		}
	}
	return ""
}

// handleResize distributes new terminal dimensions to all components.
func (m RootModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true

	m.layout.Recalculate(msg.Width, msg.Height, m.notificationRowVisible())

	m.subTabBar.SetWidth(msg.Width)
	m.tabBar.SetWidth(msg.Width)
	if m.notifBar != nil {
		m.notifBar.SetWidth(msg.Width)
	}
	if m.inputBar != nil {
		m.inputBar.SetWidth(m.layout.Region3Width())
	}
	m.help.SetSize(msg.Width, msg.Height)

	paneH := m.layout.Region1PaneRows()
	if m.chat != nil {
		m.chat.SetSize(msg.Width, paneH)
	}
	m.session.SetSize(msg.Width, paneH)
	if m.sandbox != nil {
		m.sandbox.SetSize(msg.Width, paneH)
	}
	if m.securityguard != nil {
		m.securityguard.SetSize(msg.Width, paneH)
	}
	if m.aiprovider != nil {
		m.aiprovider.SetSize(msg.Width, paneH)
	}
	if m.welcome != nil {
		m.welcome.SetSize(msg.Width, paneH)
	}
	if m.keybindings != nil {
		m.keybindings.SetSize(msg.Width, paneH)
	}
	if m.doctor != nil {
		m.doctor.SetSize(msg.Width, paneH)
	}
	if m.library != nil {
		m.library.SetSize(msg.Width, paneH)
	}

	return m, nil
}

func (m RootModel) notificationRowVisible() bool {
	return m.notifBar != nil && m.notifBar.Active()
}

func (m RootModel) activeAgentName() string {
	if strings.TrimSpace(m.activeAgent) != "" {
		return m.activeAgent
	}
	if m.sharedSess != nil && strings.TrimSpace(m.sharedSess.Agent) != "" {
		return m.sharedSess.Agent
	}
	return "none"
}

// setActiveTab updates activeTab on the model, the tab bar, the tab registry,
// and the subtab bar consistently.
func (m *RootModel) setActiveTab(id TabID) {
	if m.startupLocked && id != TabDoctor {
		id = TabDoctor
	}
	m.activeTab = id
	m.tabBar.SetActive(id)
	m.tabs.SetActive(id)
	m.subTabBar.SetSubTabs(m.tabs.ActiveTab())
	sub := m.tabs.ActiveSubTab()
	m.inputBar.SetContext(id, sub)
	m.syncCommandPalette()
	filter := ""
	if m.inputBar != nil {
		filter = m.inputBar.Value()
	}
	switch id {
	case TabWelcome:
		if m.welcome != nil {
			m.welcome.Refresh()
			m.welcome.SetFilter(filter)
		}
	case TabSandbox:
		if m.sandbox != nil {
			m.sandbox.SetFilter(filter)
		}
	case TabSecurityGuard:
		if m.securityguard != nil {
			m.securityguard.Refresh()
			m.securityguard.SetFilter(filter)
		}
	case TabAIProvider:
		if m.aiprovider != nil {
			m.aiprovider.Refresh()
			m.aiprovider.SetFilter(filter)
		}
	case TabKeybindings:
		if m.keybindings != nil {
			m.keybindings.Refresh()
			m.keybindings.SetFilter(filter)
		}
	case TabDoctor:
		if m.doctor != nil {
			m.doctor.Refresh()
			m.doctor.SetFilter(filter)
		}
	case TabLibrary:
		if m.library != nil {
			m.library.SetFilter(filter)
		}
	default:
		if m.session != nil {
			m.session.SetFrameworkMode(false)
		}
	}
	if m.chat != nil && (id == TabChat || id == TabGraph || id == TabDiff) {
		m.chat.SetSubTab(sub)
	}
	if tabAware, ok := m.chat.(TabAwarePane); ok && (id == TabChat || id == TabGraph || id == TabDiff) {
		tabAware.SetActiveTab(id)
	}
}

// setActiveSubTab changes the active subtab for the current main tab and
// notifies panes that care about subtab changes.
func (m *RootModel) setActiveSubTab(sub SubTabID) {
	m.tabs.SetSubActive(m.activeTab, sub)
	m.subTabBar.SetActive(sub)
	m.inputBar.SetContext(m.activeTab, sub)
	m.syncCommandPalette()
	if (m.activeTab == TabChat || m.activeTab == TabGraph || m.activeTab == TabDiff) && m.chat != nil {
		m.chat.SetSubTab(sub)
	}
}

// handleGlobalKey processes navigation keys emitted by InputBar when the
// input field is empty.
func (m RootModel) handleGlobalKey(key string) (tea.Model, tea.Cmd) {
	switch {
	case keyMatchesBinding(GlobalKeys.Quit, key):
		return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
	case keyMatchesBinding(GlobalKeys.Tab1, key):
		idx := 0
		id := m.tabs.TabAtIndex(idx)
		if id != "" {
			m.setActiveTab(id)
			m.refreshActivePane()
			return m, nil
		}
	case keyMatchesBinding(GlobalKeys.Tab2, key):
		idx := 1
		id := m.tabs.TabAtIndex(idx)
		if id != "" {
			m.setActiveTab(id)
			m.refreshActivePane()
			return m, nil
		}
	case keyMatchesBinding(GlobalKeys.Tab3, key):
		idx := 2
		id := m.tabs.TabAtIndex(idx)
		if id != "" {
			m.setActiveTab(id)
			m.refreshActivePane()
			return m, nil
		}
	case keyMatchesBinding(GlobalKeys.Tab4, key):
		idx := 3
		id := m.tabs.TabAtIndex(idx)
		if id != "" {
			m.setActiveTab(id)
			m.refreshActivePane()
			return m, nil
		}
	case keyMatchesBinding(GlobalKeys.Tab5, key):
		idx := 4
		id := m.tabs.TabAtIndex(idx)
		if id != "" {
			m.setActiveTab(id)
			m.refreshActivePane()
			return m, nil
		}
	case keyMatchesBinding(GlobalKeys.Tab6, key):
		idx := 5
		id := m.tabs.TabAtIndex(idx)
		if id != "" {
			m.setActiveTab(id)
			m.refreshActivePane()
			return m, nil
		}
	case key == "]":
		m.setActiveSubTab(m.tabs.CycleSubNext())
		return m, m.refreshActiveSurfaceCmd()
	case key == "[":
		m.setActiveSubTab(m.tabs.CycleSubPrev())
		return m, m.refreshActiveSurfaceCmd()
	case keyMatchesBinding(GlobalKeys.FocusRegion1, key):
		m.setFocus(FocusRegionRegion1)
		return m, nil
	case keyMatchesBinding(GlobalKeys.AgentPicker, key):
		m.openAgentPicker()
		return m, nil
	case keyMatchesBinding(GlobalKeys.SearchMode, key):
		m.searchActive = !m.searchActive
		m.inputBar.SetSearchMode(m.searchActive)
		if !m.searchActive && m.chat != nil {
			m.chat.SetSearchFilter("")
		}
		m.syncActivePaneFilter(m.inputBar.Value())
		if !m.searchActive {
			m.setFocus(FocusRegionInput)
		}
	case keyMatchesBinding(GlobalKeys.Help, key):
		m.showHelp = !m.showHelp
	case key == "esc":
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		m.searchActive = false
		m.inputBar.SetSearchMode(false)
		if m.chat != nil {
			m.chat.SetSearchFilter("")
		}
	case keyMatchesBinding(GlobalKeys.Undo, key):
		// Undo: revert to previous feed snapshot
		if m.chat != nil {
			if !m.chat.Undo() {
				m.addSystemMessage("nothing to undo")
			}
		}
	case keyMatchesBinding(GlobalKeys.Redo, key):
		// Redo: restore the next feed snapshot
		if m.chat != nil {
			if !m.chat.Redo() {
				m.addSystemMessage("nothing to redo")
			}
		}
	case keyMatchesBinding(GlobalKeys.ScrollUp, key):
		// Scroll up: scroll the chat feed up
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabGraph || m.activeTab == TabDiff) {
			m.chat.ScrollUp()
		}
	case keyMatchesBinding(GlobalKeys.ScrollDown, key):
		// Page down: scroll the chat feed down by page
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabGraph || m.activeTab == TabDiff) {
			m.chat.PageDown()
		}
	case keyMatchesBinding(GlobalKeys.PageUp, key):
		// Page up: scroll the chat feed up by page
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabGraph || m.activeTab == TabDiff) {
			m.chat.PageUp()
		}
	case keyMatchesBinding(GlobalKeys.FilePicker, key):
		// File picker: enable file selection mode in input
		m.inputBar.SetFilePickerMode(true)
	case keyMatchesBinding(GlobalKeys.SidebarToggle, key):
		// Toggle chat context sidebar
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabGraph || m.activeTab == TabDiff) {
			if sidebar, ok := m.chat.(ChatSidebarController); ok {
				sidebar.ToggleSidebar()
			}
		}
	case keyMatchesBinding(GlobalKeys.Compact, key):
		// Compact: toggle message compactness in chat feed
		if m.chat != nil {
			m.chat.ToggleCompact()
		}
	}
	return m, nil
}

func (m *RootModel) routeFocusKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.inputBar == nil {
		return false, nil
	}
	route := m.focus.Route(msg)
	switch route.Action {
	case FocusActionIgnore:
		return false, nil
	case FocusActionFocusInput:
		m.setFocus(FocusRegionInput)
		return true, nil
	case FocusActionFocusRegion1:
		m.setFocus(FocusRegionRegion1)
		return true, nil
	case FocusActionTypePrintable:
		m.setFocus(FocusRegionInput)
		if route.Printable != "" {
			return m.routeInputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(route.Printable)})
		}
		return true, nil
	case FocusActionRouteRegion1:
		return true, m.routeRegion1Key(msg)
	case FocusActionRouteInput:
		return m.routeInputKey(msg)
	default:
		return false, nil
	}
}

func (m *RootModel) handleMouse(msg tea.MouseMsg) (bool, tea.Cmd) {
	if m == nil || m.overlays == nil {
		return false, nil
	}
	if m.overlays.Len() > 0 {
		return true, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false, nil
	}
	if m.width <= 0 || m.height <= 0 {
		return false, nil
	}
	agentRow := m.height - 2
	if msg.Y != agentRow {
		return false, nil
	}
	if msg.X < 0 || msg.X >= m.layout.Region2Width() {
		return false, nil
	}
	m.openAgentPicker()
	return true, nil
}

func (m *RootModel) routeInputKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	ib, ibCmd := m.inputBar.Update(msg, m.activeTab)
	m.inputBar = ib
	m.syncActivePaneFilter(m.inputBar.Value())
	m.syncCommandPalette()
	m.syncOverlayStack()
	return true, ibCmd
}

func (m *RootModel) routeRegion1Key(msg tea.KeyMsg) tea.Cmd {
	switch m.activeTab {
	case TabWelcome:
		if m.welcome == nil {
			return nil
		}
		wp, cmd := m.welcome.Update(msg)
		m.welcome = wp
		return cmd
	case TabSandbox:
		if m.sandbox == nil {
			return nil
		}
		sp, cmd := m.sandbox.Update(msg)
		m.sandbox = sp
		return cmd
	case TabSecurityGuard:
		if m.securityguard == nil {
			return nil
		}
		cp, cmd := m.securityguard.Update(msg)
		m.securityguard = cp
		return cmd
	case TabAIProvider:
		if m.aiprovider == nil {
			return nil
		}
		sp, cmd := m.aiprovider.Update(msg)
		m.aiprovider = sp
		return cmd
	case TabKeybindings:
		if m.keybindings == nil {
			return nil
		}
		kp, cmd := m.keybindings.Update(msg)
		m.keybindings = kp
		return cmd
	case TabDoctor:
		if m.doctor == nil {
			return nil
		}
		dp, cmd := m.doctor.Update(msg)
		m.doctor = dp
		return cmd
	case TabLibrary:
		if m.library == nil {
			return nil
		}
		lp, cmd := m.library.Update(msg)
		m.library = lp
		return cmd
	default:
		if m.chat == nil {
			return nil
		}
		cp, cmd := m.chat.Update(msg)
		m.chat = cp
		return cmd
	}
}

func (m *RootModel) setFocus(region FocusRegion) {
	m.focus.state.Region = region
	if m.inputBar != nil {
		m.inputBar.SetFocused(region == FocusRegionInput)
	}
}

func (m *RootModel) applyStartupGate() {
	if m == nil || m.doctor == nil {
		return
	}
	report := m.doctor.report
	if report.Ready() {
		m.startupLocked = false
		if err := m.switchActiveAgent("euclo"); err == nil {
			m.setActiveTab(TabChat)
			return
		}
		m.activateSurface("euclo")
		m.activeAgent = "euclo"
		if m.sharedSess != nil {
			m.sharedSess.Agent = "euclo"
		}
		m.setActiveTab(TabChat)
		return
	}
	m.startupLocked = true
	m.activateSurface("none")
	m.activeAgent = "none"
	if m.sharedSess != nil {
		m.sharedSess.Agent = "none"
	}
	m.setActiveTab(TabDoctor)
	m.doctor.status = "startup checks failed; resolve Doctor issues to unlock Euclo chat"
}

func (m *RootModel) applyDoctorReport(report DoctorReport) {
	if m == nil {
		return
	}
	if report.Ready() {
		m.startupLocked = false
		if err := m.switchActiveAgent("euclo"); err != nil {
			m.activateSurface("euclo")
			m.activeAgent = "euclo"
			if m.sharedSess != nil {
				m.sharedSess.Agent = "euclo"
			}
			m.setActiveTab(TabChat)
			m.addSystemMessage("Doctor checks passed; auto-promoted to Euclo chat")
			return
		}
		m.setActiveTab(TabChat)
		m.addSystemMessage("Doctor checks passed; auto-promoted to Euclo chat")
		return
	}
	m.startupLocked = true
	if m.activeAgentName() != "none" {
		_ = m.switchActiveAgent("none")
	} else {
		m.activateSurface("none")
	}
	m.setActiveTab(TabDoctor)
	m.addSystemMessage("Startup checks failed; Doctor tab remains locked")
}

// handleInputSubmitted routes a submitted value to the active pane.
func (m RootModel) handleInputSubmitted(msg InputSubmittedMsg) (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(msg.Value)
	if value == "" {
		if m.cmdPalette != nil {
			m.cmdPalette.Close()
		}
		return m, nil
	}

	if msg.Prefix == ">" {
		switch m.activeTab {
		case TabWelcome, TabSandbox, TabSecurityGuard, TabAIProvider, TabKeybindings, TabDoctor, TabLibrary:
			return m, nil
		}
		if m.chat == nil {
			return m, nil
		}
		cmd := m.chat.HandleInputSubmit(value)
		return m, cmd
	}
	m.syncActivePaneFilter(value)
	switch m.activeTab {
	case TabWelcome, TabSandbox, TabSecurityGuard, TabAIProvider, TabKeybindings, TabDoctor, TabLibrary:
		return m, nil
	default:
		if m.chat == nil {
			return m, nil
		}
		cmd := m.chat.HandleInputSubmit(value)
		return m, cmd
	}
}

func (m RootModel) handleWorkspaceSelected(workspace string) (tea.Model, tea.Cmd) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		m.addSystemMessage("Workspace selection is empty")
		return m, nil
	}
	reloader, ok := m.runtime.(interface {
		ReloadWorkspace(context.Context, string) error
	})
	if !ok || reloader == nil {
		m.addSystemMessage("Workspace reload unavailable")
		return m, nil
	}
	if err := reloader.ReloadWorkspace(context.Background(), workspace); err != nil {
		m.addSystemMessage(fmt.Sprintf("Workspace reload failed: %v", err))
		return m, nil
	}
	if m.sharedSess != nil {
		m.sharedSess.Workspace = workspace
	}
	if m.inputBar != nil {
		m.inputBar.SetWorkspace(workspace)
	}
	m.store = NewSessionStore(workspace)
	if m.welcome != nil {
		m.welcome.store = m.store
		m.welcome.Refresh()
	}
	if m.session != nil {
		m.session.SyncContext(m.sharedCtx)
	}
	if m.chat != nil {
		m.chat.SetSearchFilter("")
	}
	if m.sandbox != nil {
		m.sandbox.Refresh()
	}
	if m.securityguard != nil {
		m.securityguard.Refresh()
	}
	if m.aiprovider != nil {
		m.aiprovider.Refresh()
	}
	if m.keybindings != nil {
		m.keybindings.Refresh()
	}
	if m.library != nil {
		m.library.Refresh()
	}
	if m.doctor != nil {
		m.doctor.Refresh()
	}
	m.applyStartupGate()
	m.syncActivePaneFilter(m.inputBar.Value())
	var cmds []tea.Cmd
	if m.session != nil {
		cmds = append(cmds, m.session.Init())
	}
	if m.chat != nil {
		cmds = append(cmds, m.chat.Init())
	}
	m.addSystemMessage(fmt.Sprintf("Workspace switched to %s", workspace))
	return m, tea.Batch(cmds...)
}

func (m *RootModel) syncActivePaneFilter(raw string) {
	if m == nil {
		return
	}
	draft := parseInputDraft(raw, m.activeTab, m.searchActive)
	filter := ""
	if draft.filterMode && !draft.commandMode && draft.prefix != ">" {
		filter = strings.TrimSpace(draft.current)
	}
	switch m.activeTab {
	case TabWelcome:
		if m.welcome != nil {
			m.welcome.SetFilter(filter)
		}
	case TabSandbox:
		if m.sandbox != nil {
			m.sandbox.SetFilter(filter)
		}
	case TabSecurityGuard:
		if m.securityguard != nil {
			m.securityguard.SetFilter(filter)
		}
	case TabAIProvider:
		if m.aiprovider != nil {
			m.aiprovider.SetFilter(filter)
		}
	case TabKeybindings:
		if m.keybindings != nil {
			m.keybindings.SetFilter(filter)
		}
	case TabDoctor:
		if m.doctor != nil {
			m.doctor.SetFilter(filter)
		}
	case TabLibrary:
		if m.library != nil {
			m.library.SetFilter(filter)
		}
	default:
		if draft.filterMode && m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabGraph || m.activeTab == TabDiff) {
			m.chat.SetSearchFilter(filter)
		}
	}
}

func (m *RootModel) refreshActivePane() {
	if m == nil {
		return
	}
	switch m.activeTab {
	case TabWelcome:
		if m.welcome != nil {
			m.welcome.Refresh()
		}
	case TabSandbox:
		if m.sandbox != nil {
			m.sandbox.Refresh()
		}
	case TabSecurityGuard:
		if m.securityguard != nil {
			m.securityguard.Refresh()
		}
	case TabAIProvider:
		if m.aiprovider != nil {
			m.aiprovider.Refresh()
		}
	case TabKeybindings:
		if m.keybindings != nil {
			m.keybindings.Refresh()
		}
	case TabDoctor:
		if m.doctor != nil {
			m.doctor.Refresh()
		}
	case TabLibrary:
		if m.library != nil {
			m.library.Refresh()
		}
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
		m.chat.AppendMessage(msg)
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
		m.chat.AddSystemMessage(text)
	}
}

func (m *RootModel) trackInteractionFrame(notificationID string, frame interaction.InteractionFrame) {
	if m == nil {
		return
	}
	if m.interactionFrames == nil {
		m.interactionFrames = make(map[string]*interaction.InteractionFrame)
	}
	frameCopy := frame
	if notificationID = strings.TrimSpace(notificationID); notificationID != "" {
		m.interactionFrames[notificationID] = &frameCopy
	}
	if frameID := strings.TrimSpace(frame.ID); frameID != "" {
		m.interactionFrames[frameID] = &frameCopy
	}
}

func (m *RootModel) openInteractionGuidance(notificationID string, frame interaction.InteractionFrame) {
	if m == nil {
		return
	}
	if m.hitlPanel.IsOpen() {
		return
	}
	body := strings.TrimSpace(frame.Question)
	if body == "" {
		body = frameLabelFromInteraction(frame)
	}
	if len(frame.Choices) > 0 {
		var choiceLines []string
		for i, choice := range frame.Choices {
			choiceLines = append(choiceLines, fmt.Sprintf("[%d] %s", i+1, choice))
		}
		if body != "" {
			body += "\n\n"
		}
		body += "Choices: " + strings.Join(choiceLines, "  ")
	}
	if len(frame.Slots) > 0 && len(frame.Choices) == 0 {
		var slotLines []string
		for i, slot := range frame.Slots {
			label := strings.TrimSpace(slot.Label)
			if label == "" {
				label = slot.ID
			}
			slotLines = append(slotLines, fmt.Sprintf("[%d] %s", i+1, label))
		}
		if body != "" {
			body += "\n\n"
		}
		body += "Actions: " + strings.Join(slotLines, "  ")
	}
	m.hitlPanel.Open(
		GuidanceTriggerAmbiguity,
		strings.TrimSpace(notificationID),
		strings.TrimSpace(frameLabelFromInteraction(frame)),
		body,
		nil,
		"",
		"",
	)
	m.syncOverlayStack()
}

func (m *RootModel) resolvePendingInteraction(notificationID, choiceID, freetext string) bool {
	if m == nil {
		return false
	}
	if m.interactionFrames == nil {
		m.interactionFrames = make(map[string]*interaction.InteractionFrame)
	}
	requestID := strings.TrimSpace(notificationID)
	if requestID == "" {
		return false
	}
	frame, ok := m.interactionFrames[requestID]
	if !ok || frame == nil {
		return false
	}
	answer := strings.TrimSpace(choiceID)
	if answer == "" {
		answer = strings.TrimSpace(freetext)
	}
	if answer == "" {
		answer = defaultInteractionAnswer(frame)
	}
	extra := map[string]any{
		"notification_id": requestID,
		"frame_id":        strings.TrimSpace(frame.ID),
		"frame_type":      string(frame.Type),
	}
	if strings.TrimSpace(frame.TaskID) != "" {
		extra["task_id"] = strings.TrimSpace(frame.TaskID)
	}
	if strings.TrimSpace(freetext) != "" {
		extra["freetext"] = strings.TrimSpace(freetext)
	}
	frame.SetResponse(answer, extra, "relurpish", time.Now().UTC())
	delete(m.interactionFrames, requestID)
	if frameID := strings.TrimSpace(frame.ID); frameID != "" {
		delete(m.interactionFrames, frameID)
	}
	if m.notifQ != nil {
		m.notifQ.Resolve(requestID)
	}
	m.syncOverlayStack()
	if m.runtime != nil {
		if err := m.runtime.ResolveInteractionFrame(context.Background(), frame.TaskID, frame.ID, answer, freetext); err != nil {
			m.addSystemMessage(fmt.Sprintf("Interaction persistence failed: %v", err))
		}
	}
	if answer != "" {
		m.addSystemMessage(fmt.Sprintf("Resolved %s: %s", frameLabelFromInteraction(*frame), answer))
	} else {
		m.addSystemMessage(fmt.Sprintf("Resolved %s", frameLabelFromInteraction(*frame)))
	}
	return true
}

func defaultInteractionAnswer(frame *interaction.InteractionFrame) string {
	if frame == nil {
		return ""
	}
	if slot := strings.TrimSpace(frame.DefaultChoice); slot != "" {
		return slot
	}
	for _, slot := range frame.Slots {
		if slot.Default {
			if id := strings.TrimSpace(slot.ID); id != "" {
				return id
			}
		}
	}
	if len(frame.Slots) > 0 {
		return strings.TrimSpace(frame.Slots[0].ID)
	}
	if len(frame.Choices) > 0 {
		return strings.TrimSpace(frame.Choices[0])
	}
	return ""
}

func (m *RootModel) deferPendingInteraction(notificationID string) bool {
	if m == nil {
		return false
	}
	requestID := strings.TrimSpace(notificationID)
	if requestID == "" {
		return false
	}
	frame, ok := m.interactionFrames[requestID]
	if !ok || frame == nil {
		return false
	}
	delete(m.interactionFrames, requestID)
	if frameID := strings.TrimSpace(frame.ID); frameID != "" {
		delete(m.interactionFrames, frameID)
	}
	if m.notifQ != nil {
		m.notifQ.Resolve(requestID)
	}
	m.addSystemMessage(fmt.Sprintf("Deferred %s", frameLabelFromInteraction(*frame)))
	m.syncOverlayStack()
	return true
}

func frameLabelFromInteraction(frame interaction.InteractionFrame) string {
	frameType := strings.TrimSpace(string(frame.Type))
	if frameType == "" {
		frameType = strings.TrimSpace(string(frame.Kind))
	}
	if frameType == "" {
		frameType = "interaction"
	}
	return prettyFrameLabel(frameType)
}

func prettyFrameLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

// autoSave persists the current session after each completed run.
func (m RootModel) autoSave() {
	if m.store == nil || m.chat == nil {
		return
	}
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:        m.sharedSess.ID,
			Agent:     m.activeAgentName(),
			Workspace: m.sharedSess.Workspace,
			UpdatedAt: time.Now(),
		},
		Messages: m.chat.Messages(),
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
	if m.chat != nil {
		if svc := m.chat.HITLService(); svc != nil {
			pending = svc.PendingHITL()
		}
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
	m.session.SyncQueuedTasks(m.tasks.Items())
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
	msgs := m.chat.Messages()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleAgent && len(msgs[i].Content.Changes) > 0 {
			return append([]FileChange(nil), msgs[i].Content.Changes...)
		}
	}
	return nil
}
