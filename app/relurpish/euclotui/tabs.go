package euclotui

import "codeburg.org/lexbit/relurpify/app/relurpish/tui"

// RegisterEucloTabs adds the chat tab for the euclo agent.
// It is called via EucloPlugin.SetupTabs from tui.newRootModel.
func RegisterEucloTabs(reg *tui.TabRegistry) {
	reg.Register(tui.TabDefinition{
		ID: tui.TabChat, Label: "chat", AgentFilter: []string{"euclo"},
		SubTabs: []tui.SubTabDefinition{
			{ID: tui.SubTabChatLocalRead, Label: "local-read-only"},
			{ID: tui.SubTabChatLocalEdit, Label: "local-edit-on"},
			{ID: tui.SubTabChatOnlineRead, Label: "online-read-on"},
			{ID: tui.SubTabChatOnlineEdit, Label: "online-edit-on"},
		},
	})
}
