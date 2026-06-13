package tui

// Shared command, label, runtime, and formatting identifiers used by the TUI.
const (
	CapArgs      = "args"
	CmdAdd       = "add"
	CmdCommit    = "commit"
	CmdCompact   = "compact"
	CmdDiff      = "diff"
	CmdModel     = "model"
	CmdOff       = "off"
	CmdOn        = "on"
	CmdWorkspace = "workspace"

	LabelAIProvider = "ai provider"

	RuntimeDocker    = "docker"
	RuntimeGVisor    = "gvisor"
	RuntimeOllama    = "ollama"
	RuntimeLocal     = "local"
	RuntimeArchitect = "architect"

	SearchFilter = "filter"
	SearchSearch = "search"
	SearchPrompt = "prompt"
	SearchSlash  = "slash"
	SearchShell  = "shell"

	ModeExecution = "execution"

	FormatJSON = "json"
	FormatMD   = "md"

	NodeIDTools   = "tools"
	SourceBuiltIn = "built-in"
)
