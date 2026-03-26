package euclotui

import "github.com/charmbracelet/lipgloss"

// Local color palette — mirrors the values from tui/styles.go so that
// euclotui can render consistently without importing the tui package's
// unexported style variables.
var (
	colorPrimary   = lipgloss.Color("39")
	colorSecondary = lipgloss.Color("86")
	colorSuccess   = lipgloss.Color("42")
	colorWarning   = lipgloss.Color("220")
	colorError     = lipgloss.Color("196")
	colorDim       = lipgloss.Color("241")
)

var (
	sectionHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSecondary)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	filePathStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary)

	completedStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	inProgressStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	diffRemoveStyle = lipgloss.NewStyle().
			Foreground(colorError)

	pendingStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	panelItemActiveStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	// Euclo-specific styles.
	eucloFrameStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	eucloPhaseStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	eucloPhaseActiveStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)

	eucloPhaseCompletedStyle = lipgloss.NewStyle().
					Foreground(colorSuccess)

	eucloPhasePendingStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	eucloFindingCriticalStyle = lipgloss.NewStyle().
					Foreground(colorError).
					Bold(true)

	eucloFindingWarningStyle = lipgloss.NewStyle().
					Foreground(colorWarning)

	eucloFindingInfoStyle = lipgloss.NewStyle().
				Foreground(colorDim)
)
