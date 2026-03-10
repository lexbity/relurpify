package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) handleWindowSize(msg tea.WindowSizeMsg) {
	m.width = msg.Width
	m.height = msg.Height
	m.tabBar.SetWidth(msg.Width)
	m.inputBar.SetWidth(msg.Width)
	m.notifBar.SetWidth(msg.Width)
	m.statusBar.SetWidth(msg.Width)
	m.help.SetSize(msg.Width, msg.Height)
	m.syncSizeToPanes(msg.Width, msg.Height)
}

func (m *model) handleKeyMsg(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.showHelp {
		switch msg.String() {
		case "?", "esc":
			m.showHelp = false
			return nil, false
		}
	}
	if m.notifBar != nil {
		if action, ok := m.notifBar.HandleKey(msg); ok {
			return func() tea.Msg { return action }, false
		}
	}
	if m.inputBar != nil {
		if handled, cmd := m.inputBar.Update(msg); handled {
			return cmd, false
		}
	}
	switch msg.String() {
	case "q", "ctrl+c":
		return nil, true
	case "r":
		m.loading = true
		return loadStateCmd(m.runtime), false
	case "tab", "right", "l":
		m.setActiveTab(m.nextTab(m.activeTab))
	case "shift+tab", "left", "h":
		m.setActiveTab(m.prevTab(m.activeTab))
	default:
		if tab, ok := m.tabByKey(msg.String()); ok {
			m.setActiveTab(tab.id)
		}
	}
	if pane, ok := m.activeKeyPane(); ok {
		return pane.Update(msg), false
	}
	return nil, false
}

func (m *model) handleStateLoaded(msg stateLoadedMsg) {
	m.loading = false
	m.state = msg.state
	m.loadErr = msg.err
	if msg.err == nil {
		m.lastLoaded = time.Now()
		m.statusBar.SetState(msg.state)
		m.notifBar.SyncState(msg.state)
		m.tabBar.SetBadge(tabNodes, pairingBadge(len(msg.state.PendingPairings)), len(msg.state.PendingPairings) > 0)
		m.syncStateToPanes(msg.state)
	}
}

func (m *model) handleActionComplete(msg actionCompleteMsg) {
	level := NotificationInfo
	if msg.err != nil {
		level = NotificationError
	}
	m.notifBar.Push(Notification{
		Kind:   level,
		Title:  msg.message,
		Value:  "",
		Sticky: msg.err != nil,
	})
	m.loading = true
}

func (m model) handlePairingAction(msg pairingActionRequestMsg) tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		var err error
		switch msg.action {
		case "approve":
			err = rt.ApprovePairing(context.Background(), msg.code)
		case "reject":
			err = rt.RejectPairing(context.Background(), msg.code)
		default:
			err = fmt.Errorf("unsupported pairing action: %s", msg.action)
		}
		return actionCompleteMsg{
			message: summaryMessage(msg.action, msg.code, err),
			err:     err,
		}
	}
}

func (m model) handleTokenAction(msg tokenActionRequestMsg) tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		var (
			err     error
			message string
		)
		switch msg.action {
		case "issue":
			token, issueErr := rt.IssueToken(context.Background(), IssueTokenRequest{SubjectID: msg.subjectID, Scope: msg.scope})
			err = issueErr
			if err == nil {
				message = fmt.Sprintf("issued token for %s: %s", msg.subjectID, token)
			}
		case "revoke":
			err = rt.RevokeToken(context.Background(), msg.tokenID)
			message = summaryMessage("revoke", msg.tokenID, err)
		default:
			err = fmt.Errorf("unsupported token action: %s", msg.action)
		}
		if message == "" {
			message = summaryMessage(msg.action, emptyFallback(msg.tokenID, msg.subjectID), err)
		}
		return actionCompleteMsg{message: message, err: err}
	}
}

func (m model) handlePolicyAction(msg policyActionRequestMsg) tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		err := rt.SetPolicyRuleEnabled(context.Background(), msg.ruleID, msg.enabled)
		verb := "disable"
		if msg.enabled {
			verb = "enable"
		}
		return actionCompleteMsg{
			message: summaryMessage(verb, msg.ruleID, err),
			err:     err,
		}
	}
}

func (m model) handleChannelAction(msg channelActionRequestMsg) tea.Cmd {
	rt := m.runtime
	return func() tea.Msg {
		var err error
		switch msg.action {
		case "restart":
			err = rt.RestartChannel(context.Background(), msg.channel)
		default:
			err = fmt.Errorf("unsupported channel action: %s", msg.action)
		}
		return actionCompleteMsg{
			message: summaryMessage(msg.action, msg.channel, err),
			err:     err,
		}
	}
}

func (m model) handleInputSubmitted(msg inputSubmittedMsg) tea.Cmd {
	switch msg.mode {
	case InputModeFilter:
		if pane, ok := m.activeFilterablePane(); ok {
			pane.SetFilter(msg.value)
		}
	case InputModeCommand:
		if pane, ok := m.activeCommandablePane(); ok {
			cmd, args := parseCommand(msg.value)
			return pane.HandleCommand(cmd, args)
		}
	case InputModeHelp:
		m.showHelp = !m.showHelp
	}
	return nil
}

func (m model) handleNotifAction(msg notifActionMsg) tea.Cmd {
	switch msg.action {
	case "approve", "reject":
		return m.handlePairingAction(pairingActionRequestMsg{action: msg.action, code: msg.value})
	case "dismiss":
		m.notifBar.DismissActive()
	}
	return nil
}

func (m *model) setActiveTab(tab tabID) {
	m.activeTab = tab
	m.tabBar.SetActive(tab)
	m.syncActivePane()
}

func (m *model) syncStateToPanes(state RuntimeState) {
	for _, tab := range m.tabs {
		if tab.pane != nil {
			tab.pane.SetData(state)
		}
	}
}

func (m *model) syncSizeToPanes(width, height int) {
	_, body, _, _, _, _ := m.layoutHeights()
	for _, tab := range m.tabs {
		if tab.pane != nil {
			tab.pane.SetSize(width, body)
		}
	}
}

func (m *model) syncActivePane() {
	for _, tab := range m.tabs {
		if activatable, ok := tab.pane.(activatablePane); ok {
			activatable.SetActive(tab.id == m.activeTab)
		}
	}
}

func (m model) currentRefreshInterval() time.Duration {
	if tab, ok := m.tabByID(tabEvents); ok {
		if pane, ok := tab.pane.(*EventsPane); ok && m.activeTab == tabEvents && pane.FollowEnabled() {
			return followRefreshInterval
		}
	}
	return refreshInterval
}

func parseCommand(value string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], fields[1:]
}

func summaryMessage(action, value string, err error) string {
	if err != nil {
		return fmt.Sprintf("%s %s failed: %v", action, value, err)
	}
	return fmt.Sprintf("%s %s completed", action, value)
}
