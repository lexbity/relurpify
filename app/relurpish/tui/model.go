package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

var reduceMotionPreference bool
var terminalNamePreference string

// SetReduceMotionPreference configures whether new TUI models should reduce motion.
func SetReduceMotionPreference(reduced bool) {
	reduceMotionPreference = reduced
}

// SetTerminalNamePreference configures the terminal name hint used by motion detection.
func SetTerminalNamePreference(name string) {
	terminalNamePreference = strings.TrimSpace(name)
}

// detectReduceMotion checks the terminal and the precomputed preference to
// decide whether animations should be disabled.
func detectReduceMotion(preferred bool) bool {
	if preferred {
		return true
	}
	// 3. Non-interactive / pipe — no terminal to animate on.
	stat, _ := os.Stdout.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return true
	}
	// 4. SSH or remote session — local motion may not render smoothly.
	// 5. Dumb terminal or very limited colour support.
	profile := termenv.EnvColorProfile()
	if profile == termenv.Ascii {
		return true
	}
	term := terminalNamePreference
	if term == "dumb" || term == "" {
		return true
	}
	return false
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
	chat        ChatPaner
	baseSurface Region1Surface
	activeInput InputSurface
	activeNav   NavSurface
	tasks       *TasksPane
	session     *SessionPane

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
	// startupAgent is the agent activated at construction (before any startup
	// gate). The gate parks the active agent at "none" while locked; on unlock
	// it is restored to startupAgent so chat reaches the real agent surface.
	startupAgent string

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

	// HITL notification row: renders between Region 1 and the bottom bar
	// when the agent emits an interaction frame.
	hitlRow *HITLRow

	// Input gate blocks > prompts during active execution.
	inputGate *InputGate

	// Theme is the active semantic style source, threaded to all components.
	th *theme.Theme

	// Animation manager and reduce-motion detector.
	anim   *AnimationManager
	reduce *ReduceMotion

	// Phase G: instance-based command registry and corpus scope.
	cmdReg *CommandRegistry
	scope  string
}

type surfaceState struct {
	surface AgentSurface
	tabs    *TabRegistry
	cmdReg  *CommandRegistry
	chat    ChatPaner
	region1 Region1Surface
	input   InputSurface
	nav     NavSurface
}

// propagateTheme distributes the active theme to all host-owned components.
func (m *RootModel) propagateTheme() {
	if m.th == nil {
		return
	}
	if m.chat != nil {
		if setter, ok := m.chat.(ThemeSetter); ok {
			setter.SetTheme(m.th)
		}
	}
	if m.tasks != nil {
		m.tasks.SetTheme(m.th)
	}
	if m.session != nil {
		m.session.SetTheme(m.th)
	}
	if m.notifBar != nil {
		m.notifBar.SetTheme(m.th)
	}
	if m.inputBar != nil {
		m.inputBar.SetTheme(m.th)
	}
	m.tabBar.SetTheme(m.th)
	if m.cmdPalette != nil {
		m.cmdPalette.SetTheme(m.th)
	}
	if m.hitlRow != nil {
		m.hitlRow.SetTheme(m.th)
	}
	if m.agentPicker != nil {
		m.agentPicker.SetTheme(m.th)
	}
	if m.overlays != nil {
		m.overlays.SetTheme(m.th)
	}
	m.help.SetTheme(m.th)
}

// propagateAnimManager distributes the animation manager to components that
// consume it for frame-by-frame animation updates.
func (m *RootModel) propagateAnimManager() {
	if m.anim == nil {
		return
	}
	if m.chat != nil {
		if setter, ok := m.chat.(AnimSetter); ok {
			setter.SetAnimManager(m.anim)
		}
	}
}

// resolveSurfaceTheme returns the surface's preferred theme or the host default.
func resolveSurfaceTheme(surface AgentSurface) *theme.Theme {
	if surface == nil {
		return theme.Default()
	}
	if t := surface.Theme(); t != nil {
		return t
	}
	return theme.Default()
}

func propagateSurfaceTheme(m *RootModel, surface AgentSurface) {
	m.th = resolveSurfaceTheme(surface)
	if m.chat != nil {
		if setter, ok := m.chat.(ThemeSetter); ok {
			setter.SetTheme(m.th)
		}
	}
	m.propagateTheme()
	m.propagateAnimManager()
}

func isBaseFrameworkTab(id TabID) bool {
	switch id {
	case TabWelcome, TabSandbox, TabSecurityGuard, TabAIProvider, TabKeybindings, TabDoctor:
		return true
	default:
		return false
	}
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
		ExecutionMode: info.ExecutionMode,
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
	var store *SessionStore
	if info.Workspace != "" {
		store = NewSessionStore(info.Workspace)
	}
	initialAgent := normalizeSurfaceKey(info.Agent)
	if initialAgent == "" {
		initialAgent = "none"
	}
	state := buildSurfaceState(factory, initialAgent, rt, ctx, sess, store, notifQ)
	inputBar.SetCommandRegistry(state.cmdReg)
	inputBar.SetContext(state.tabs.ActiveTab().ID, state.tabs.ActiveSubTab())

	tabBar := NewTabBar(state.tabs.ActiveTab().ID)
	tabBar.SetRegistry(state.tabs)

	m := RootModel{
		tabs:              state.tabs,
		subTabBar:         NewSubTabBar(state.tabs.ActiveTab()),
		hitlRow:           &HITLRow{th: theme.Default()},
		inputGate:         &InputGate{},
		tabBar:            tabBar,
		notifBar:          NewNotificationBar(notifQ),
		inputBar:          inputBar,
		cmdPalette:        NewCommandPalette(),
		notifQ:            notifQ,
		overlays:          NewOverlayStack(),
		activeAgent:       initialAgent,
		startupAgent:      initialAgent,
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
		th:                resolveSurfaceTheme(state.surface),
		anim:              NewAnimationManager(),
		reduce:            NewReduceMotion(detectReduceMotion(reduceMotionPreference)),
	}
	m.notifBar.SetInteractionRenderer(state.surface.RenderNotification)
	m.propagateTheme()
	m.propagateAnimManager()
	if state.region1 != nil {
		state.region1.SetStore(store)
		m.baseSurface = state.region1
	}
	m.store = store

	m.chat = state.chat
	if setter, ok := m.chat.(interface{ SetSessionStore(*SessionStore) }); ok && setter != nil {
		setter.SetSessionStore(m.store)
	}
	m.tasks = NewTasksPane(rt, notifQ)
	m.session = NewSessionPane(ctx, sess, rt)
	m.session.SyncQueuedTasks(m.tasks.Items())
	if rt != nil {
		m.applyStartupGate()
	}
	m.setFocus(FocusRegionInput)
	m.setActiveTab(m.activeTab)
	m.syncActivePaneFilter(m.inputBar.Value())

	return m
}

func buildSurfaceState(factory SurfaceFactory, agentName string, rt RuntimeAdapter, ctx *AgentContext, sess *Session, store *SessionStore, notifQ *NotificationQueue) *surfaceState {
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
	region1 := surface.NewRegion1(rt, ctx, sess, store, notifQ)
	input := surface.NewInput(rt, ctx, sess)
	nav := surface.NewNav(rt, ctx, sess)
	return &surfaceState{
		surface: surface,
		tabs:    tabs,
		cmdReg:  cmdReg,
		chat:    chat,
		region1: region1,
		input:   input,
		nav:     nav,
	}
}

func (m *RootModel) activateSurface(agentName string) {
	if m == nil {
		return
	}
	key := normalizeSurfaceKey(agentName)
	if key == "" {
		key = normalizeSurfaceKey(m.activeAgent)
	}
	if key == "" {
		key = "none"
	}
	state := m.surfaceStateFor(key)
	m.activeAgent = key
	m.activeSurface = state.surface
	m.tabs = state.tabs
	m.cmdReg = state.cmdReg
	m.chat = state.chat
	if setter, ok := m.chat.(interface{ SetSessionStore(*SessionStore) }); ok && setter != nil {
		setter.SetSessionStore(m.store)
	}
	m.activeInput = state.input
	m.activeNav = state.nav
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
	if state.region1 != nil {
		state.region1.SetStore(m.store)
		m.baseSurface = state.region1
	}
	m.activeTab = m.tabs.ActiveTab().ID
	m.setActiveTab(m.activeTab)
	propagateSurfaceTheme(m, state.surface)
}

func (m *RootModel) surfaceStateFor(agentName string) *surfaceState {
	if m == nil {
		return nil
	}
	if m.surfaceCache == nil {
		m.surfaceCache = make(map[string]*surfaceState)
	}
	key := normalizeSurfaceKey(agentName)
	if key == "" {
		key = "none"
	}
	state, ok := m.surfaceCache[key]
	if !ok || state == nil {
		state = buildSurfaceState(m.surfaceFactory, key, m.runtime, m.sharedCtx, m.sharedSess, m.store, m.notifQ)
		m.surfaceCache[key] = state
	}
	return state
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
	seen := map[string]struct{}{"none": {}}
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

// Init starts the HITL listener, spinner, text-input blink, and animation tick.
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
	if cmd := m.startupDoctorReportCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if tick := m.anim.TickCmd(); tick != nil {
		cmds = append(cmds, tick)
	}
	return tea.Batch(cmds...)
}

// startupDoctorReportCmd builds the initial doctor report off the UI thread and
// delivers it as a DoctorStatusMsg, so applyDoctorReport runs once at boot and
// unlocks the startup gate when the workspace is healthy. Without this, a fully
// ready workspace boots locked on the Doctor tab until the user manually
// refreshes ('r'): the construction-time gate only sees the empty zero-value
// report. Returns nil when no runtime is wired (degraded/headless construction).
func (m RootModel) startupDoctorReportCmd() tea.Cmd {
	rt := m.runtime
	if rt == nil {
		return nil
	}
	return func() tea.Msg {
		return DoctorStatusMsg{
			Action: "refresh",
			Report: rt.BuildDoctorReport(context.Background()),
		}
	}
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

	case AnimationTickMsg:
		frames := m.anim.Advance()
		for _, fr := range frames {
			// Broadcast frame texts to interested components.
			_ = fr.Text
		}
		return m, m.anim.TickCmd()

	case tea.MouseMsg:
		if handled, cmd := m.handleMouse(msg); handled {
			return m, cmd
		}

	case GlobalKeyMsg:
		return m.handleGlobalKey(msg.Key)

	case tea.KeyMsg:
		// Quit shortcuts bypass everything.
		if isReservedChord(msg) {
			switch {
			case keyMatchesBinding(GlobalKeys.Quit, msg.String()):
				return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
			case keyMatchesBinding(GlobalKeys.Help, msg.String()):
				m.showHelp = !m.showHelp
				return m, nil
			case keyMatchesBinding(GlobalKeys.AgentPicker, msg.String()):
				m.openAgentPicker()
				return m, nil
			}
		}
		ownsInput := m.activeInput != nil
		if m.activeInput != nil {
			if cmd, handled := m.activeInput.HandleKey(msg); handled {
				return m, cmd
			}
		}
		if m.activeNav != nil {
			if cmd, handled := m.activeNav.HandleKey(msg); handled {
				return m, cmd
			}
		}
		if ownsInput {
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
		// HITL Row captures keys when active.
		if m.hitlRow != nil && m.hitlRow.Active() {
			if cmd, handled := m.hitlRow.HandleKey(msg); handled {
				m.syncCommandPalette()
				m.syncOverlayStack()
				return m, cmd
			}
		}

		// Notification bar captures keys when active.
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

	case LibraryRunRequestedMsg:
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
		if msg.Scope == policy.GrantScopePersistent {
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

	case WorkspaceSelectedMsg:
		return m.handleWorkspaceSelected(msg.Workspace)

	case StartSessionMsg:
		if err := m.switchActiveAgent(msg.Agent); err != nil {
			m.addSystemMessage(fmt.Sprintf("Failed to start session: %v", err))
		}
		return m, nil

	case ResumeSessionMsg:
		if msg.SessionID == "" {
			return m, nil
		}
		if m.store == nil || m.runtime == nil {
			m.addSystemMessage("Resume unavailable: no session store or runtime")
			return m, nil
		}
		rec, err := m.store.Load(msg.SessionID)
		if err != nil {
			m.addSystemMessage(fmt.Sprintf("Failed to load session: %v", err))
			return m, nil
		}
		agent := rec.Agent
		if agent == "" {
			agent = m.activeAgentName()
		}
		if err := m.switchActiveAgent(agent); err != nil {
			m.addSystemMessage(fmt.Sprintf("Resume: agent switch failed: %v", err))
			return m, nil
		}
		// Restore transcript onto the active chat pane.
		if m.chat != nil && len(rec.Messages) > 0 {
			m.chat.ClearMessages()
			for _, msg := range rec.Messages {
				m.chat.AppendMessage(msg)
			}
		}
		if len(rec.Messages) > 0 {
			m.addSystemMessage(fmt.Sprintf("Resumed session %s (%d messages)", msg.SessionID, len(rec.Messages)))
		}
		if rec.WorkflowID != "" {
			if err := m.runtime.ResumeSession(context.Background(), rec.WorkflowID); err != nil {
				m.addSystemMessage(fmt.Sprintf("Runtime resume: %v", err))
			}
		}
		m.setActiveTab(TabChat)
		m.setFocus(FocusRegionInput)
		return m, nil

	case OpenDoctorMsg:
		if m.baseSurface != nil {
			m.baseSurface.OpenDoctor()
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
	case ConfigRefreshMsg:
		if m.baseSurface != nil {
			m.baseSurface.Refresh()
		}
		return m, nil

	case DoctorStatusMsg:
		m.applyDoctorReport(msg.Report)
		if msg.Err != nil {
			m.addSystemMessage(fmt.Sprintf("%s failed: %v", msg.Action, msg.Err))
		}
		return m, nil

	case SandboxPersistedMsg:
		if msg.Err != nil {
			m.addSystemMessage(fmt.Sprintf("Sandbox save failed: %v", msg.Err))
			return m, nil
		}
		if m.baseSurface != nil {
			m.baseSurface.Refresh()
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

	// HITL Row responses.
	case HITLRowAnswerMsg:
		if m.resolvePendingInteraction(msg.FrameID, msg.SlotID, "") {
			return m, nil
		}
		return m, nil
	case HITLRowDismissMsg:
		if m.deferPendingInteraction(msg.FrameID) {
			return m, nil
		}
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

	if isBaseFrameworkTab(m.activeTab) && m.baseSurface != nil {
		pane, cmd := m.baseSurface.Update(msg)
		m.baseSurface = pane
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
	if m.inputGate != nil {
		m.inputGate.SetActive(streaming)
	}
	if m.inputBar != nil {
		m.inputBar.SetGated(streaming)
	}

	if m.hitlRow != nil && m.hitlRow.Active() {
		m.hitlRow.SetWidth(m.width)
		parts = append(parts, m.hitlRow.View())
	}

	bottom := lipgloss.JoinHorizontal(
		lipgloss.Top,
		renderAgentCell(m.th, m.activeAgentName(), m.layout.Region2Width()),
		func() string {
			if m.activeInput != nil {
				return m.activeInput.View()
			}
			return m.inputBar.View(m.activeTab, streaming)
		}(),
	)
	parts = append(parts, bottom)
	if m.activeNav != nil {
		parts = append(parts, m.activeNav.View())
	} else {
		parts = append(parts, m.tabBar.View())
	}

	base := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Help overlay sits on top of everything.
	if m.showHelp {
		return m.help.View(base)
	}
	return base
}

func (m RootModel) activePaneView() string {
	if isBaseFrameworkTab(m.activeTab) && m.baseSurface != nil {
		return m.baseSurface.View()
	}
	if m.chat != nil {
		return m.chat.View()
	}
	return ""
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
	switch {
	case isBaseFrameworkTab(id) && m.baseSurface != nil:
		m.baseSurface.SetActiveTab(id)
		m.baseSurface.SetFilter(filter)
	default:
		if m.session != nil {
			m.session.SetFrameworkMode(false)
		}
	}
	if m.chat != nil && (id == TabChat || id == TabDiff) {
		m.chat.SetSubTab(sub)
	}
	if tabAware, ok := m.chat.(TabAwarePane); ok && (id == TabChat || id == TabDiff) {
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
	if (m.activeTab == TabChat || m.activeTab == TabDiff) && m.chat != nil {
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
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabDiff) {
			m.chat.ScrollUp()
		}
	case keyMatchesBinding(GlobalKeys.ScrollDown, key):
		// Page down: scroll the chat feed down by page
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabDiff) {
			m.chat.PageDown()
		}
	case keyMatchesBinding(GlobalKeys.PageUp, key):
		// Page up: scroll the chat feed up by page
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabDiff) {
			m.chat.PageUp()
		}
	case keyMatchesBinding(GlobalKeys.FilePicker, key):
		// File picker: enable file selection mode in input
		m.inputBar.SetFilePickerMode(true)
	case keyMatchesBinding(GlobalKeys.SidebarToggle, key):
		// Toggle chat context sidebar
		if m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabDiff) {
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
	if isBaseFrameworkTab(m.activeTab) && m.baseSurface != nil {
		pane, cmd := m.baseSurface.Update(msg)
		m.baseSurface = pane
		return cmd
	}
	if m.chat == nil {
		return nil
	}
	cp, cmd := m.chat.Update(msg)
	m.chat = cp
	return cmd
}

func (m *RootModel) setFocus(region FocusRegion) {
	m.focus.state.Region = region
	if m.inputBar != nil {
		m.inputBar.SetFocused(region == FocusRegionInput)
	}
}

func (m *RootModel) applyStartupGate() {
	if m == nil {
		return
	}
	report := DoctorReport{}
	if reporter, ok := m.baseSurface.(StartupGateController); ok && reporter != nil {
		report = reporter.DoctorReport()
	}
	if report.Ready() {
		m.startupLocked = false
		if controller, ok := m.baseSurface.(StartupGateController); ok && controller != nil {
			controller.SetDoctorReport(report)
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
	if controller, ok := m.baseSurface.(StartupGateController); ok && controller != nil {
		controller.SetDoctorReport(report)
		controller.SetDoctorStatus("startup checks failed; resolve Doctor issues to unlock guest chat")
	}
}

func (m *RootModel) applyDoctorReport(report DoctorReport) {
	if m == nil {
		return
	}
	if report.Ready() {
		m.startupLocked = false
		if controller, ok := m.baseSurface.(StartupGateController); ok && controller != nil {
			controller.SetDoctorReport(report)
		}
		// The lock path parks the active agent surface at "none"; restore the
		// real agent surface so the Chat tab renders the agent (not the gate).
		// The runtime's active agent was never changed (parking is surface-only),
		// so activateSurface is the correct symmetric inverse — switchActiveAgent
		// would redundantly re-invoke runtime.SwitchAgent and can fail.
		if m.activeAgentName() == "none" && m.startupAgent != "" && m.startupAgent != "none" {
			if m.sharedSess != nil {
				m.sharedSess.Agent = m.startupAgent
			}
			m.activateSurface(m.startupAgent)
		}
		m.setActiveTab(TabChat)
		m.addSystemMessage("Doctor checks passed; startup checks are healthy")
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
	if controller, ok := m.baseSurface.(StartupGateController); ok && controller != nil {
		controller.SetDoctorReport(report)
		controller.SetDoctorStatus("startup checks failed; resolve Doctor issues to unlock guest chat")
	}
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
		if m.inputGate != nil && m.inputGate.Active() {
			m.addSystemMessage("Cannot submit prompt while a run is active. Use /stop to cancel or wait for completion.")
			return m, nil
		}
		switch {
		case isBaseFrameworkTab(m.activeTab):
			return m, nil
		}
		if m.chat == nil {
			return m, nil
		}
		cmd := m.chat.HandleInputSubmit(value)
		return m, cmd
	}
	m.syncActivePaneFilter(value)
	switch {
	case isBaseFrameworkTab(m.activeTab):
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
	if m.baseSurface != nil {
		m.baseSurface.SetStore(m.store)
		m.baseSurface.Refresh()
	}
	if m.chat != nil {
		if setter, ok := m.chat.(interface{ SetSessionStore(*SessionStore) }); ok && setter != nil {
			setter.SetSessionStore(m.store)
		}
	}
	if m.session != nil {
		m.session.SyncContext(m.sharedCtx)
	}
	if m.chat != nil {
		m.chat.SetSearchFilter("")
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
	switch {
	case isBaseFrameworkTab(m.activeTab) && m.baseSurface != nil:
		m.baseSurface.SetFilter(filter)
	default:
		if draft.filterMode && m.chat != nil && (m.activeTab == TabChat || m.activeTab == TabDiff) {
			m.chat.SetSearchFilter(filter)
		}
	}
}

func (m *RootModel) refreshActivePane() {
	if m == nil {
		return
	}
	switch {
	case isBaseFrameworkTab(m.activeTab) && m.baseSurface != nil:
		m.baseSurface.Refresh()
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

// autoSave persists the current session after each completed run.
func (m RootModel) autoSave() {
	if m.store == nil || m.chat == nil {
		return
	}
	workflowID := ""
	if m.runtime != nil {
		workflowID = m.runtime.ActiveWorkflowID()
	}
	mode := ""
	if m.sharedSess != nil {
		mode = strings.TrimSpace(m.sharedSess.Mode)
	}
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:         m.sharedSess.ID,
			Agent:      m.activeAgentName(),
			Workspace:  m.sharedSess.Workspace,
			UpdatedAt:  time.Now(),
			WorkflowID: workflowID,
			Mode:       mode,
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
