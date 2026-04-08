package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

type fakePane struct {
	state      RuntimeState
	width      int
	height     int
	filter     string
	active     bool
	command    string
	args       []string
	updates    int
	subTabBar  string
	hasSubTabs bool
}

func (p *fakePane) SetData(state RuntimeState) { p.state = state }
func (p *fakePane) SetSize(width, height int)  { p.width, p.height = width, height }
func (p *fakePane) View() string               { return "pane:" + p.command + ":" + p.filter }
func (p *fakePane) SetFilter(query string)     { p.filter = strings.TrimSpace(strings.ToLower(query)) }
func (p *fakePane) HandleCommand(cmd string, args []string) tea.Cmd {
	p.command = cmd
	p.args = append([]string(nil), args...)
	return func() tea.Msg { return actionCompleteMsg{message: "done"} }
}
func (p *fakePane) Update(msg tea.Msg) tea.Cmd { p.updates++; return nil }
func (p *fakePane) SetActive(active bool)      { p.active = active }
func (p *fakePane) SubTabBar() string          { return p.subTabBar }
func (p *fakePane) HasSubTabs() bool           { return p.hasSubTabs }

func TestTUIHelperFunctionsAndComponents(t *testing.T) {
	require.Equal(t, "yes", yesNo(true))
	require.Equal(t, "no", yesNo(false))
	require.Equal(t, "local-only", gatewayExposure(":8090"))
	require.Equal(t, "network", gatewayExposure("0.0.0.0:8090"))
	require.True(t, matchesFilter("", "anything"))
	require.True(t, matchesFilter("mix", "MiXeD"))
	require.Equal(t, "fallback", emptyFallback(" ", "fallback"))
	require.Equal(t, "3!", pairingBadge(3))
	require.Equal(t, 4, max(4, 3))
	require.Equal(t, 3, min(4, 3))
	require.Greater(t, paneWidthFor(100), 0)
	require.Equal(t, []string{"cmd", "a", "b"}, func() []string {
		cmd, args := parseCommand(" cmd   a b ")
		return append([]string{cmd}, args...)
	}())
	require.Contains(t, summaryMessage("approve", "PAIR-1", nil), "completed")
	require.Contains(t, summaryMessage("approve", "PAIR-1", errors.New("boom")), "failed")
	require.Equal(t, "expired", timeUntil(time.Now().Add(-time.Hour)))
	require.Equal(t, "unknown", timeUntil(time.Time{}))
	require.NotEqual(t, "expired", timeUntil(time.Now().Add(time.Hour)))

	overlay := HelpOverlay{}
	require.Equal(t, "base", overlay.View("base"))
	overlay.SetSize(80, 24)
	rendered := overlay.View("base")
	require.Contains(t, rendered, "Navigation")
	require.Contains(t, rendered, "General")

	bar := NewInputBar()
	handled, cmd := bar.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	require.True(t, handled)
	require.Nil(t, cmd)
	handled, cmd = bar.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	require.True(t, handled)
	require.NotNil(t, cmd)
	submittedMsg := cmd().(inputSubmittedMsg)
	require.Equal(t, InputModeFilter, submittedMsg.mode)
	require.Equal(t, "a", submittedMsg.value)
	handled, cmd = bar.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.True(t, handled)
	require.NotNil(t, cmd)
	msg := cmd()
	submitted, ok := msg.(inputSubmittedMsg)
	require.True(t, ok)
	require.Equal(t, InputModeFilter, submitted.mode)
	require.Equal(t, "a", submitted.value)
	require.Contains(t, bar.View(tabDashboard), "a")

	notifBar := NewNotificationBar()
	notifBar.Push(Notification{Kind: NotificationAction, Title: "Pair", Value: "PAIR-1", Sticky: true})
	notifBar.Push(Notification{Kind: NotificationInfo, Title: "Info"})
	require.True(t, notifBar.Active())
	require.Contains(t, notifBar.View(), "Pair")
	action, ok := notifBar.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	require.True(t, ok)
	require.Equal(t, "approve", action.action)
	notifBar.Tick(time.Now())
	notifBar.DismissActive()
	require.True(t, notifBar.Active())

	tabBar := NewTabBar(tabDashboard, []tabSpec{{id: tabDashboard, label: "Dash"}, {id: tabNodes, label: "Nodes"}})
	tabBar.SetWidth(60)
	tabBar.SetBadge(tabNodes, "2", true)
	require.Contains(t, tabBar.View(), "Dash")
	require.Contains(t, tabBar.View(), "Nodes")
}

func TestTUIModelRegistryAndDispatch(t *testing.T) {
	p1 := &fakePane{subTabBar: "one", hasSubTabs: true}
	p2 := &fakePane{subTabBar: "two", hasSubTabs: true}
	p3 := &fakePane{subTabBar: "three", hasSubTabs: false}
	m := model{
		tabs: []tabSpec{
			{id: tabDashboard, key: "1", label: "Dashboard", pane: p1},
			{id: tabNodes, key: "2", label: "Nodes", pane: p2},
			{id: tabEvents, key: "6", label: "Events", pane: NewEventsPane()},
			{id: tabSecurity, key: "5", label: "Security", pane: p3},
		},
		activeTab: tabDashboard,
		inputBar:  NewInputBar(),
		notifBar:  NewNotificationBar(),
		statusBar: NewStatusBar(),
		tabBar:    NewTabBar(tabDashboard, []tabSpec{{id: tabDashboard, key: "1", label: "Dashboard"}, {id: tabNodes, key: "2", label: "Nodes"}, {id: tabEvents, key: "6", label: "Events"}, {id: tabSecurity, key: "5", label: "Security"}}),
	}

	tab, ok := m.tabByID(tabNodes)
	require.True(t, ok)
	require.Equal(t, "2", tab.key)
	tab, ok = m.tabByKey("5")
	require.True(t, ok)
	require.Equal(t, tabSecurity, tab.id)
	_, ok = m.activeContentPane()
	require.True(t, ok)
	keyPane, ok := m.activeKeyPane()
	require.True(t, ok)
	require.NotNil(t, keyPane)
	subPane, ok := m.activeSubTabbedPane()
	require.True(t, ok)
	require.Equal(t, "one", subPane.SubTabBar())
	filterPane, ok := m.activeFilterablePane()
	require.True(t, ok)
	filterPane.SetFilter(" Query ")
	require.Equal(t, "query", p1.filter)
	commandPane, ok := m.activeCommandablePane()
	require.True(t, ok)
	cmd := commandPane.HandleCommand("run", []string{"a", "b"})
	require.NotNil(t, cmd)
	require.Equal(t, "run", p1.command)
	require.Equal(t, []string{"a", "b"}, p1.args)

	m.syncStateToPanes(RuntimeState{Online: true})
	require.True(t, p1.state.Online)
	require.True(t, p2.state.Online)
	m.syncSizeToPanes(100, 40)
	require.Equal(t, 100, p1.width)
	require.Greater(t, p1.height, 0)
	m.setActiveTab(tabNodes)
	require.True(t, p2.active)
	require.False(t, p1.active)
	require.Equal(t, tabDashboard, m.nextTab(tabSecurity))
	require.Equal(t, tabNodes, m.prevTab(tabEvents))

	handled := m.handleInputSubmitted(inputSubmittedMsg{mode: InputModeFilter, value: " filter "})
	require.Nil(t, handled)
	require.Equal(t, "filter", p2.filter)
	handled = m.handleInputSubmitted(inputSubmittedMsg{mode: InputModeCommand, value: "run arg1 arg2"})
	require.NotNil(t, handled)
	require.Equal(t, "run", p2.command)
	require.Equal(t, []string{"arg1", "arg2"}, p2.args)

	quitCmd, quit := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	require.Nil(t, quitCmd)
	require.True(t, quit)

	m.activeTab = tabEvents
	require.Equal(t, followRefreshInterval, m.currentRefreshInterval())
	if events, ok := m.tabByID(tabEvents); ok {
		if pane, ok := events.pane.(*EventsPane); ok {
			pane.HandleCommand("follow", nil)
			require.Equal(t, refreshInterval, m.currentRefreshInterval())
		}
	}
}
