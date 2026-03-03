package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	fruntime "github.com/lexcodex/relurpify/framework/runtime"
)

// CommandHandler is a function that handles a slash command and returns the updated model.
type CommandHandler func(m *RootModel, args []string) (*RootModel, tea.Cmd)

// Command describes a slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Handler     CommandHandler
}

var rootCommandRegistry = map[string]Command{}

func init() {
	for _, cmd := range []Command{
		{Name: "help", Aliases: []string{"h", "?"}, Description: "Show available commands", Usage: "/help [command]", Handler: rootHandleHelp},
		{Name: "add", Aliases: []string{"a"}, Description: "Add file to context", Usage: "/add <path>", Handler: rootHandleAdd},
		{Name: "remove", Aliases: []string{"rm"}, Description: "Remove file from context", Usage: "/remove <path>", Handler: rootHandleRemove},
		{Name: "context", Aliases: []string{"ctx", "c"}, Description: "Show current context", Usage: "/context", Handler: rootHandleContext},
		{Name: "clear", Aliases: []string{"cls"}, Description: "Clear chat history", Usage: "/clear", Handler: rootHandleClear},
		{Name: "approve", Aliases: []string{"ap"}, Description: "Approve pending changes", Usage: "/approve", Handler: rootHandleApprove},
		{Name: "reject", Aliases: []string{"rej"}, Description: "Reject pending changes", Usage: "/reject", Handler: rootHandleReject},
		{Name: "diff", Aliases: []string{"d"}, Description: "Toggle diff expansion", Usage: "/diff [index|path]", Handler: rootHandleDiff},
		{Name: "export", Aliases: []string{"ex"}, Description: "Export session", Usage: "/export [md|json] [path]", Handler: rootHandleExport},
		{Name: "hitl", Aliases: []string{"hi"}, Description: "Show pending HITL approvals", Usage: "/hitl", Handler: rootHandleHITL},
		{Name: "mode", Aliases: []string{"m"}, Description: "Set agent mode", Usage: "/mode <mode>", Handler: rootHandleMode},
		{Name: "agent", Aliases: []string{"ag"}, Description: "Switch agent type", Usage: "/agent <name>", Handler: rootHandleAgent},
		{Name: "strategy", Aliases: []string{"s", "strat"}, Description: "Set execution strategy", Usage: "/strategy <strategy>", Handler: rootHandleStrategy},
		{Name: "parallel", Aliases: []string{"par"}, Description: "Toggle parallel runs", Usage: "/parallel on|off", Handler: rootHandleParallel},
		{Name: "stop", Aliases: []string{"cancel"}, Description: "Stop current run", Usage: "/stop", Handler: rootHandleStop},
		{Name: "retry", Aliases: []string{"re"}, Description: "Retry last prompt", Usage: "/retry", Handler: rootHandleRetry},
	} {
		rootCommandRegistry[cmd.Name] = cmd
	}
}

// executeCommand dispatches a command by name (with alias fallback).
func executeCommand(m *RootModel, name string, args []string) (*RootModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	cmd, ok := rootCommandRegistry[name]
	if !ok {
		for _, registered := range rootCommandRegistry {
			for _, alias := range registered.Aliases {
				if alias == name {
					cmd = registered
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
	}
	if !ok {
		m.addSystemMessage(fmt.Sprintf("Unknown command: /%s. Try /help.", name))
		return m, nil
	}
	return cmd.Handler(m, args)
}

// listCommandsSorted returns all commands sorted alphabetically (used by InputBar palette).
func listCommandsSorted() []Command {
	cmds := make([]Command, 0, len(rootCommandRegistry))
	for _, cmd := range rootCommandRegistry {
		cmds = append(cmds, cmd)
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

// --- handlers ---

func rootHandleHelp(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 0 {
		if cmd, ok := rootCommandRegistry[args[0]]; ok {
			m.addSystemMessage(fmt.Sprintf("%s - %s\nUsage: %s", cmd.Name, cmd.Description, cmd.Usage))
			return m, nil
		}
	}
	names := make([]string, 0, len(rootCommandRegistry))
	for name := range rootCommandRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("Available commands:\n\n")
	for _, name := range names {
		cmd := rootCommandRegistry[name]
		b.WriteString(fmt.Sprintf("  %s - %s\n", cmd.Usage, cmd.Description))
	}
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleAdd(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		m.addSystemMessage("Usage: /add <path>")
		return m, nil
	}
	if m.chat != nil {
		return m, m.chat.AddFile(args[0])
	}
	return m, nil
}

func rootHandleRemove(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		m.addSystemMessage("Usage: /remove <path>")
		return m, nil
	}
	if m.sharedCtx != nil {
		m.sharedCtx.RemoveFile(args[0])
		m.addSystemMessage(fmt.Sprintf("Removed from context: %s", args[0]))
	}
	return m, nil
}

func rootHandleContext(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.sharedCtx == nil || len(m.sharedCtx.Files) == 0 {
		m.addSystemMessage("Context is empty")
		return m, nil
	}
	var b strings.Builder
	b.WriteString("Files in context:\n\n")
	for _, f := range m.sharedCtx.Files {
		b.WriteString(fmt.Sprintf("  • %s\n", f))
	}
	b.WriteString(fmt.Sprintf("\nTokens: %d / %d", m.sharedCtx.UsedTokens, m.sharedCtx.MaxTokens))
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleClear(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.chat != nil {
		m.chat.feed.ClearMessages()
		m.addSystemMessage("History cleared")
	}
	return m, nil
}

func rootHandleApprove(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	count := m.chat.ApplyPendingChanges(StatusApproved)
	if count == 0 {
		m.addSystemMessage("No pending changes")
	} else {
		m.addSystemMessage(fmt.Sprintf("Approved %d change(s)", count))
	}
	return m, nil
}

func rootHandleReject(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	count := m.chat.ApplyPendingChanges(StatusRejected)
	if count == 0 {
		m.addSystemMessage("No pending changes")
	} else {
		m.addSystemMessage(fmt.Sprintf("Rejected %d change(s)", count))
	}
	return m, nil
}

func rootHandleDiff(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	messages := m.chat.feed.messages
	index := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleAgent && len(messages[i].Content.Changes) > 0 {
			index = i
			break
		}
	}
	if index == -1 {
		m.addSystemMessage("No recent changes to show diffs")
		return m, nil
	}
	changes := messages[index].Content.Changes
	if len(args) == 0 {
		var b strings.Builder
		b.WriteString("Recent changes:\n\n")
		for i, c := range changes {
			state := "collapsed"
			if c.Expanded {
				state = "expanded"
			}
			b.WriteString(fmt.Sprintf("  %d) %s (%s)\n", i+1, c.Path, state))
		}
		m.addSystemMessage(b.String())
		return m, nil
	}
	arg := strings.TrimSpace(args[0])
	if pos, err := strconv.Atoi(arg); err == nil {
		pos--
		if pos < 0 || pos >= len(changes) {
			m.addSystemMessage("Diff index out of range")
			return m, nil
		}
		messages[index].Content.Changes[pos].Expanded = !messages[index].Content.Changes[pos].Expanded
		m.chat.feed.refresh()
		return m, nil
	}
	var matches []int
	for i, c := range changes {
		if c.Path == arg || strings.Contains(c.Path, arg) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		m.addSystemMessage(fmt.Sprintf("No diff matched: %s", arg))
	} else if len(matches) > 1 {
		var b strings.Builder
		b.WriteString("Multiple diffs matched:\n\n")
		for _, i := range matches {
			b.WriteString(fmt.Sprintf("  %d) %s\n", i+1, changes[i].Path))
		}
		m.addSystemMessage(b.String())
	} else {
		messages[index].Content.Changes[matches[0]].Expanded = !messages[index].Content.Changes[matches[0]].Expanded
		m.chat.feed.refresh()
	}
	return m, nil
}

func rootHandleExport(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	format, path := parseExportArgs(args)
	if format == "" {
		m.addSystemMessage("Usage: /export [md|json] [path]")
		return m, nil
	}
	opts := ExportOptions{Format: format, Path: path, Limit: 200}
	if m.sharedSess != nil {
		opts.WorkspaceRoot = m.sharedSess.Workspace
	}
	if m.runtime != nil {
		artifacts := m.runtime.SessionArtifacts()
		opts.TelemetryPath = artifacts.TelemetryPath
		opts.LogPath = artifacts.LogPath
	}
	var msgs []Message
	if m.chat != nil {
		msgs = m.chat.feed.Messages()
	}
	out, err := WriteSessionExport(msgs, m.sharedSess, m.sharedCtx, opts)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Export failed: %v", err))
	} else {
		m.addSystemMessage(fmt.Sprintf("Exported session to %s", out))
	}
	return m, nil
}

func rootHandleHITL(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.runtime == nil {
		m.addSystemMessage("Runtime unavailable")
		return m, nil
	}
	pending := m.runtime.PendingHITL()
	if len(pending) == 0 {
		m.addSystemMessage("No pending approvals")
		return m, nil
	}
	var b strings.Builder
	b.WriteString("Pending approvals:\n")
	for _, req := range pending {
		b.WriteString(fmt.Sprintf(" - %s %s (%s)\n", req.ID, req.Permission.Action, req.Justification))
	}
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleMode(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		if m.sharedSess != nil && m.sharedSess.Mode != "" {
			m.addSystemMessage(fmt.Sprintf("Current mode: %s", m.sharedSess.Mode))
		} else {
			m.addSystemMessage("Current mode: (default)")
		}
		return m, nil
	}
	if m.sharedSess != nil {
		m.sharedSess.Mode = args[0]
	}
	m.titleBar.Update(0, 0)
	m.addSystemMessage(fmt.Sprintf("Set mode to: %s", args[0]))
	return m, nil
}

func rootHandleAgent(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		current := "(default)"
		if m.sharedSess != nil && m.sharedSess.Agent != "" {
			current = m.sharedSess.Agent
		}
		available := ""
		if m.runtime != nil {
			list := m.runtime.AvailableAgents()
			if len(list) > 0 {
				available = fmt.Sprintf("\nAvailable: %s", strings.Join(list, ", "))
			}
		}
		m.addSystemMessage(fmt.Sprintf("Current agent: %s%s", current, available))
		return m, nil
	}
	if m.runtime == nil {
		m.addSystemMessage("Runtime unavailable: cannot switch agent")
		return m, nil
	}
	name := args[0]
	if err := m.runtime.SwitchAgent(name); err != nil {
		m.addSystemMessage(fmt.Sprintf("Agent switch failed: %v", err))
		return m, nil
	}
	if m.sharedSess != nil {
		m.sharedSess.Agent = name
	}
	m.addSystemMessage(fmt.Sprintf("Switched agent to: %s", name))
	return m, nil
}

func rootHandleStrategy(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		if m.sharedSess != nil && m.sharedSess.Strategy != "" {
			m.addSystemMessage(fmt.Sprintf("Current strategy: %s", m.sharedSess.Strategy))
		} else {
			m.addSystemMessage("Current strategy: (auto-detect)")
		}
		return m, nil
	}
	if m.sharedSess != nil {
		m.sharedSess.Strategy = args[0]
	}
	m.addSystemMessage(fmt.Sprintf("Set strategy to: %s", args[0]))
	return m, nil
}

func rootHandleParallel(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	if len(args) == 0 {
		state := "off"
		if m.chat.allowParallel {
			state = "on"
		}
		m.addSystemMessage(fmt.Sprintf("Parallel runs: %s", state))
		return m, nil
	}
	switch strings.ToLower(args[0]) {
	case "on", "true", "yes":
		m.chat.allowParallel = true
		m.addSystemMessage("Parallel runs enabled")
	case "off", "false", "no":
		m.chat.allowParallel = false
		m.addSystemMessage("Parallel runs disabled")
	default:
		m.addSystemMessage("Usage: /parallel on|off")
	}
	return m, nil
}

func rootHandleStop(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	return m, m.chat.StopLatestRun()
}

func rootHandleRetry(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	return m, m.chat.RetryLastRun()
}

// pendingHITLSummaryCmd surfaces pending HITL via /hitl command.
func pendingHITLSummaryCmd(svc hitlService) tea.Cmd {
	if svc == nil {
		return nil
	}
	return func() tea.Msg {
		pending := svc.PendingHITL()
		if len(pending) == 0 {
			return chatSystemMsg{text: "No pending approvals"}
		}
		var b strings.Builder
		b.WriteString("Pending approvals:\n")
		for _, req := range pending {
			b.WriteString(fmt.Sprintf(" - %s %s (%s)\n", req.ID, req.Permission.Action, req.Justification))
		}
		return chatSystemMsg{text: b.String()}
	}
}

// approveHITLRootCmd approves a HITL request with the given scope.
func approveHITLRootCmd(svc hitlService, requestID string, scope fruntime.GrantScope) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return hitlResolvedMsg{requestID: requestID, approved: true, err: fmt.Errorf("hitl service unavailable")}
		}
		err := svc.ApproveHITL(requestID, "tui", scope, 0)
		return hitlResolvedMsg{requestID: requestID, approved: true, err: err}
	}
}

// savePolicyCmd persists a permanent tool policy to the agent manifest.
// action is the raw HITL action (e.g. "tool:cli_mkdir"); only "tool:X" actions are handled.
func savePolicyCmd(rt RuntimeAdapter, action string) tea.Cmd {
	if rt == nil {
		return nil
	}
	toolName := strings.TrimPrefix(action, "tool:")
	if toolName == action || toolName == "" {
		return nil // not a tool action
	}
	return func() tea.Msg {
		if err := rt.SaveToolPolicy(toolName, fruntime.AgentPermissionAllow); err != nil {
			return chatSystemMsg{text: fmt.Sprintf("Failed to save policy for %s: %v", toolName, err)}
		}
		return chatSystemMsg{text: fmt.Sprintf("Policy for '%s' saved to manifest (always allow)", toolName)}
	}
}

// denyHITLRootCmd denies a HITL request.
func denyHITLRootCmd(svc hitlService, requestID string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return hitlResolvedMsg{requestID: requestID, approved: false, err: fmt.Errorf("hitl service unavailable")}
		}
		err := svc.DenyHITL(requestID, "denied in TUI")
		return hitlResolvedMsg{requestID: requestID, approved: false, err: err}
	}
}
