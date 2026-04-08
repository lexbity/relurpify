package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary   = lipgloss.Color("39")
	colorSecondary = lipgloss.Color("86")
	colorSuccess   = lipgloss.Color("42")
	colorWarning   = lipgloss.Color("220")
	colorError     = lipgloss.Color("196")
	colorDim       = lipgloss.Color("241")

	messageBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim).
			Padding(0, 2)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	sectionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSecondary)

	textStyle = lipgloss.NewStyle()

	dimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	detailStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true)

	completedStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	inProgressStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	pendingStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	filePathStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary)

	diffBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colorDim).
			Padding(0, 1)

	diffAddStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	diffRemoveStyle = lipgloss.NewStyle().
			Foreground(colorError)

	diffHeaderStyle = lipgloss.NewStyle().
			Foreground(colorSecondary)

	diffContextStyle = lipgloss.NewStyle()

	statusStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	promptBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Padding(0, 1)

	buttonStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	welcomeStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Italic(true).
			Align(lipgloss.Center)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim).
			Padding(0, 1)

	panelHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSecondary)

	panelItemStyle = lipgloss.NewStyle()

	panelItemActiveStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)
)

var (
	// New styles for rewritten components

	titleBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	tabBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("234")).
			Padding(0, 1)

	tabActiveStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorDim).
				Padding(0, 1)

	notifInfoStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("18")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	notifHITLStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("202")).
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Padding(0, 1)

	notifSuccessStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("28")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)

	notifErrorStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("124")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	inputBarNewStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Padding(0, 1)

	inputPrefixStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	paneStyle = lipgloss.NewStyle()

	taskDoneStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	taskPendingStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	taskRunningStyle = lipgloss.NewStyle().
				Foreground(colorWarning)

	helpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(1, 2).
				Background(lipgloss.Color("236"))

	// Subtab bar styles (layout.go SubTabBar).
	subtabBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	subtabBarEmptyStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("235")).
				Height(1)

	subtabActiveStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true).
				Background(lipgloss.Color("237")).
				Padding(0, 1)

	subtabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorDim).
				Padding(0, 1)

	// Guidance panel (comp_hitl_guidance.go).
	guidancePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorWarning).
				Padding(0, 1)
)
