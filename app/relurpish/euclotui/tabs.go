package euclotui

import "codeburg.org/lexbit/relurpify/app/relurpish/tui"

// RegisterEucloTabs adds the Euclo surface tabs.
func RegisterEucloTabs(reg *tui.TabRegistry) {
	if reg == nil {
		return
	}
	reg.Register(tui.TabDefinition{
		ID:          tui.TabChat,
		Label:       "chat",
		AgentFilter: []string{"euclo"},
		SubTabs: []tui.SubTabDefinition{
			{ID: tui.SubTabChatLocalRead, Label: "local-read-only"},
			{ID: tui.SubTabChatLocalEdit, Label: "local-edit-on"},
			{ID: tui.SubTabChatOnlineRead, Label: "online-read-on"},
			{ID: tui.SubTabChatOnlineEdit, Label: "online-edit-on"},
		},
	})
	reg.Register(tui.TabDefinition{ID: tui.TabDiff, Label: "diff", AgentFilter: []string{"euclo"}})
}
