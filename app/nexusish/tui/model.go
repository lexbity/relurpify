package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	runtime   RuntimeAdapter
	inputBar  *InputBar
	notifBar  *NotificationBar
	statusBar StatusBar
	tabBar    TabBar
	help      HelpOverlay
	tabs      []tabSpec

	width     int
	height    int
	activeTab tabID
	showHelp  bool

	state      RuntimeState
	loading    bool
	lastLoaded time.Time
	loadErr    error
}

func newModel(rt RuntimeAdapter) model {
	tabs := []tabSpec{
		{id: tabDashboard, key: "1", label: "1 Dashboard", pane: NewDashboardPane()},
		{id: tabNodes, key: "2", label: "2 Nodes", pane: NewNodesPane()},
		{id: tabChannels, key: "3", label: "3 Channels", pane: NewChannelsPane()},
		{id: tabIdentity, key: "4", label: "4 Identity", pane: NewIdentityPane()},
		{id: tabSecurity, key: "5", label: "5 Security", pane: NewSecurityPane()},
		{id: tabEvents, key: "6", label: "6 Events", pane: NewEventsPane()},
	}
	activeTab := tabDashboard
	return model{
		runtime:   rt,
		inputBar:  NewInputBar(),
		notifBar:  NewNotificationBar(),
		statusBar: NewStatusBar(),
		tabBar:    NewTabBar(activeTab, tabs),
		tabs:      tabs,
		activeTab: activeTab,
		loading:   true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadStateCmd(m.runtime), tickCmd(m.currentRefreshInterval()))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.handleWindowSize(msg)
	case tea.KeyMsg:
		if cmd, quit := m.handleKeyMsg(msg); quit {
			return m, tea.Quit
		} else if cmd != nil {
			return m, cmd
		}
	case stateLoadedMsg:
		m.handleStateLoaded(msg)
	case tickMsg:
		m.statusBar.SetNow(time.Time(msg))
		m.notifBar.Tick(time.Time(msg))
		return m, tea.Batch(loadStateCmd(m.runtime), tickCmd(m.currentRefreshInterval()))
	case pairingActionRequestMsg:
		return m, m.handlePairingAction(msg)
	case tokenActionRequestMsg:
		return m, m.handleTokenAction(msg)
	case policyActionRequestMsg:
		return m, m.handlePolicyAction(msg)
	case channelActionRequestMsg:
		return m, m.handleChannelAction(msg)
	case actionCompleteMsg:
		m.handleActionComplete(msg)
		return m, loadStateCmd(m.runtime)
	case inputSubmittedMsg:
		return m, m.handleInputSubmitted(msg)
	case notifActionMsg:
		return m, m.handleNotifAction(msg)
	}
	return m, nil
}

func loadStateCmd(rt RuntimeAdapter) tea.Cmd {
	return func() tea.Msg {
		state, err := rt.State(context.Background())
		return stateLoadedMsg{state: state, err: err}
	}
}

func tickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = refreshInterval
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
