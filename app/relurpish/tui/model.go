package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	runtimesvc "codeburg.org/lexbit/relurpify/app/relurpish/runtime"
	archaeolearning "codeburg.org/lexbit/relurpify/archaeo/learning"
	fauthorization "codeburg.org/lexbit/relurpify/framework/authorization"
	"codeburg.org/lexbit/relurpify/archaeo/guidance"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Run bootstraps the TUI without a euclo plugin. This is the public entrypoint
// called by cmd/start.go when no agent-specific plugin is provided.
func Run(ctx context.Context, rt *runtimesvc.Runtime) error {
	return RunWithEuclo(ctx, rt, nil)
}

// RunWithEuclo bootstraps the TUI with an optional EucloPlugin that injects
// euclo-specific panes, tabs, and an interaction emitter.
func RunWithEuclo(ctx context.Context, rt *runtimesvc.Runtime, plugin *EucloPlugin) error {
	if rt == nil {
		return fmt.Errorf("runtime is required")
	}
	adapter := newRuntimeAdapter(rt)
	m := newRootModel(adapter, plugin)
	program := tea.NewProgram(
		m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	// Inject the interaction emitter into the euclo agent after program is created.
	if plugin != nil && plugin.NewEmitter != nil {
		emitter := plugin.NewEmitter(program)
		m.eucloEmitter = emitter
		if adapter != nil {
			adapter.SetInteractionEmitter(emitter)
		}
	}
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
	titleBar   TitleBar
	tabBar     TabBar
	notifBar   *NotificationBar
	inputBar   *InputBar
	cmdPalette *CommandPalette
	notifQ     *NotificationQueue

	// Panes (pointer types — mutations survive tea.Model value copies)
	// chat, planner, debug use interfaces so the euclo plugin can inject
	// its implementations; the concrete *ChatPane/*PlannerPane/*DebugPane still
	// live in this package and are the default implementations.
	chat    ChatPaner
	tasks   *TasksPane
	session *SessionPane
	planner PlannerPaner
	debug   DebugPaner
	archaeo ArchaeoPaner
	config  *ConfigPane

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
	eucloEmitter EucloEmitter

	// HITL subscription
	hitlCh    <-chan fauthorization.HITLEvent
	hitlUnsub func()

	guidanceCh    <-chan guidance.GuidanceEvent
	guidanceUnsub func()

	learningCh    <-chan archaeolearning.Event
	learningUnsub func()

	// Task queue: maps run IDs that originated from the task queue.
	taskRunIDs map[string]bool

	// Guidance panel (Phase B): renders above input bar when open.
	hitlPanel GuidancePanel

	// Phase G: instance-based command registry and corpus scope.
	cmdReg *CommandRegistry
	scope  string
}

type plannerDataRuntime interface {
	QueryPatternProposals(scope string) ([]PatternProposalInfo, error)
	QueryConfirmedPatterns(scope string) ([]PatternRecordInfo, error)
	QueryIntentGaps(filePath, scope string) ([]IntentGapInfo, error)
	QueryTensions(scope string) ([]TensionInfo, error)
	LoadLivePlan(workflowID string) (*LivePlanInfo, error)
	AddPlanNote(stepRef string, body string) error
	GetPlanDiff(workflowID string) (PlanDiffInfo, error)
	GetLatestTrace() (TraceInfo, error)
}

type debugExecRuntime interface {
	RunTests(pkg string) (DebugTestResultMsg, error)
	RunBenchmark(pkg string) (DebugBenchmarkResultMsg, error)
}

func newRootModel(rt RuntimeAdapter, plugins ...*EucloPlugin) RootModel {
	var plugin *EucloPlugin
	if len(plugins) > 0 {
		plugin = plugins[0]
	}
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

	inputBar := NewInputBar()
	if info.Workspace != "" {
		inputBar.SetWorkspace(info.Workspace)
	}
	if rt != nil {
		inputBar.SetRuntime(rt)
	}
	inputBar.SetCommandRegistry(rootCommandRegistry)
	inputBar.SetContext(TabChat, SubTabChatLocalEdit)

	// Build tab registry: euclo tabs (if plugin provided) + universal config + session tabs.
	tabs := NewTabRegistry()
	if plugin != nil && plugin.SetupTabs != nil {
		plugin.SetupTabs(tabs)
	} else {
		registerEucloTabs(tabs)
	}
	tabs.Register(TabDefinition{
		ID:    TabConfig,
		Label: "config",
	})
	tabs.Register(TabDefinition{
		ID:    TabSession,
		Label: "session",
		SubTabs: []SubTabDefinition{
			{SubTabSessionLive, "live"},
			{SubTabSessionTasks, "tasks"},
			{SubTabSessionServices, "services"},
			{SubTabSessionSettings, "settings"},
		},
	})
	tabs.SetActive(TabChat)
	tabs.SetSubActive(TabChat, SubTabChatLocalEdit)
	tabs.SetSubActive(TabSession, SubTabSessionLive)

	tabBar := NewTabBar(TabChat)
	tabBar.SetRegistry(tabs)

	m := RootModel{
		tabs:         tabs,
		subTabBar:    NewSubTabBar(tabs.ActiveTab()),
		hitlPanel:    newGuidancePanel(),
		titleBar:     NewTitleBar(info),
		tabBar:       tabBar,
		notifBar:     NewNotificationBar(notifQ),
		inputBar:     inputBar,
		cmdPalette:   NewCommandPalette(),
		notifQ:       notifQ,
		activeTab:    TabChat,
		titleVisible: true,
		sharedSess:   sess,
		sharedCtx:    ctx,
		runtime:      rt,
		taskRunIDs:   make(map[string]bool),
		cmdReg:       rootCommandRegistry,
		scope:        info.Workspace,
	}

	var store *SessionStore
	if info.Workspace != "" {
		store = NewSessionStore(info.Workspace)
	}
	m.store = store

	if plugin != nil && plugin.NewChat != nil {
		m.chat = plugin.NewChat(rt, ctx, sess, notifQ)
	} else {
		m.chat = NewChatPane(rt, ctx, sess, notifQ)
	}
	m.tasks = NewTasksPane(rt, notifQ)
	m.session = NewSessionPane(ctx, sess, rt)
	m.session.SyncQueuedTasks(m.tasks.Items())
	if plugin != nil && plugin.NewPlanner != nil {
		m.planner = plugin.NewPlanner()
	} else {
		m.planner = NewPlannerPane()
	}
	if plugin != nil && plugin.NewDebug != nil {
		m.debug = plugin.NewDebug()
	} else {
		m.debug = NewDebugPane()
	}
	m.archaeo = NewArchaeoPane(rt)
	m.config = NewConfigPane(rt)

	return m
}

func (m *RootModel) syncCommandPalette() {
	if m.inputBar == nil || m.cmdPalette == nil {
		return
	}
	open, items, sel := m.inputBar.PaletteState()
	m.cmdPalette.Sync(open, items, sel, m.width)
}

func (m RootModel) refreshActiveSurfaceCmd() tea.Cmd {
	loader, ok := m.runtime.(plannerDataRuntime)
	if !ok {
		if m.activeTab == TabSession && m.tabs.ActiveSubTab() == SubTabSessionLive && m.runtime != nil {
			return func() tea.Msg {
				workflows, _ := m.runtime.ListWorkflows(3)
				return SessionLiveSnapshotMsg{
					Info:      m.runtime.Diagnostics(),
					Workflows: workflows,
					Providers: m.runtime.ListLiveProviders(),
					Approvals: m.runtime.ListApprovals(),
				}
			}
		}
		return nil
	}
	switch m.activeTab {
	case TabPlanner:
		switch m.tabs.ActiveSubTab() {
		case SubTabPlannerExplore:
			return func() tea.Msg {
				records, err := loader.QueryConfirmedPatterns(m.scope)
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("planner load failed: %v", err)}
				}
				proposals, err := loader.QueryPatternProposals(m.scope)
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("planner load failed: %v", err)}
				}
				return PlannerPatternsMsg{Records: records, Proposals: proposals}
			}
		case SubTabPlannerAnalyze:
			return func() tea.Msg {
				tensions, err := loader.QueryTensions(m.scope)
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("analysis load failed: %v", err)}
				}
				gaps, err := loader.QueryIntentGaps("", m.scope)
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("analysis load failed: %v", err)}
				}
				return PlannerTensionsMsg{Tensions: tensions, Gaps: gaps}
			}
		case SubTabPlannerFinalize:
			return func() tea.Msg {
				plan, err := loader.LoadLivePlan("")
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("plan load failed: %v", err)}
				}
				if plan == nil {
					return PlannerPlanMsg{}
				}
				return PlannerPlanMsg{Plan: *plan}
			}
		}
	case TabDebug:
		switch m.tabs.ActiveSubTab() {
		case SubTabDebugTest, SubTabDebugBenchmark:
			return nil
		case SubTabDebugTrace:
			return func() tea.Msg {
				trace, err := loader.GetLatestTrace()
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("trace load failed: %v", err)}
				}
				return DebugTraceMsg{Trace: trace}
			}
		case SubTabDebugPlanDiff:
			return func() tea.Msg {
				diff, err := loader.GetPlanDiff("")
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("plan diff load failed: %v", err)}
				}
				return DebugPlanDiffMsg{Diff: diff}
			}
		}
	case TabArchaeo:
		if m.runtime == nil {
			return nil
		}
		rt := m.runtime
		switch m.tabs.ActiveSubTab() {
		case SubTabArchaeoPlan:
			planCmd := func() tea.Msg {
				plan, err := rt.LoadActivePlan(context.Background(), "")
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("plan load failed: %v", err)}
				}
				return PlanUpdatedMsg{Plan: plan}
			}
			blobsCmd := func() tea.Msg {
				blobs, err := rt.LoadBlobs(context.Background(), "")
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("blob load failed: %v", err)}
				}
				return BlobsUpdatedMsg{Blobs: blobs}
			}
			return tea.Batch(planCmd, blobsCmd)
		case SubTabArchaeoReview:
			return func() tea.Msg {
				return ArchaeoLearningQueueMsg{Interactions: rt.PendingLearning()}
			}
		case SubTabArchaeoHistory:
			return func() tea.Msg {
				versions, err := rt.ListPlanVersions(context.Background(), "")
				if err != nil {
					return chatSystemMsg{Text: fmt.Sprintf("history load failed: %v", err)}
				}
				return PlanHistoryUpdatedMsg{Versions: versions}
			}
		}
	case TabSession:
		if m.runtime == nil {
			return nil
		}
		switch m.tabs.ActiveSubTab() {
		case SubTabSessionLive:
			return func() tea.Msg {
				workflows, _ := m.runtime.ListWorkflows(3)
				return SessionLiveSnapshotMsg{
					Info:      m.runtime.Diagnostics(),
					Workflows: workflows,
					Providers: m.runtime.ListLiveProviders(),
					Approvals: m.runtime.ListApprovals(),
				}
			}
		case SubTabSessionServices:
			return func() tea.Msg {
				return ServicesUpdatedMsg{Services: m.runtime.ListServices()}
			}
		}
	}
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
	return tea.Batch(
		textinput.Blink,
		m.chat.Init(),
		m.session.Init(),
		m.restorePromptCmd(),
		m.subscribeHITLCmd(),
		m.subscribeGuidanceCmd(),
		m.subscribeLearningCmd(),
		m.refreshArchaeoLearningQueueCmd(),
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

func (m RootModel) subscribeGuidanceCmd() tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		if rt == nil {
			return nil
		}
		ch, unsub := rt.SubscribeGuidance()
		return guidanceSubscribedMsg{ch: ch, unsub: unsub}
	}
}

func (m RootModel) subscribeLearningCmd() tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		if rt == nil {
			return nil
		}
		ch, unsub := rt.SubscribeLearning()
		return learningSubscribedMsg{ch: ch, unsub: unsub}
	}
}

func (m RootModel) refreshArchaeoLearningQueueCmd() tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		if rt == nil {
			return nil
		}
		return ArchaeoLearningQueueMsg{Interactions: rt.PendingLearning()}
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
		// Quit shortcuts bypass everything.
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
		}
		// Guidance panel captures all keys when open.
		if m.hitlPanel.IsOpen() {
			panel, cmd := m.hitlPanel.Update(msg)
			m.hitlPanel = panel
			return m, cmd
		}
		// Notification bar captures keys when active unless the current guidance
		// request expects freetext input through the input bar.
		if m.notifBar.Active() && !m.shouldRouteNotificationKeyToInput(msg) {
			nb, cmd := m.notifBar.Update(msg)
			m.notifBar = nb
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
		return m.handleInputSubmitted(msg.Value)

	case CommandInvokedMsg:
		if m.cmdPalette != nil {
			m.cmdPalette.Close()
		}
		nm, cmd := executeCommand(&m, msg.Name, msg.Args)
		return *nm, cmd

	// Notification responses
	case NotifHITLApproveMsg:
		var hitlSvc hitlService
		if m.chat != nil {
			hitlSvc = m.chat.HITLService()
		}
		cmds := []tea.Cmd{approveHITLRootCmd(hitlSvc, msg.ID, msg.Scope)}
		if msg.Scope == fauthorization.GrantScopePersistent {
			cmds = append(cmds, savePolicyCmd(m.runtime, msg.Action))
		}
		return m, tea.Batch(cmds...)
	case NotifHITLDenyMsg:
		var hitlSvc hitlService
		if m.chat != nil {
			hitlSvc = m.chat.HITLService()
		}
		return m, denyHITLRootCmd(hitlSvc, msg.ID)
	case NotifDismissMsg:
		m.notifQ.Resolve(msg.ID)
		m.syncCommandPalette()
		return m, nil
	case NotifRestoreSessionMsg:
		return m.handleRestoreSession(msg.ID)
	case NotifReviewDeferredMsg:
		return rootHandleDeferred(&m, nil)

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

	// HITL subscription initialization.
	case hitlSubscribedMsg:
		m.hitlCh = msg.ch
		m.hitlUnsub = msg.unsub
		if m.hitlCh != nil {
			return m, listenHITLEvents(m.hitlCh)
		}
		return m, nil

	case guidanceSubscribedMsg:
		m.guidanceCh = msg.ch
		m.guidanceUnsub = msg.unsub
		if m.guidanceCh != nil {
			return m, listenGuidanceEvents(m.guidanceCh)
		}
		return m, nil

	case learningSubscribedMsg:
		m.learningCh = msg.ch
		m.learningUnsub = msg.unsub
		if m.learningCh != nil {
			return m, listenLearningEvents(m.learningCh)
		}
		return m, nil

	case learningEventMsg:
		return m.handleLearningEvent(msg)

	case learningResolvedMsg:
		if msg.err != nil {
			m.addSystemMessage(fmt.Sprintf("Learning resolve failed: %v", msg.err))
		} else {
			m.addSystemMessage(fmt.Sprintf("Learning resolved: %s", msg.interactionID))
		}
		return m, m.refreshArchaeoLearningQueueCmd()

	case ArchaeoLearningQueueMsg:
		if m.archaeo != nil {
			ap, cmd := m.archaeo.Update(msg)
			m.archaeo = ap
			return m, cmd
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
		cp, cmd := m.manifest.Update(msg)
		m.config = cp
		return m, cmd

	// Debug data messages — route to debug pane regardless of active tab.
	case DebugTestResultMsg, DebugBenchmarkResultMsg, DebugTraceMsg, DebugPlanDiffMsg:
		dp, cmd := m.debug.Update(msg)
		m.debug = dp
		return m, cmd

	// Planner data messages — route to planner regardless of active tab.
	case PlannerPatternsMsg, PlannerTensionsMsg, PlannerPlanMsg:
		pp, cmd := m.planner.Update(msg)
		m.planner = pp
		return m, cmd
	case plannerNoteAddedMsg:
		if loader, ok := m.runtime.(plannerDataRuntime); ok {
			if err := loader.AddPlanNote(msg.stepID, msg.note); err != nil {
				m.addSystemMessage(fmt.Sprintf("plan note save failed: %v", err))
			}
		}
		pp, cmd := m.planner.Update(msg)
		m.planner = pp
		return m, tea.Batch(cmd, m.refreshActiveSurfaceCmd())

	// Archaeo pane data messages — route to archaeo pane regardless of active tab.
	case PlanUpdatedMsg, ArchaeoExploreMsg, clearPlanHighlightMsg, PlanHistoryUpdatedMsg:
		ap, cmd := m.archaeo.Update(msg)
		m.archaeo = ap
		return m, cmd

	case BlobsUpdatedMsg:
		ap, cmd := m.archaeo.Update(msg)
		m.archaeo = ap
		// Update titlebar blob counts when archaeo tab is active.
		if m.activeTab == TabArchaeo {
			tensions, patterns, learning := countBlobsByKind(msg.Blobs)
			m.titleBar.SetBlobCounts(tensions, patterns, learning)
		}
		return m, cmd

	// Archaeo blob operations — route to pane, then trigger a plan+blob refresh.
	case blobAddedMsg, blobRemovedMsg:
		ap, cmd := m.archaeo.Update(msg)
		m.archaeo = ap
		return m, tea.Batch(cmd, m.refreshActiveSurfaceCmd())

	case planVersionActivatedMsg:
		ap, cmd := m.archaeo.Update(msg)
		m.archaeo = ap
		// Refresh history after activation.
		return m, tea.Batch(cmd, m.refreshActiveSurfaceCmd())

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
	case guidanceEventMsg:
		return m.handleGuidanceEvent(msg)
	case guidanceResolvedMsg:
		return m.handleGuidanceResolved(msg)
	case NotifGuidanceResolveMsg:
		return m, guidanceRequestCmd(m.runtime, msg.RequestID, msg.ChoiceID, msg.Freetext)

	// Guidance panel responses.
	case GuidancePanelSubmitMsg:
		m.syncCommandPalette()
		return m, guidanceRequestCmd(m.runtime, msg.RequestID, "", msg.Response)
	case GuidancePanelDeferMsg:
		// Defer: resolve with empty choice (system records as deferred observation).
		m.syncCommandPalette()
		return m, guidanceRequestCmd(m.runtime, msg.RequestID, "defer", "")
	case GuidancePanelAnnotateMsg:
		// Annotation saved; panel stays open — no further model action needed here.
		return m, nil
	case GuidancePanelJumpExploreMsg:
		// Jump to planner/explore at the relevant pattern.
		m.setActiveTab(TabPlanner)
		m.setActiveSubTab(SubTabPlannerExplore)
		return m, m.refreshActiveSurfaceCmd()

	// Euclo interaction frame handling.
	case EucloFrameMsg:
		// Push the frame as an interactive notification.
		if m.notifQ != nil {
			m.notifQ.PushInteraction(msg.Frame)
		}
		// Add the rendered frame to the chat feed and update sidebar on proposal frames.
		if m.chat != nil {
			m.chat.AppendMessage(msg.Msg)
			m.chat.UpdateSidebarFromFrame(msg.Frame)
		}
		// Route archaeo findings frames to the archaeo pane regardless of active tab.
		if msg.Frame.Kind == interaction.FrameArchaeoFindings && m.archaeo != nil {
			ap, cmd := m.archaeo.Update(msg)
			m.archaeo = ap
			return m, cmd
		}
		return m, nil

	// Euclo interaction response handling.
	case EucloResponseMsg:
		if m.eucloEmitter != nil {
			m.eucloEmitter.Resolve(msg.Response)
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
		if msg.err != nil {
			// Roll back the undo snapshot we pushed before the call.
			if m.chat != nil {
				m.chat.RollbackLastUndo()
			}
			m.addSystemMessage(fmt.Sprintf("Compact failed: %v", msg.err))
			return m, nil
		}
		if m.chat != nil {
			m.chat.ClearMessages()
			m.chat.AddSystemMessage(fmt.Sprintf(
				"Session compacted — %d messages → 1 summary. [ctrl+z to undo]",
				msg.originalCount,
			))
			m.chat.AppendMessage(Message{
				ID:        fmt.Sprintf("compact-%d", time.Now().UnixNano()),
				Role:      RoleAgent,
				Timestamp: time.Now(),
				Content:   MessageContent{Text: msg.summary},
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

	chatPane, chatCmd := m.chat.Update(msg)
	m.chat = chatPane
	if chatCmd != nil {
		cmds = append(cmds, chatCmd)
	}

	switch m.activeTab {
	case TabSession:
		sp, cmd := m.session.Update(msg)
		m.session = sp
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case TabPlanner:
		pp, cmd := m.planner.Update(msg)
		m.planner = pp
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case TabDebug:
		dp, cmd := m.debug.Update(msg)
		m.debug = dp
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case TabArchaeo:
		ap, cmd := m.archaeo.Update(msg)
		m.archaeo = ap
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case TabConfig:
		cp, cmd := m.manifest.Update(msg)
		m.config = cp
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

	// Subtab bar (always present; renders empty row when no subtabs).
	parts = append(parts, m.subTabBar.View())

	// Active pane content.
	parts = append(parts, m.activePaneView())

	overlay := overlayPanelView(
		func() string {
			if m.notifBar != nil && m.notifBar.Active() {
				return m.notifBar.View()
			}
			return ""
		}(),
		func() string {
			if m.cmdPalette != nil && m.cmdPalette.IsOpen() {
				return m.cmdPalette.View()
			}
			return ""
		}(),
		func() string {
			if m.hitlPanel.IsOpen() {
				return m.hitlPanel.View()
			}
			return ""
		}(),
	)
	if overlay != "" {
		parts = append(parts, overlay)
	}

	// Input bar (always).
	streaming := m.chat != nil && m.chat.HasActiveRuns()
	parts = append(parts, m.inputBar.View(m.activeTab, streaming))

	// Tab bar (always).
	parts = append(parts, m.tabBar.View())
	parts = append(parts, m.renderStatusBar())

	base := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Help overlay sits on top of everything.
	if m.showHelp {
		return m.help.View(base)
	}
	return base
}

func (m RootModel) activePaneView() string {
	switch m.activeTab {
	case TabSession:
		return m.session.View()
	case TabPlanner:
		return m.planner.View()
	case TabDebug:
		return m.debug.View()
	case TabArchaeo:
		return m.archaeo.View()
	case TabConfig:
		return m.manifest.View()
	default:
		return m.chat.View()
	}
}

// handleResize distributes new terminal dimensions to all components.
func (m RootModel) handleResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	m.ready = true

	m.layout.Recalculate(msg.Width, msg.Height)
	paneH := m.layout.MainPaneHeight(0)

	m.titleBar.SetWidth(msg.Width)
	m.subTabBar.SetWidth(msg.Width)
	m.tabBar.SetWidth(msg.Width)
	m.notifBar.SetWidth(msg.Width)
	m.inputBar.SetWidth(msg.Width)
	m.help.SetSize(msg.Width, msg.Height)

	m.chat.SetSize(msg.Width, paneH)
	m.session.SetSize(msg.Width, paneH)
	m.planner.SetSize(msg.Width, paneH)
	m.debug.SetSize(msg.Width, paneH)
	m.archaeo.SetSize(msg.Width, paneH)
	m.manifest.SetSize(msg.Width, paneH)

	return m, nil
}

// layoutHeights computes component heights for the current terminal size.
// It remains as a small compatibility wrapper around ChromeLayout.
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

// setActiveTab updates activeTab on the model, the tab bar, the tab registry,
// and the subtab bar consistently.
func (m *RootModel) setActiveTab(id TabID) {
	m.activeTab = id
	m.titleBar.SetActiveTab(id)
	m.tabBar.SetActive(id)
	m.tabs.SetActive(id)
	m.subTabBar.SetSubTabs(m.tabs.ActiveTab())
	sub := m.tabs.ActiveSubTab()
	m.inputBar.SetContext(id, sub)
	m.syncCommandPalette()
	if id == TabChat && m.chat != nil {
		m.chat.SetSubTab(sub)
	}
	if id == TabPlanner && m.planner != nil {
		m.planner.SetSubTab(sub)
	}
	if id == TabDebug && m.debug != nil {
		m.debug.SetSubTab(sub)
	}
	if id == TabSession && m.session != nil {
		m.session.SetSubTab(sub)
	}
	if id == TabArchaeo && m.archaeo != nil {
		m.archaeo.SetSubTab(sub)
	}
}

// setActiveSubTab changes the active subtab for the current main tab and
// notifies panes that care about subtab changes.
func (m *RootModel) setActiveSubTab(sub SubTabID) {
	m.tabs.SetSubActive(m.activeTab, sub)
	m.subTabBar.SetActive(sub)
	m.inputBar.SetContext(m.activeTab, sub)
	m.syncCommandPalette()
	if m.activeTab == TabChat && m.chat != nil {
		m.chat.SetSubTab(sub)
	}
	if m.activeTab == TabPlanner && m.planner != nil {
		m.planner.SetSubTab(sub)
	}
	if m.activeTab == TabDebug && m.debug != nil {
		m.debug.SetSubTab(sub)
	}
	if m.activeTab == TabSession && m.session != nil {
		m.session.SetSubTab(sub)
	}
	if m.activeTab == TabArchaeo && m.archaeo != nil {
		m.archaeo.SetSubTab(sub)
	}
}

// handleGlobalKey processes navigation keys emitted by InputBar when the
// input field is empty.
func (m RootModel) handleGlobalKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c", "ctrl+d":
		return m, tea.Batch(func() tea.Msg { m.cleanup(); return nil }, tea.Quit)
	case "1", "2", "3", "4", "5", "6":
		idx := int(key[0]-'0') - 1
		id := m.tabs.TabAtIndex(idx)
		if id != "" {
			m.setActiveTab(id)
			return m, m.refreshActiveSurfaceCmd()
		}
	case "tab":
		m.setActiveTab(m.tabs.CycleNext())
		return m, m.refreshActiveSurfaceCmd()
	case "shift+tab":
		m.setActiveTab(m.tabs.CyclePrev())
		return m, m.refreshActiveSurfaceCmd()
	case "]":
		m.setActiveSubTab(m.tabs.CycleSubNext())
		return m, m.refreshActiveSurfaceCmd()
	case "[":
		m.setActiveSubTab(m.tabs.CycleSubPrev())
		return m, m.refreshActiveSurfaceCmd()
	case "ctrl+t":
		m.titleVisible = !m.titleVisible
		if m.titleVisible {
			m.layout.TitleHeight = 1
		} else {
			m.layout.TitleHeight = 0
		}
		paneH := m.layout.MainPaneHeight(0)
		m.chat.SetSize(m.width, paneH)
		m.session.SetSize(m.width, paneH)
		m.planner.SetSize(m.width, paneH)
		m.debug.SetSize(m.width, paneH)
		m.manifest.SetSize(m.width, paneH)
	case "ctrl+f":
		m.searchActive = !m.searchActive
		m.inputBar.SetSearchMode(m.searchActive)
		if !m.searchActive && m.chat != nil {
			m.chat.SetSearchFilter("")
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
			m.chat.SetSearchFilter("")
		}
	case "ctrl+z":
		// Undo: revert to previous feed snapshot
		if m.chat != nil {
			if !m.chat.Undo() {
				m.addSystemMessage("nothing to undo")
			}
		}
	case "ctrl+y":
		// Redo: restore the next feed snapshot
		if m.chat != nil {
			if !m.chat.Redo() {
				m.addSystemMessage("nothing to redo")
			}
		}
	case "ctrl+u":
		// Scroll up: scroll the chat feed up
		if m.chat != nil && m.activeTab == TabChat {
			m.chat.ScrollUp()
		}
	case "pagedown":
		// Page down: scroll the chat feed down by page
		if m.chat != nil && m.activeTab == TabChat {
			m.chat.PageDown()
		}
	case "pageup":
		// Page up: scroll the chat feed up by page
		if m.chat != nil && m.activeTab == TabChat {
			m.chat.PageUp()
		}
	case "@":
		// File picker: enable file selection mode in input
		m.inputBar.SetFilePickerMode(true)
	case "ctrl+]":
		// Toggle chat context sidebar
		if m.chat != nil && m.activeTab == TabChat {
			// We need to cast to *ChatPane to access ToggleSidebar
			if chatPane, ok := m.chat.(*ChatPane); ok {
				chatPane.ToggleSidebar()
			}
		}
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
		if m.cmdPalette != nil {
			m.cmdPalette.Close()
		}
		return m, nil
	}
	if requestID, ok := m.activeGuidanceFreetextRequestID(); ok && !strings.HasPrefix(value, "/") {
		if m.notifQ != nil {
			m.notifQ.Resolve(requestID)
		}
		return m, guidanceRequestCmd(m.runtime, requestID, "", value)
	}
	if notifID, actionID, ok := m.activeInteractionFreetextActionID(); ok && !strings.HasPrefix(value, "/") {
		if m.notifQ != nil {
			m.notifQ.Resolve(notifID)
		}
		resp := interaction.UserResponse{ActionID: actionID, Text: value}
		return m, func() tea.Msg { return EucloResponseMsg{Response: resp} }
	}
	// Search mode: filter the chat feed instead of submitting a prompt.
	if m.searchActive && m.chat != nil {
		m.chat.SetSearchFilter(value)
		return m, nil
	}
	switch m.activeTab {
	case TabSession:
		m.session.HandleFilterInput(value)
		return m, nil
	case TabPlanner:
		cmd := m.planner.HandleInputSubmit(value)
		return m, tea.Batch(cmd, m.refreshActiveSurfaceCmd())
	case TabDebug:
		cmd := m.debug.HandleInputSubmit(value)
		if runner, ok := m.runtime.(debugExecRuntime); ok {
			switch m.tabs.ActiveSubTab() {
			case SubTabDebugTest:
				return m, tea.Batch(cmd, func() tea.Msg {
					result, err := runner.RunTests(value)
					if result.Package == "" {
						result.Package = strings.TrimSpace(value)
					}
					if err != nil {
						result.Err = err
					}
					return result
				})
			case SubTabDebugBenchmark:
				return m, tea.Batch(cmd, func() tea.Msg {
					result, err := runner.RunBenchmark(value)
					if result.Package == "" {
						result.Package = strings.TrimSpace(value)
					}
					if err != nil {
						result.Err = err
					}
					return result
				})
			}
		}
		return m, tea.Batch(cmd, m.refreshActiveSurfaceCmd())
	case TabArchaeo:
		cmd := m.archaeo.HandleInputSubmit(value)
		return m, cmd
	case TabConfig:
		// Config pane input: trigger refresh (any non-empty text acts as refresh).
		return m, func() tea.Msg { return configRefreshMsg{} }
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
	rec := SessionRecord{
		SessionMeta: SessionMeta{
			ID:        m.sharedSess.ID,
			Agent:     m.sharedSess.Agent,
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
	if m.guidanceUnsub != nil {
		m.guidanceUnsub()
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

func (m RootModel) handleGuidanceEvent(msg guidanceEventMsg) (RootModel, tea.Cmd) {
	switch msg.event.Type {
	case guidance.GuidanceEventRequested:
		if msg.event.Request != nil {
			req := msg.event.Request
			m.hitlPanel.Open(guidanceKindToTrigger(req.Kind), req.ID, req.Title, req.Description, nil, "", "")
		}
	case guidance.GuidanceEventResolved, guidance.GuidanceEventExpired:
		if msg.event.Request != nil {
			if m.hitlPanel.IsOpen() && m.hitlPanel.RequestID() == msg.event.Request.ID {
				m.hitlPanel.Close()
			}
			if m.notifQ != nil {
				m.notifQ.Resolve(msg.event.Request.ID)
			}
		}
	case guidance.GuidanceEventDeferred:
		if msg.event.Request != nil {
			if m.hitlPanel.IsOpen() && m.hitlPanel.RequestID() == msg.event.Request.ID {
				m.hitlPanel.Close()
			}
			if m.notifQ != nil {
				m.notifQ.Resolve(msg.event.Request.ID)
			}
		}
		if m.runtime != nil && m.notifQ != nil {
			pending := m.runtime.PendingDeferrals()
			if len(pending) > 0 {
				m.notifQ.Push(NotificationItem{
					Kind: NotifKindDeferred,
					ID:   generateID(),
					Msg:  fmt.Sprintf("%d deferred guidance items pending review", len(pending)),
				})
			}
		}
	}
	return m, listenGuidanceEvents(m.guidanceCh)
}

// guidanceKindToTrigger maps a guidance.GuidanceKind to the TUI GuidanceTriggerKind.
func guidanceKindToTrigger(k guidance.GuidanceKind) GuidanceTriggerKind {
	switch k {
	case guidance.GuidanceAmbiguity, guidance.GuidanceContradiction:
		return GuidanceTriggerAmbiguity
	case guidance.GuidanceApproach, guidance.GuidanceScopeExpansion:
		return GuidanceTriggerLearning
	case guidance.GuidanceRecovery:
		return GuidanceTriggerDeferred
	default:
		return GuidanceTriggerAmbiguity
	}
}

func (m RootModel) handleGuidanceResolved(msg guidanceResolvedMsg) (RootModel, tea.Cmd) {
	if m.notifQ != nil {
		m.notifQ.Resolve(msg.requestID)
	}
	if msg.err != nil {
		m.addSystemMessage(fmt.Sprintf("Guidance %s failed: %v", msg.requestID, msg.err))
	} else {
		m.addSystemMessage(fmt.Sprintf("Guidance resolved: %s", msg.requestID))
	}
	return m, nil
}

// handleLearningEvent routes learning broker events to the notification queue.
// Blocking interactions also open the guidance panel so the operator can
// confirm or defer before plan execution resumes.
func (m RootModel) handleLearningEvent(msg learningEventMsg) (RootModel, tea.Cmd) {
	switch msg.event.Type {
	case archaeolearning.EventRequested:
		if msg.event.Interaction != nil {
			interaction := msg.event.Interaction
			if m.notifQ != nil {
				m.notifQ.Push(NotificationItem{
					Kind: NotifKindGuidance,
					ID:   interaction.ID,
					Msg:  fmt.Sprintf("Learning: %s", interaction.Title),
					Extra: map[string]string{
						"interaction_id": interaction.ID,
						"workflow_id":    interaction.WorkflowID,
						"kind":           string(interaction.Kind),
					},
				})
			}
			// Blocking interactions open the guidance panel so the operator must
			// respond before execution can continue.
			if interaction.Blocking {
				m.hitlPanel.Open(GuidanceTriggerLearning, interaction.ID, interaction.Title, interaction.Description, nil, "", "")
			}
		}
	case archaeolearning.EventResolved, archaeolearning.EventExpired, archaeolearning.EventDeferred:
		if msg.event.Interaction != nil {
			id := msg.event.Interaction.ID
			if m.hitlPanel.IsOpen() && m.hitlPanel.RequestID() == id {
				m.hitlPanel.Close()
			}
			if m.notifQ != nil {
				m.notifQ.Resolve(id)
			}
		}
	}
	return m, tea.Batch(listenLearningEvents(m.learningCh), m.refreshArchaeoLearningQueueCmd())
}

func (m RootModel) shouldRouteNotificationKeyToInput(msg tea.KeyMsg) bool {
	if !m.notifBar.Active() {
		return false
	}
	item, ok := m.notifQ.Current()
	if !ok {
		return false
	}
	isGuidanceFreetext := item.Kind == NotifKindGuidance && notificationAllowsFreetext(item)
	isInteractionFreetext := item.Kind == NotifKindInteraction && notificationAllowsFreetext(item)
	if !isGuidanceFreetext && !isInteractionFreetext {
		return false
	}
	switch msg.String() {
	case "d", "esc", "enter":
		return false
	}
	if _, handled := ResolveInteractionKey(item, msg.String()); handled {
		return false
	}
	return true
}

func (m RootModel) activeGuidanceFreetextRequestID() (string, bool) {
	if m.notifQ == nil {
		return "", false
	}
	item, ok := m.notifQ.Current()
	if !ok || item.Kind != NotifKindGuidance || !notificationAllowsFreetext(item) {
		return "", false
	}
	requestID := strings.TrimSpace(item.Extra["guidance_request_id"])
	if requestID == "" {
		return "", false
	}
	return requestID, true
}

// activeInteractionFreetextActionID returns the notification item ID and
// freetext action ID when the current notification is an interaction with a
// freetext slot active.
func (m RootModel) activeInteractionFreetextActionID() (notifID, actionID string, ok bool) {
	if m.notifQ == nil {
		return "", "", false
	}
	item, exists := m.notifQ.Current()
	if !exists || item.Kind != NotifKindInteraction || !notificationAllowsFreetext(item) {
		return "", "", false
	}
	count, _ := strconv.Atoi(item.Extra["action_count"])
	for i := 0; i < count; i++ {
		if item.Extra[fmt.Sprintf("action_%d_kind", i)] == string(interaction.ActionFreetext) {
			aID := strings.TrimSpace(item.Extra[fmt.Sprintf("action_%d_id", i)])
			if aID != "" {
				return item.ID, aID, true
			}
		}
	}
	return "", "", false
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
