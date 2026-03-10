package tui

func (m model) tabByID(id tabID) (tabSpec, bool) {
	for _, tab := range m.tabs {
		if tab.id == id {
			return tab, true
		}
	}
	return tabSpec{}, false
}

func (m model) tabByKey(key string) (tabSpec, bool) {
	for _, tab := range m.tabs {
		if tab.key == key {
			return tab, true
		}
	}
	return tabSpec{}, false
}

func (m model) activeKeyPane() (keyPane, bool) {
	tab, ok := m.tabByID(m.activeTab)
	if !ok || tab.pane == nil {
		return nil, false
	}
	key, ok := tab.pane.(keyPane)
	return key, ok
}

func (m model) activeContentPane() (pane, bool) {
	tab, ok := m.tabByID(m.activeTab)
	if !ok || tab.pane == nil {
		return nil, false
	}
	return tab.pane, true
}

func (m model) activeSubTabbedPane() (subTabbedPane, bool) {
	tab, ok := m.tabByID(m.activeTab)
	if !ok || tab.pane == nil {
		return nil, false
	}
	p, ok := tab.pane.(subTabbedPane)
	return p, ok
}

func (m model) activeSubTabBar() string {
	if p, ok := m.activeSubTabbedPane(); ok && p.HasSubTabs() {
		return p.SubTabBar()
	}
	return ""
}

func (m model) activeFilterablePane() (filterablePane, bool) {
	tab, ok := m.tabByID(m.activeTab)
	if !ok || tab.pane == nil {
		return nil, false
	}
	p, ok := tab.pane.(filterablePane)
	return p, ok
}

func (m model) activeCommandablePane() (commandablePane, bool) {
	tab, ok := m.tabByID(m.activeTab)
	if !ok || tab.pane == nil {
		return nil, false
	}
	p, ok := tab.pane.(commandablePane)
	return p, ok
}
