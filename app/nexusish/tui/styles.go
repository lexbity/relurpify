package tui

import "github.com/charmbracelet/lipgloss"

var (
	warningColor = lipgloss.Color("214")
	headerBg     = lipgloss.Color("236")
	tabBg        = lipgloss.Color("234")
	accentBg     = lipgloss.Color("31")
)

var (
	subtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
	tabBarStyle = lipgloss.NewStyle().
			Background(tabBg).
			Padding(0, 1)
	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(accentBg).
			Padding(0, 1)
	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244")).
				Padding(0, 1)
	subTabBarStyle = lipgloss.NewStyle().
			Background(headerBg).
			Padding(0, 1)
	subTabActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(accentBg)
	subTabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("244"))
	bodyStyle = lipgloss.NewStyle().
			Padding(0, 1)
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("239")).
			Padding(0, 1)
	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("110"))
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203")).
			Bold(true)
	okStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("149")).
		Bold(true)
	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)
	accentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117"))
	selectedLineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(accentBg).
				Bold(true)
	inputBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("248")).
			Padding(0, 1)
	statusOnlineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("149")).Bold(true)
	statusOfflineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	notifInfoStyle     = lipgloss.NewStyle().
				Background(lipgloss.Color("238")).
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)
	notifWarnStyle = notifInfoStyle.Copy().
			Background(lipgloss.Color("94"))
	notifErrorStyle = notifInfoStyle.Copy().
			Background(lipgloss.Color("52"))
	notifActionStyle = notifInfoStyle.Copy().
				Background(lipgloss.Color("24"))
	badgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("52")).
			Padding(0, 1)
	helpOverlayStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(lipgloss.Color("252")).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(accentBg).
				Padding(1, 2)
)
