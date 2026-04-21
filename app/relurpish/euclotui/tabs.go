package euclotui

import "codeburg.org/lexbit/relurpify/app/relurpish/tui"

// RegisterEucloTabs adds the chat, planner, and debug tabs for the euclo agent.
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
	reg.Register(tui.TabDefinition{
		ID: tui.TabPlanner, Label: "planner", AgentFilter: []string{"euclo"},
		SubTabs: []tui.SubTabDefinition{
			{ID: tui.SubTabPlannerExplore, Label: "explore"},
			{ID: tui.SubTabPlannerAnalyze, Label: "analyze"},
			{ID: tui.SubTabPlannerFinalize, Label: "finalize"},
		},
	})
	reg.Register(tui.TabDefinition{
		ID: tui.TabDebug, Label: "debug", AgentFilter: []string{"euclo"},
		SubTabs: []tui.SubTabDefinition{
			{ID: tui.SubTabDebugTest, Label: "test"},
			{ID: tui.SubTabDebugBenchmark, Label: "benchmark"},
			{ID: tui.SubTabDebugTrace, Label: "trace"},
			{ID: tui.SubTabDebugPlanDiff, Label: "live-plan-diff"},
		},
	})
}
