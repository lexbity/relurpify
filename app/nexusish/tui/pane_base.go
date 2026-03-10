package tui

import "strings"

type paneState struct {
	state  RuntimeState
	filter string
	width  int
	height int
}

func (p *paneState) SetData(state RuntimeState) {
	p.state = state
}

func (p *paneState) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *paneState) SetFilter(query string) {
	p.filter = strings.TrimSpace(strings.ToLower(query))
}

func (p paneState) panelWidth() int {
	return max(p.width-2, 20)
}

type subtabPaneState struct {
	paneState
	subtabs SubTabBar
}

func (p *subtabPaneState) initTabs(tabs []SubTab) {
	p.subtabs.SetTabs(tabs)
}

func (p *subtabPaneState) SetSize(width, height int) {
	p.paneState.SetSize(width, height)
	p.subtabs.SetWidth(width)
}

func (p *subtabPaneState) HasSubTabs() bool { return true }

func (p *subtabPaneState) SubTabBar() string { return p.subtabs.View() }

func (p *subtabPaneState) activeSubtab() int { return p.subtabs.Active() }

func (p *subtabPaneState) stepSubtab(delta, maxIndex int) {
	p.subtabs.SetActive(min(maxIndex, max(0, p.subtabs.Active()+delta)))
}
