package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"codeburg.org/lexbit/relurpify/app/relurpish/theme"
	fauthorization "codeburg.org/lexbit/relurpify/governance/authorization"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/telemetry"
)

var reduceMotionPreference bool   //nolint:gochecknoglobals // UI preference propagated from the environment at boot
var terminalNamePreference string //nolint:gochecknoglobals // UI preference propagated from the environment at boot

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
	chat                ChatPaner
	baseSurface         Region1Surface
	activeInput         InputSurface
	activeNav           NavSurface
	tasks               *TasksPane
	session             *SessionPane
	doctor              *DoctorPane
	workshop            *Workshop
	keys                *KeyMap
	keybindings         *KeybindingPane
	demoScenario        string
	conversation        ConversationSession
	frontendState       FrontendSnapshot
	frontendPending     bool
	decisionChoice      int
	decisionAnswer      string
	decisionEditing     bool
	settingsSelection   int
	configurationReader ConfigurationReader
	configurationEpoch  uint64
	configuration       documentInspector
	services            *ServicesPane
	activity            activityCanvas
	demoObserved        uint64
	provider            *ProviderPane

	// Shared state
	activeTab       TabID
	searchActive    bool
	showHelp        bool
	ready           bool
	width           int
	height          int
	focus           FocusRouter
	activeAgent     string
	agentPicker     *AgentPicker
	startupLocked   bool
	decisionFocused bool
	canvasOffset    int
	helpOffset      int
	helpCommand     string
	circuit         circuitCanvas
	notice          string
	// Session-level state shared across panes
	sharedSess     *Session
	sharedCtx      *AgentContext
	runtime        RuntimeAdapter
	doctorReader   DoctorReader
	store          *SessionStore
	surfaceFactory SurfaceFactory
	surfaceCache   map[string]*surfaceState
	activeSurface  AgentSurface

	// HITL subscription
	hitlCh <-chan fauthorization.HITLEvent

	// Execution event subscription
	execEventCh      <-chan telemetry.Event
	observationEpoch uint64

	// Interaction frames keyed by notification ID and frame ID so the host can
	// resolve slot selections and freetext answers back into the pending frame.
	interactionFrames map[string]*interaction.InteractionFrame

	// Task queue: maps run IDs that originated from the task queue.
	taskRunIDs map[string]bool
	parentCtx  context.Context
	lifetime   *hostLifetime

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

func NewRootModel(parentCtx context.Context, options HostOptions) RootModel {
	rt, factory := options.Runtime, options.SurfaceFactory
	if options.DemoScenario != "" && (rt != nil || factory != nil || options.Conversation == nil) {
		panic("demo requires isolated application ports, no live runtime or surface factory")
	}
	if parentCtx == nil {
		panic("NewRootModel requires a parent context")
	}
	parentCtx, cancel := context.WithCancel(parentCtx)
	lifetime := &hostLifetime{cancel: cancel}
	info := SessionInfo{MaxTokens: 100000}
	if rt != nil {
		info = rt.SessionInfo()
	}
	if info.Workspace == "" {
		info.Workspace = options.Workspace
	}
	if options.DemoScenario != "" {
		info.Agent = "euclo"
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
	state := buildSurfaceState(parentCtx, factory, initialAgent, rt, ctx, sess, store, notifQ)
	lifetime.ownSurface(state)
	inputBar.SetCommandRegistry(state.cmdReg)
	inputBar.SetContext(state.tabs.ActiveTab().ID, state.tabs.ActiveSubTab())

	tabBar := NewTabBar(state.tabs.ActiveTab().ID)
	tabBar.SetRegistry(state.tabs)

	m := RootModel{
		keys:              NewKeyMap(),
		demoScenario:      options.DemoScenario,
		conversation:      options.Conversation,
		parentCtx:         parentCtx,
		lifetime:          lifetime,
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
		agentPicker:       NewAgentPicker(),
		activeTab:         state.tabs.ActiveTab().ID,
		focus:             NewFocusRouter(),
		sharedSess:        sess,
		sharedCtx:         ctx,
		runtime:           rt,
		doctorReader:      rt,
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
	if options.Doctor != nil {
		m.doctorReader = options.Doctor
	}
	initializer := options.Initializer
	if initializer == nil && rt != nil {
		initializer = rt
	}
	m.doctor = NewDoctorPane(parentCtx, m.doctorReader, initializer)
	m.doctor.demo = options.DemoScenario != ""
	m.workshop = NewWorkshop(parentCtx, options.Recipes)
	m.workshop.demo = options.DemoScenario != ""
	m.keybindings = NewKeybindingPane(m.keys, info.Workspace, options.DemoScenario != "")
	m.configurationReader = options.Configuration
	if m.configurationReader == nil {
		m.configurationReader, _ = rt.(ConfigurationReader)
	}
	servicePort := options.Services
	if servicePort == nil {
		servicePort, _ = rt.(ServicePort)
	}
	m.services = NewServicesPane(parentCtx, servicePort, options.DemoScenario != "")
	m.services.workspace = info.Workspace
	m.activity.demo = options.DemoScenario != ""
	if options.Providers != nil {
		activator, _ := options.Providers.(ProviderActivationPort)
		m.provider = NewProviderPane(parentCtx, options.Providers, activator, options.DemoScenario != "")
	}
	if options.Recipes != nil {
		registerWorkshopCommands(m.cmdReg)
		m.tabs.Register(TabDefinition{ID: TabWorkshop, Label: "workshop"})
	}
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
	if options.DemoScenario != "" {
		m.applyFrontend(options.Conversation.Current())
		m.cmdReg.retain("ask", "chat", "run", "inspect", "attention", "stop", "doctor", "help", "keybindings", "settings", "config", "services", "service", "activity", "model-provider", "model", "save-model")
		if options.Recipes != nil {
			registerWorkshopCommands(m.cmdReg)
		}
		m.setActiveTab(TabChat)
		if options.DemoScenario == "first-launch" || strings.HasPrefix(options.DemoScenario, "doctor-") {
			m.setActiveTab(TabDoctor)
			m.startupLocked = true
		}
	}
	m.syncActivePaneFilter(m.inputBar.Value())

	return m
}

func buildSurfaceState(parentCtx context.Context, factory SurfaceFactory, agentName string, rt RuntimeAdapter, ctx *AgentContext, sess *Session, store *SessionStore, notifQ *NotificationQueue) *surfaceState {
	surface := factory.Resolve(agentName)
	if surface == nil {
		surface = newGenericSurface()
	}
	tabs := NewTabRegistry()
	surface.RegisterTabs(tabs)
	tabs.Register(TabDefinition{ID: TabRun, Label: "run"})
	tabs.Register(TabDefinition{ID: TabSettings, Label: "settings"})
	tabs.Register(TabDefinition{ID: TabConfiguration, Label: "configuration"})
	tabs.Register(TabDefinition{ID: TabServices, Label: string(TabServices)})
	tabs.Register(TabDefinition{ID: TabActivity, Label: "activity"})
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
	cmdReg := newHostCommandRegistry()
	surface.RegisterCommands(cmdReg)
	chat := surface.NewChat(parentCtx, rt, ctx, sess, notifQ)
	if tabAware, ok := chat.(TabAwarePane); ok {
		tabAware.SetActiveTab(initialTab)
	}
	region1 := surface.NewRegion1(rt, ctx, sess, store, notifQ)
	if base := factory.Resolve("none"); base != nil && surface.Name() != "none" {
		base.RegisterTabs(tabs)
		region1 = base.NewRegion1(rt, ctx, sess, store, notifQ)
	}
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
	m.refreshCircuit()
	m.setActiveTab(m.activeTab)
	propagateSurfaceTheme(m, state.surface)
	// Surfaces are built lazily — after the boot's initial WindowSizeMsg a
	// late-activated pane has seen no size at all and would render at
	// width 0 (the dead "terminal too narrow" feed). Re-push the current
	// terminal dimensions to the freshly activated surface.
	if m.width > 0 && m.height > 0 {
		updated, _ := m.handleResize(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		if rm, ok := updated.(RootModel); ok {
			*m = rm
		}
	}
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
		state = buildSurfaceState(m.parentCtx, m.surfaceFactory, key, m.runtime, m.sharedCtx, m.sharedSess, m.store, m.notifQ)
		m.lifetime.ownSurface(state)
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
	epoch uint64
	ch    <-chan fauthorization.HITLEvent
	unsub func()
}

// execEventsSubscribedMsg carries the execution event subscription info.
type execEventsSubscribedMsg struct {
	epoch uint64
	ch    <-chan telemetry.Event
	unsub func()
}

// ExecEventBatchMsg delivers a batch of telemetry events for live projection.
type ExecEventBatchMsg struct {
	epoch  uint64
	source <-chan telemetry.Event
	Events []telemetry.Event
}

// Init starts the HITL listener, spinner, text-input blink, and animation tick.
func (m RootModel) Init() tea.Cmd {
	if m.demoScenario != "" {
		return tea.Batch(textarea.Blink, m.nextFrontendCmd(), m.startupDoctorReportCmd())
	}
	cmds := []tea.Cmd{
		textarea.Blink,
		m.session.Init(),
		m.restorePromptCmd(),
		m.subscribeHITLCmd(),
		m.subscribeExecEventsCmd(),
		m.refreshBackendHealthCmd(),
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

// backendHealthMsg carries the post-first-paint backend health result.
type backendHealthMsg struct{ state string }

// refreshBackendHealthCmd probes the model backend off the UI thread after
// first paint (NFR-7: the boot path never blocks on the backend). The probe
// is bounded inside the adapter; the initial session snapshot carries the
// "checking" state until this lands.
func (m RootModel) refreshBackendHealthCmd() tea.Cmd {
	rt := m.runtime
	if rt == nil {
		return nil
	}
	return func() tea.Msg {
		return backendHealthMsg{state: rt.ProbeBackendHealth(context.Background())}
	}
}

// startupDoctorReportCmd builds the initial doctor report off the UI thread and
// delivers it as a DoctorStatusMsg, so applyDoctorReport runs once at boot and
// unlocks the startup gate when the workspace is healthy. Without this, a fully
// ready workspace boots locked on the Doctor tab until the user manually
// refreshes ('r'): the construction-time gate only sees the empty zero-value
// report. Returns nil when no runtime is wired (degraded/headless construction).
// DoctorReader is a read-only bootstrap dependency, independent of execution.
type DoctorReader interface {
	BuildDoctorReport(context.Context) runtimesvc.DoctorReport
}

func (m RootModel) startupDoctorReportCmd() tea.Cmd {
	reader := m.doctorReader
	if reader == nil {
		return nil
	}
	initializer, _ := reader.(DoctorOperations)
	m.doctor.SetReader(reader, initializer)
	return m.doctor.run(doctorActionRefresh)
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
	epoch := m.observationEpoch
	rt := m.runtime
	return func() tea.Msg {
		if rt == nil {
			return nil
		}
		ch, unsub := rt.SubscribeHITL()
		return hitlSubscribedMsg{ch: ch, unsub: unsub, epoch: epoch}
	}
}

// subscribeExecEventsCmd subscribes to execution telemetry events.
func (m RootModel) subscribeExecEventsCmd() tea.Cmd {
	epoch := m.observationEpoch
	rt := m.runtime
	return func() tea.Msg {
		if rt == nil {
			return nil
		}
		ch, unsub := rt.SubscribeExecutionEvents()
		return execEventsSubscribedMsg{ch: ch, unsub: unsub, epoch: epoch}
	}
}

// Update is the central message router.
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case configurationLoadedMsg:
		if msg.Epoch != m.configurationEpoch {
			return m, nil
		}
		if msg.Err != nil {
			m.notice = "Configuration inspection failed: " + msg.Err.Error()
		} else {
			m.configuration.apply(msg.Document)
		}
		return m, nil
	case servicesResultMsg:
		m.services.apply(msg)
		return m, nil
	case ProviderModelsLoadedMsg:
		if m.baseSurface != nil {
			pane, cmd := m.baseSurface.Update(msg)
			m.baseSurface = pane
			return m, cmd
		}
		return m, nil
	case providerResultMsg:
		if m.provider != nil {
			m.provider.apply(msg)
		}
		return m, nil
	case keybindingsSavedMsg:
		_, cmd := m.keybindings.Update(msg)
		return m, cmd
	case workshopResultMsg:
		if msg.Epoch != m.workshop.epoch {
			return m, nil
		}
		m.workshop.apply(msg)
		if msg.Err == nil && msg.Operation == "trigger" {
			if m.conversation != nil {
				m.applyFrontend(m.conversation.Current())
			}
			m.refreshCircuit()
			m.setActiveTab(TabRun)
			m.addSystemMessage(fmt.Sprintf("Recipe triggered: %s", msg.RunResult.Status))
		}
		return m, nil
	case frontendSnapshotMsg:
		next, cmd := m.handleFrontendSnapshot(msg)
		result := next.(RootModel)
		if msg.Observed && msg.Err == nil {
			return result, result.nextFrontendCmd()
		}
		return result, cmd
	case frontendSubmitMsg:
		m.frontendPending = false
		if msg.Err != nil {
			m.notice = "Request kept as a draft: " + msg.Err.Error()
			return m, nil
		}
		m.inputBar.Accept(msg.Draft)
		m.notice = ""
		m.applyFrontend(msg.Snapshot)
		return m, nil

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
		// Rune batches are text, even when their spelling equals a key name.
		// Route single runes so widget key maps cannot interpret "delete" as Delete.
		if msg.Type == tea.KeyRunes && !msg.Paste && len(msg.Runes) > 1 {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = next.(RootModel)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			return m, tea.Batch(cmds...)
		}
		if msg.Paste && m.focus.State().InInput() {
			_, cmd := m.routeInputKey(msg)
			return m, cmd
		}
		// Ctrl+Q is immutable and cannot be rebound or captured by a form.
		if msg.Type == tea.KeyCtrlQ || (isReservedChord(msg, m.keys) && keyMatchesBinding(m.keys.Quit, msg.String())) {
			return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
		}
		formOwnsKeys := m.activeTab == TabWorkshop && m.focus.State().InRegion1() && m.workshop.Editing()
		formOwnsKeys = formOwnsKeys || (m.activeTab == TabKeybindings && m.focus.State().InRegion1() && m.keybindings.OwnsEscape())
		formOwnsKeys = formOwnsKeys || (m.provider != nil && m.activeTab == TabAIProvider && m.focus.State().InRegion1() && m.provider.ownsEscape())
		if owner, ok := m.baseSurface.(EscapeOwner); ok && m.focus.State().InRegion1() && isBaseFrameworkTab(m.activeTab) {
			formOwnsKeys = formOwnsKeys || owner.OwnsEscape()
		}
		if isReservedChord(msg, m.keys) && (!formOwnsKeys || m.showHelp) {
			switch {
			case keyMatchesBinding(m.keys.Help, msg.String()):
				m.showHelp = !m.showHelp
				return m, nil
			case keyMatchesBinding(m.keys.AgentPicker, msg.String()):
				m.openAgentPicker()
				return m, nil
			}
		}
		if keyMsgMatchesBinding(msg, m.keys.FocusToggle) && !m.decisionFocused && !(m.activeTab == TabKeybindings && m.keybindings.OwnsEscape() && m.focus.State().InRegion1()) {
			if m.focus.State().InInput() {
				m.setFocus(FocusRegionRegion1)
			} else {
				m.setFocus(FocusRegionInput)
			}
			return m, nil
		}
		if !formOwnsKeys && !m.decisionFocused && !m.showHelp {
			// Focus can leave an editor without applying or discarding its field.
			if msg.Type == tea.KeyEsc && m.inputBar.CancelCommands() {
				m.syncCommandPalette()
				return m, nil
			}
			if keyMsgMatchesBinding(msg, m.keys.Commands) {
				m.inputBar.OpenCommands()
				m.setFocus(FocusRegionInput)
				m.syncCommandPalette()
				return m, nil
			}
			if m.focus.State().InInput() && keyMsgMatchesBinding(msg, m.keys.FocusRegion1) && !m.cmdPalette.IsOpen() {
				m.setFocus(FocusRegionRegion1)
				return m, nil
			}
			if m.focus.State().InRegion1() && (keyMsgMatchesBinding(msg, m.keys.Undo) || keyMsgMatchesBinding(msg, m.keys.Redo)) {
				return m.handleGlobalKey(msg.String())
			}
		}
		if m.activeTab == TabKeybindings && m.focus.State().InRegion1() && m.keybindings.OwnsEscape() && !m.showHelp && !m.decisionFocused {
			_, cmd := m.keybindings.Update(msg)
			return m, cmd
		}
		if m.activeTab == TabWorkshop && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused {
			if cmd, handled := m.workshop.key(msg); handled {
				return m, cmd
			}
		}
		if m.activeTab == TabSettings && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused {
			if cmd, handled := m.settingsKey(msg); handled {
				return m, cmd
			}
		}
		if m.provider != nil && m.activeTab == TabAIProvider && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused {
			if cmd, handled := m.provider.key(msg); handled || msg.Type != tea.KeyEsc {
				return m, cmd
			}
		}
		if m.activeTab == TabConfiguration && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused && m.configuration.key(msg) {
			return m, nil
		}
		if m.activeTab == TabActivity && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused && m.activity.inspector.key(msg) {
			return m, nil
		}
		if m.activeTab == TabServices && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused {
			if cmd, handled := m.services.key(msg); handled {
				return m, cmd
			}
		}
		if m.activeTab == TabRun && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused && m.circuit.key(msg) {
			return m, nil
		}
		if m.activeTab == TabChat && !m.showHelp && (keyMsgMatchesBinding(msg, m.keys.PageUp) || keyMsgMatchesBinding(msg, m.keys.ScrollDown)) {
			rows := max(1, m.height-6)
			end := max(0, len(m.chatCanvasLines())-rows)
			start := end
			if m.canvasOffset > 0 {
				start = m.canvasOffset - 1
			}
			if keyMsgMatchesBinding(msg, m.keys.PageUp) {
				start = max(0, start-rows)
			} else {
				start = min(end, start+rows)
			}
			if start == end {
				m.canvasOffset = 0
			} else {
				m.canvasOffset = start + 1
			}
			return m, nil
		}
		if m.showHelp && (msg.String() == "down" || msg.String() == "up" || msg.String() == "pgdown" || msg.String() == "pgup") {
			delta := 1
			if msg.String() == "up" || msg.String() == "pgup" {
				delta = -1
			}
			m.helpOffset = max(0, m.helpOffset+delta)
			return m, nil
		}
		if m.focus.State().InRegion1() && m.activeTab == TabDoctor && msg.String() == "r" && m.doctorReader != nil {
			return m, m.startupDoctorReportCmd()
		}
		if m.activeTab == TabKeybindings && m.focus.State().InRegion1() && !m.showHelp && !m.decisionFocused && msg.String() != "esc" && !keyMsgMatchesBinding(msg, m.keys.Attention) {
			_, cmd := m.keybindings.Update(msg)
			return m, cmd
		}
		if m.focus.State().InRegion1() && m.activeTab == TabDoctor && !m.showHelp && !m.decisionFocused && (msg.String() != "esc" || m.doctor.OwnsEscape()) && !keyMsgMatchesBinding(msg, m.keys.Attention) {
			_, cmd := m.doctor.Update(msg)
			return m, cmd
		}
		ownsEscape := false
		if owner, ok := m.baseSurface.(EscapeOwner); ok {
			ownsEscape = owner.OwnsEscape()
		}
		if m.focus.State().InRegion1() && isBaseFrameworkTab(m.activeTab) && !m.showHelp && !m.decisionFocused && (msg.String() != "esc" || ownsEscape) && !keyMsgMatchesBinding(msg, m.keys.Attention) {
			return m, m.routeRegion1Key(msg)
		}
		if msg.String() == "esc" && m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if keyMsgMatchesBinding(msg, m.keys.Attention) {
			m.decisionFocused = true
			return m, nil
		}
		if m.decisionFocused {
			if m.conversation != nil && m.frontendState.Decision != nil {
				return m, m.frontendDecisionKey(msg)
			}
			if msg.String() == "esc" {
				m.decisionFocused = false
				return m, nil
			}
			if m.hitlRow != nil && m.hitlRow.Active() {
				if cmd, handled := m.hitlRow.HandleKey(msg); handled {
					return m, cmd
				}
			}
			if m.notifBar != nil && m.notifBar.Active() {
				nb, cmd := m.notifBar.Update(msg)
				m.notifBar = nb
				if cmd != nil {
					m.decisionFocused = false
				}
				return m, cmd
			}
			m.decisionFocused = false
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
		if handled, cmd := m.routeFocusKey(msg); handled {
			return m, cmd
		}
		// Route to input bar first.
		if m.inputBar == nil {
			return m, nil
		}
		ib, ibCmd := m.inputBar.Update(msg, m.activeTab)
		m.inputBar = ib
		m.syncCommandPalette()
		return m, ibCmd

	case InputRejectedMsg:
		m.addSystemMessage(msg.Reason)
		return m, nil

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
		if msg.Name == "ask" {
			next, cmd := rootHandleLiteralRequest(&m, msg.Args)
			if cmd != nil && m.conversation != nil {
				operation := cmd
				cmd = func() tea.Msg {
					result := operation().(frontendSubmitMsg)
					result.Draft = msg.Draft
					return result
				}
				return *next, cmd
			}
			if cmd != nil && next.inputBar != nil {
				next.inputBar.Accept(msg.Draft)
			}
			return *next, cmd
		}
		if m.cmdReg != nil {
			if command, ok := m.cmdReg.Lookup(msg.Name); ok && m.cmdReg.eligible(command, m.activeTab, m.tabs.ActiveSubTab()) && m.inputBar != nil {
				m.inputBar.Accept(msg.Draft)
				m.notice = ""
			}
		}
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
	// RunFinishedMsg is the single terminal of a chat run: the pane renders
	// it, the host does its post-run bookkeeping, and the task queue
	// advances here and only here — including after errored runs.
	case RunFinishedMsg:
		next, paneCmd := m.routeToActivePanes(msg)
		m2, ok := next.(RootModel)
		if !ok {
			m2 = m
		}
		m2.autoSave(msg.WorkflowID)
		m2.session.SyncChanges(m2.latestChanges())
		m2.session.SyncContext(m2.sharedCtx)
		if m2.taskRunIDs[msg.RunID] {
			m2.tasks.MarkComplete(msg.RunID)
			m2.session.SyncQueuedTasks(m2.tasks.Items())
			delete(m2.taskRunIDs, msg.RunID)
		}
		return m2, tea.Batch(paneCmd, m2.dequeueNextTask())

	case backendHealthMsg:
		if m.sharedSess != nil {
			m.sharedSess.BackendState = msg.state
		}
		return m, nil

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
		// Resume never queue-jumps an active run.
		if m.chat != nil && m.chat.HasActiveRuns() {
			m.addSystemMessage("a run is active; resume refused")
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
		m.addSystemMessage(fmt.Sprintf("Resumed from transcript %s (%d messages)", msg.SessionID, len(rec.Messages)))
		m.setActiveTab(TabChat)
		m.setFocus(FocusRegionInput)
		if rec.WorkflowID == "" {
			// Transcript-only record: the runtime has no prior task to
			// re-ground from, so resume stops at the restored transcript
			// until the user sends a follow-up.
			return m, nil
		}
		// The continuation turn executes off the UI thread; its completion
		// is reported through resumeFinishedMsg.
		workflowID := rec.WorkflowID
		rt := m.runtime
		return m, func() tea.Msg {
			err := rt.ResumeSession(context.Background(), workflowID, "")
			return resumeFinishedMsg{err: err}
		}

	case resumeFinishedMsg:
		if msg.err != nil {
			m.addSystemMessage(fmt.Sprintf("Runtime resume: %v", msg.err))
			return m, nil
		}
		m.addSystemMessage("Continuation turn finished")
		m.autoSave("")
		return m, nil

	case OpenDoctorMsg:
		if m.baseSurface != nil {
			m.baseSurface.OpenDoctor()
		}
		return m, nil

	// HITL subscription initialization.
	case hitlSubscribedMsg:
		if msg.epoch != m.observationEpoch {
			if msg.unsub != nil {
				msg.unsub()
			}
			return m, nil
		}
		m.hitlCh = msg.ch
		m.lifetime.replace("hitl", msg.unsub)
		if m.hitlCh != nil {
			return m, listenHITLEvents(m.hitlCh, m.observationEpoch)
		}
		return m, nil

	case execEventsSubscribedMsg:
		if msg.epoch != m.observationEpoch {
			if msg.unsub != nil {
				msg.unsub()
			}
			return m, nil
		}
		m.execEventCh = msg.ch
		m.lifetime.replace("execution", msg.unsub)
		if m.execEventCh != nil {
			return m, listenExecEvents(m.execEventCh, m.observationEpoch)
		}
		return m, nil

	case ExecEventBatchMsg:
		return m.handleExecEventBatch(msg)

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
	case doctorStatusMsg:
		_, cmd := m.doctor.Update(msg)
		if cmd != nil {
			return m.Update(cmd())
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
	case ChatSystemMsg:
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
			m.activeSurface.HandleFrame(m.parentCtx, &m, msg)
			m.refreshCircuit()
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

	case CompactResultMsg:
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

// View renders one canvas and a persistent universal bar. It never updates gates,
// focus or widget state; repeated renders of one model are observationally pure.
func (m RootModel) View() string {
	if !m.ready {
		return "Initializing..."
	}
	if m.width < 1 || m.height < 1 {
		return ""
	}
	if m.height < 6 {
		if m.demoScenario != "" {
			return boundedScreen("DEMO\n"+m.inputBar.View(m.activeTab, false), m.width, m.height)
		}
		return boundedScreen(m.inputBar.View(m.activeTab, false), m.width, m.height)
	}
	state := m.InteractionState()
	label := string(state.Canvas)
	header := m.th.Dim().Render("relurpish · " + terminalText(state.Agent) + " / " + label)
	if m.demoScenario != "" {
		header = m.th.Dim().Render("DEMO · relurpish / " + label + " · " + terminalText(m.demoScenario))
	}
	body := terminalText(m.activePaneView())
	if m.activeTab == TabChat {
		body = strings.Join(m.chatCanvasLines(), "\n")
	}
	if m.activeTab == TabRun {
		body = m.circuit.view(m.width, max(1, m.height-5), m.focus.State().InRegion1())
	}
	if m.activeTab == TabWorkshop {
		body = m.workshop.View()
	}
	if m.showHelp {
		body = m.helpView()
	}
	status := "ready · /help · Ctrl+G attention"
	if m.chat == nil {
		status = "execution unavailable · /doctor · /help"
	}
	if m.conversation != nil {
		status = m.frontendState.Status
	}
	if m.startupLocked {
		status = "setup needed · /doctor · your request stays a draft"
	}
	if m.chat != nil && m.chat.HasActiveRuns() {
		status = "working · /stop cancels · keep reading or composing"
	}
	if m.notice != "" {
		status = terminalText(m.notice)
	}
	if m.notifQ != nil && m.notifQ.Len() > 0 {
		status = fmt.Sprintf("%d attention item(s) · Ctrl+G to open deliberately", m.notifQ.Len())
	}
	if m.frontendState.Decision != nil {
		status = "Decision waiting · Ctrl+G opens it · typing stays in your request"
	}
	if m.decisionFocused {
		status = "decision focused · Escape returns to your draft"
		if m.hitlRow != nil && m.hitlRow.Active() {
			body = m.hitlRow.View()
		} else if m.notifBar != nil {
			body = m.notifBar.View()
		}
		if m.conversation != nil && m.frontendState.Decision != nil {
			body = m.frontendDecisionView()
		}
	}
	if m.cmdPalette != nil && m.cmdPalette.IsOpen() {
		body = m.cmdPalette.View()
	}
	if m.agentPicker != nil && m.agentPicker.IsOpen() {
		body = m.agentPicker.Render(m.width, max(1, m.height-5))
	}
	bar := m.inputBar.View(m.activeTab, m.chat != nil && m.chat.HasActiveRuns())
	rows := max(0, m.height-2-lipgloss.Height(bar))
	lines := strings.Split(body, "\n")
	start := min(max(0, m.canvasOffset-1), max(0, len(lines)-rows))
	if m.activeTab == TabChat && m.canvasOffset == 0 && len(lines) > rows && !m.showHelp {
		start = len(lines) - rows
	}
	content := boundedScreen(strings.Join(lines[start:], "\n"), m.width, rows)
	for lipgloss.Height(content) < rows {
		content += "\n"
	}
	return boundedScreen(strings.Join([]string{header, content, m.th.Dim().Render(status), bar}, "\n"), m.width, m.height)
}

func (m RootModel) activePaneView() string {
	if m.provider != nil && m.activeTab == TabAIProvider {
		return m.provider.view()
	}
	if m.activeTab == TabActivity {
		return m.activity.inspector.view(m.width, max(1, m.height-5))
	}
	if m.activeTab == TabConfiguration {
		return m.configuration.view(m.width, max(1, m.height-5))
	}
	if m.activeTab == TabServices {
		return m.services.view(m.width, max(1, m.height-5))
	}
	if m.activeTab == TabSettings {
		return m.settingsView()
	}
	if m.activeTab == TabKeybindings {
		return m.keybindings.View()
	}
	if m.activeTab == TabDoctor {
		return m.doctor.View()
	}
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
	if m.activeTab != id {
		m.canvasOffset = 0
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

// handleGlobalKey resolves delivered host actions through this host's map.
func (m RootModel) handleGlobalKey(chord string) (tea.Model, tea.Cmd) {
	switch {
	case keyMatchesBinding(m.keys.Quit, chord), chord == "ctrl+q":
		return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
	case keyMatchesBinding(m.keys.Help, chord):
		m.showHelp = !m.showHelp
		m.helpOffset = 0
		m.helpCommand = ""
	case keyMatchesBinding(m.keys.AgentPicker, chord):
		m.openAgentPicker()
	case keyMatchesBinding(m.keys.FocusRegion1, chord):
		m.setFocus(FocusRegionRegion1)
	case keyMatchesBinding(m.keys.FocusToggle, chord):
		if m.focus.State().InInput() {
			m.setFocus(FocusRegionRegion1)
		} else {
			m.setFocus(FocusRegionInput)
		}
	case keyMatchesBinding(m.keys.Attention, chord):
		m.decisionFocused = true
	case keyMatchesBinding(m.keys.Undo, chord):
		if m.activeTab == TabWorkshop {
			m.workshop.history(false)
		} else if m.chat != nil && !m.chat.Undo() {
			m.addSystemMessage("Nothing to undo")
		}
	case keyMatchesBinding(m.keys.Redo, chord):
		if m.activeTab == TabWorkshop {
			m.workshop.history(true)
		} else if m.chat != nil && !m.chat.Redo() {
			m.addSystemMessage("Nothing to redo")
		}
	case chord == "esc":
		m.searchActive = false
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
	report := m.doctor.Report()
	m.startupLocked = !report.Ready()
	if m.startupLocked {
		m.setActiveTab(TabDoctor)
	}
}

func (m *RootModel) applyDoctorReport(report runtimesvc.DoctorReport) {
	if m == nil {
		return
	}
	wasLocked := m.startupLocked
	m.startupLocked = !report.Ready()
	m.doctor.SetReport(report)
	if wasLocked && !m.startupLocked {
		m.addSystemMessage("Doctor checks passed. Submit your preserved draft when ready.")
		if postures := doctorPostureSummary(report); postures != "" {
			m.addSystemMessage(postures)
		}
	}
}

// handleInputSubmitted routes a submitted value to the active pane.
func (m RootModel) handleInputSubmitted(msg InputSubmittedMsg) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(msg.Value) == "" {
		return m, nil
	}
	if m.conversation != nil {
		return m.submitFrontend(msg.Value)
	}
	if m.startupLocked || m.chat == nil {
		m.addSystemMessage("Request kept as a draft: execution is unavailable. Open /doctor.")
		return m, nil
	}
	if (m.inputGate != nil && m.inputGate.Active()) || m.chat.HasActiveRuns() {
		m.addSystemMessage("Request kept as a draft while work is running. Use /stop to cancel.")
		return m, nil
	}
	cmd := m.chat.HandleInputSubmit(msg.Value)
	if cmd != nil && m.inputBar != nil {
		m.inputBar.Accept(msg.Value)
		m.notice = ""
	}
	return m, cmd
}

func (m RootModel) handleWorkspaceSelected(workspace string) (tea.Model, tea.Cmd) {
	if m.chat != nil && m.chat.HasActiveRuns() {
		m.addSystemMessage("Stop or finish the active execution before switching workspace")
		return m, nil
	}

	if m.provider != nil && (m.provider.working || m.provider.dirty()) {
		m.addSystemMessage("Save or discard the provider draft and finish pending operations before switching workspace")
		return m, nil
	}

	if m.services.working {
		m.addSystemMessage("Wait for the pending service operation before switching workspace")
		return m, nil
	}
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
	if err := reloader.ReloadWorkspace(m.parentCtx, workspace); err != nil {
		m.addSystemMessage(fmt.Sprintf("Workspace reload failed: %v", err))
		return m, nil
	}
	if canonical := m.runtime.SessionInfo().Workspace; canonical != "" {
		workspace = canonical
	}
	m.configurationEpoch++
	m.configuration = documentInspector{}
	if m.provider != nil {
		previous := m.provider
		m.provider = NewProviderPane(m.parentCtx, previous.port, previous.activator, m.demoScenario != "")
		m.provider.resize(m.width, max(1, m.height-5))
	}
	m.activity = activityCanvas{demo: m.demoScenario != ""}
	m.observationEpoch++
	m.execEventCh = nil
	m.hitlCh = nil
	m.lifetime.replace("execution", nil)
	m.lifetime.replace("hitl", nil)
	m.notifQ.Clear()
	m.hitlRow.Close()
	m.decisionFocused = false
	m.interactionFrames = make(map[string]*interaction.InteractionFrame)
	m.circuit = circuitCanvas{}

	m.services.epoch++
	m.services.workspace = workspace
	m.services.items = nil
	m.services.review = nil
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
	cmds = append(cmds, m.subscribeExecEventsCmd(), m.subscribeHITLCmd())
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
	if isBaseFrameworkTab(m.activeTab) && m.baseSurface != nil {
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
	if m.chat != nil {
		m.chat.ClearMessages()
		for _, msg := range rec.Messages {
			m.chat.AppendMessage(msg)
		}
	}
	if rec.Context != nil {
		m.sharedCtx.Files = rec.Context.Files
	}
	m.addSystemMessage(fmt.Sprintf("Restored session %s (%d messages)", id, len(rec.Messages)))
	return m, nil
}

// addSystemMessage adds a system line to the chat feed.
func (m *RootModel) addSystemMessage(text string) {
	m.notice = text
	if m.chat != nil {
		m.chat.AddSystemMessage(text)
	}
}

// autoSave persists the current session after each completed run. The
// workflow ID comes from the finished run's terminal message; when absent
// (transcript-only session) the previously stored record's ID is preserved
// so a resumable session never loses its workflow linkage.
func (m RootModel) autoSave(workflowID string) {
	if m.store == nil || m.chat == nil {
		return
	}
	if workflowID == "" && m.runtime != nil {
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
	if workflowID == "" {
		if prev, err := m.store.Load(rec.ID); err == nil {
			rec.WorkflowID = prev.WorkflowID
		}
	}
	_ = m.store.Save(rec) // fire-and-forget; errors are silently dropped
}

// cleanup cancels all runs and unsubscribes HITL.
func (m RootModel) cleanup() {
	if m.lifetime != nil {
		m.lifetime.close()
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

// doctorPostureSummary condenses the doctor's posture rows into one boot
// message: who answers approvals and who disambiguates intent. Deterministic
// selection means clarification frames are always human-answered — an
// operator reading the chat should not have to discover that from a timeout.
func doctorPostureSummary(report runtimesvc.DoctorReport) string {
	var hitl, posture string
	for _, dep := range report.Dependencies {
		switch dep.Name {
		case "hitl_mode":
			hitl = firstPostureClause(dep.Details)
		case "selection_posture":
			posture = firstPostureClause(dep.Details)
		}
	}
	if hitl == "" && posture == "" {
		return ""
	}
	summary := "Postures —"
	if hitl != "" {
		summary += " approvals: " + hitl + ";"
	}
	if posture != "" {
		summary += " intent selection: " + posture
	}
	return strings.TrimRight(summary, "; ")
}

// firstPostureClause takes the detail text's lead clause (before the first
// colon or parenthesis) as the compact posture label.
func firstPostureClause(details string) string {
	detail := strings.TrimSpace(details)
	if detail == "" {
		return ""
	}
	if idx := strings.IndexAny(detail, ":("); idx >= 0 {
		detail = strings.TrimSpace(detail[:idx])
	}
	return detail
}
