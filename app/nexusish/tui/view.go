package tui

import "github.com/charmbracelet/lipgloss"

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	parts := make([]string, 0, 6)
	if bar := m.activeSubTabBar(); bar != "" {
		parts = append(parts, bar)
	}
	parts = append(parts, m.renderBody())
	if m.notifBar != nil && m.notifBar.Active() {
		parts = append(parts, m.notifBar.View())
	}
	parts = append(parts, m.inputBar.View(m.activeTab))
	parts = append(parts, m.tabBar.View())
	parts = append(parts, m.statusBar.View())
	base := lipgloss.JoinVertical(lipgloss.Left, parts...)
	if m.showHelp {
		return m.help.View(base)
	}
	return base
}

func (m model) renderBody() string {
	_, bodyHeight, _, _, _, _ := m.layoutHeights()
	if m.loadErr != nil {
		return bodyStyle.Width(m.width).Height(bodyHeight).Render(errorStyle.Render(m.loadErr.Error()))
	}
	content := ""
	if pane, ok := m.activeContentPane(); ok {
		content = pane.View()
	}
	return bodyStyle.Width(m.width).Height(bodyHeight).Render(content)
}
