package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	refreshInterval       = 2 * time.Second
	followRefreshInterval = 500 * time.Millisecond
)

type tabID int

const (
	tabDashboard tabID = iota + 1
	tabNodes
	tabChannels
	tabIdentity
	tabSecurity
	tabEvents
)

type stateLoadedMsg struct {
	state RuntimeState
	err   error
}

type tickMsg time.Time

type actionCompleteMsg struct {
	message string
	err     error
}

type tokenActionRequestMsg struct {
	action    string
	subjectID string
	scope     string
	tokenID   string
}

type policyActionRequestMsg struct {
	ruleID  string
	enabled bool
}

type channelActionRequestMsg struct {
	action  string
	channel string
}

type inputSubmittedMsg struct {
	mode  InputMode
	value string
}

type notifActionMsg struct {
	action string
	value  string
}

type pane interface {
	SetData(state RuntimeState)
	SetSize(width, height int)
	View() string
}

type subTabbedPane interface {
	pane
	SubTabBar() string
	HasSubTabs() bool
}

type filterablePane interface {
	pane
	SetFilter(query string)
}

type commandablePane interface {
	pane
	HandleCommand(cmd string, args []string) tea.Cmd
}

type keyPane interface {
	Update(msg tea.Msg) tea.Cmd
}

type activatablePane interface {
	SetActive(active bool)
}

type tabSpec struct {
	id    tabID
	key   string
	label string
	pane  pane
}
