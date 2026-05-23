package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorBackground = lipgloss.AdaptiveColor{Light: "#f4f4f5", Dark: "#1f1f23"}
	colorSurface    = lipgloss.AdaptiveColor{Light: "#d8d8dd", Dark: "#2b2f36"}
	colorPrimary    = lipgloss.AdaptiveColor{Light: "#005f87", Dark: "#7fd7ff"}
	colorSecondary  = lipgloss.AdaptiveColor{Light: "#4f6d7a", Dark: "#8ad6c2"}
	colorSuccess    = lipgloss.AdaptiveColor{Light: "#2f7d32", Dark: "#87d75f"}
	colorWarning    = lipgloss.AdaptiveColor{Light: "#9a5f00", Dark: "#ffd75f"}
	colorError      = lipgloss.AdaptiveColor{Light: "#b00020", Dark: "#ff8787"}
	colorDim        = lipgloss.AdaptiveColor{Light: "#62666d", Dark: "#8d94a1"}

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

	promptBarStyle = lipgloss.NewStyle().
			Background(colorSurface).
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

	agentStripStyle = lipgloss.NewStyle().
			Background(colorSurface).
			Foreground(colorSecondary).
			Bold(true).
			Padding(0, 1)

	agentStripActiveStyle = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorBackground).
				Bold(true).
				Padding(0, 1)
)

var (
	tabBarStyle = lipgloss.NewStyle().
			Background(colorSurface).
			Padding(0, 1)

	tabActiveStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Background(colorBackground).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorDim).
				Padding(0, 1)

	notifInfoStyle = lipgloss.NewStyle().
			Background(colorPrimary).
			Foreground(colorBackground).
			Padding(0, 1)

	notifHITLStyle = lipgloss.NewStyle().
			Background(colorWarning).
			Foreground(colorBackground).
			Bold(true).
			Padding(0, 1)

	notifSuccessStyle = lipgloss.NewStyle().
				Background(colorSuccess).
				Foreground(colorBackground).
				Padding(0, 1)

	notifErrorStyle = lipgloss.NewStyle().
			Background(colorError).
			Foreground(colorBackground).
			Padding(0, 1)

	inputBarNewStyle = lipgloss.NewStyle().
				Background(colorSurface).
				Padding(0, 1)

	inputBarFocusedStyle = lipgloss.NewStyle().
				Background(colorBackground).
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	inputBarBlurredStyle = lipgloss.NewStyle().
				Background(colorSurface).
				Border(lipgloss.NormalBorder()).
				BorderForeground(colorDim).
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
				BorderForeground(colorSecondary).
				Padding(1, 2).
				Background(colorBackground)

	// Subtab bar styles (layout.go SubTabBar).
	subtabBarStyle = lipgloss.NewStyle().
			Background(colorSurface).
			Padding(0, 1)

	subtabBarEmptyStyle = lipgloss.NewStyle().
				Background(colorSurface).
				Height(1)

	subtabActiveStyle = lipgloss.NewStyle().
				Foreground(colorSecondary).
				Bold(true).
				Background(colorBackground).
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
