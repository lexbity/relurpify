package tui

func (m model) layoutHeights() (subTab, body, notif, input, tab, status int) {
	status = 1
	tab = 1
	input = 1
	if m.notifBar != nil && m.notifBar.Active() {
		notif = 1
	}
	if p, ok := m.activeSubTabbedPane(); ok && p.HasSubTabs() {
		subTab = 1
	}
	body = m.height - status - tab - input - notif - subTab
	if body < 1 {
		body = 1
	}
	return
}
