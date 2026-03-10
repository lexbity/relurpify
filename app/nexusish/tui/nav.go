package tui

func (m model) nextTab(current tabID) tabID {
	for idx, tab := range m.tabs {
		if tab.id == current {
			return m.tabs[(idx+1)%len(m.tabs)].id
		}
	}
	if len(m.tabs) == 0 {
		return tabDashboard
	}
	return m.tabs[0].id
}

func (m model) prevTab(current tabID) tabID {
	for idx, tab := range m.tabs {
		if tab.id == current {
			if idx == 0 {
				return m.tabs[len(m.tabs)-1].id
			}
			return m.tabs[idx-1].id
		}
	}
	if len(m.tabs) == 0 {
		return tabDashboard
	}
	return m.tabs[0].id
}
