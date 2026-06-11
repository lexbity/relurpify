package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
)

// CommandHandler is a function that handles a slash command and returns the updated model.
type CommandHandler func(m *RootModel, args []string) (*RootModel, tea.Cmd)

// Command describes a slash command.
type Command struct {
	Name        string
	Description string
	Usage       string
	Handler     CommandHandler
	// TabFilter restricts the command to specific tabs. Empty = available in all tabs.
	TabFilter []TabID
	// SubTabFilter restricts the command to specific subtabs. Empty = available in all subtabs.
	SubTabFilter []SubTabID
}

// CommandRegistry holds a set of slash commands and supports context-aware lookup.
type CommandRegistry struct {
	cmds []Command
}

// NewCommandRegistry returns an empty CommandRegistry.
func NewCommandRegistry() *CommandRegistry { return &CommandRegistry{} }

// Register appends a command to the registry.
func (r *CommandRegistry) Register(cmd Command) {
	r.cmds = append(r.cmds, cmd)
}

// Match returns commands eligible for the given tab/subtab context that also
// match the name prefix (empty prefix = all eligible commands).
func (r *CommandRegistry) Match(prefix string, tabID TabID, subTabID SubTabID) []Command {
	var out []Command
	for _, cmd := range r.cmds {
		if !r.eligible(cmd, tabID, subTabID) {
			continue
		}
		if prefix == "" {
			out = append(out, cmd)
			continue
		}
		if strings.HasPrefix(cmd.Name, prefix) {
			out = append(out, cmd)
		}
	}
	return out
}

func (r *CommandRegistry) eligible(cmd Command, tabID TabID, subTabID SubTabID) bool {
	if len(cmd.TabFilter) > 0 {
		found := false
		for _, t := range cmd.TabFilter {
			if t == tabID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(cmd.SubTabFilter) > 0 {
		found := false
		for _, st := range cmd.SubTabFilter {
			if st == subTabID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Lookup finds a command by name (ignores tab context).
func (r *CommandRegistry) Lookup(name string) (Command, bool) {
	for _, cmd := range r.cmds {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return Command{}, false
}

// All returns all registered commands sorted by name.
func (r *CommandRegistry) All() []Command {
	out := make([]Command, len(r.cmds))
	copy(out, r.cmds)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

var rootCommandRegistry *CommandRegistry

func registerUniversalCommands(r *CommandRegistry) {
	registerContextualCommands(r)
	for _, cmd := range []Command{
		{Name: "workspace", Description: "Switch workspace root", Usage: "/workspace [path]", Handler: rootHandleWorkspace},
		{Name: "permissions", Description: "Open sandbox permissions", Usage: "/permissions", Handler: rootHandlePermissions},
		{Name: "securityguard", Description: "Open SecurityGuard filters", Usage: "/securityguard", Handler: rootHandleSecurityGuard},
		{Name: "filescopes", Description: "Focus file scope controls", Usage: "/filescopes", Handler: rootHandleFileScopes},
		{Name: "doctor", Description: "Show workspace doctor report", Usage: "/doctor", Handler: rootHandleDoctor},
		{Name: "model", Description: "Inspect or set the active model", Usage: "/model [model_name]", Handler: rootHandleModel},
		{Name: "model-provider", Description: "Open model provider configuration", Usage: "/model-provider", Handler: rootHandleModelProvider},
		{Name: "keybindings", Description: "Open keybinding configuration", Usage: "/keybindings", Handler: rootHandleKeybindings},
		{Name: "help", Description: "Show available commands", Usage: "/help [command]", Handler: rootHandleHelp},
		{Name: "mode", Description: "Set agent mode", Usage: "/mode <mode>", Handler: rootHandleMode},
		{Name: "agent", Description: "Switch agent type", Usage: "/agent <name>", Handler: rootHandleAgent},
		{Name: "strategy", Description: "Set execution strategy", Usage: "/strategy <strategy>", Handler: rootHandleStrategy},
		{Name: "stop", Description: "Stop current run", Usage: "/stop", Handler: rootHandleStop},
		{Name: "retry", Description: "Retry last prompt", Usage: "/retry", Handler: rootHandleRetry},
		{Name: "export", Description: "Export session", Usage: "/export [md|json] [path]", Handler: rootHandleExport},
		{Name: "workflows", Description: "List persisted workflows", Usage: "/workflows [limit]", Handler: rootHandleWorkflows},
		{Name: "workflow", Description: "Inspect one workflow", Usage: "/workflow <workflow-id>", Handler: rootHandleWorkflow},
		{Name: "rerun", Description: "Replay a workflow from a step", Usage: "/rerun <workflow-id> <step-id>", Handler: rootHandleRerun},
		{Name: "cancelwf", Description: "Mark a workflow canceled", Usage: "/cancelwf <workflow-id>", Handler: rootHandleCancelWorkflow},
		{Name: "resume", Description: "Resume architect execution from a workflow", Usage: "/resume <workflow-id> | /resume latest", Handler: rootHandleResume},
		{Name: "hitl", Description: "Show pending HITL approvals", Usage: "/hitl", Handler: rootHandleHITL},
		{Name: "queue", Description: "Queue a task for sequential execution", Usage: "/queue <instruction>", Handler: rootHandleQueueTask},
		{Name: "service", Description: "Service management commands", Usage: "/service <stop|restart|restart-all> <id>", Handler: rootHandleService, TabFilter: []TabID{TabAIProvider}},
	} {
		r.Register(cmd)
	}
}

func registerContextualCommands(reg *CommandRegistry) {
	if reg == nil {
		return
	}
	for _, cmd := range []Command{
		{Name: "save", Description: "Save and reload sandbox manifest", Usage: "/save", Handler: rootHandleSave, TabFilter: []TabID{TabSandbox}},
		{Name: "backend", Description: "Switch sandbox backend", Usage: "/backend [type]", Handler: rootHandleBackend, TabFilter: []TabID{TabSandbox}},
		{Name: "test-latency", Description: "Test provider latency", Usage: "/test-latency", Handler: rootHandleTestLatency, TabFilter: []TabID{TabAIProvider}},
		{Name: "save-model", Description: "Save model configuration", Usage: "/save-model", Handler: rootHandleSaveModel, TabFilter: []TabID{TabAIProvider}},
		{Name: "test-rule", Description: "Test a security rule against input", Usage: "/test-rule", Handler: rootHandleTestRule, TabFilter: []TabID{TabSecurityGuard}},
		{Name: "restart-all", Description: "Restart all services", Usage: "/restart-all", Handler: rootHandleRestartAll, TabFilter: []TabID{TabAIProvider}},
		{Name: "reset-bindings", Description: "Reset all keybindings to defaults", Usage: "/reset-bindings", Handler: rootHandleResetBindings, TabFilter: []TabID{TabKeybindings}},
		{Name: "diff-apply-all", Description: "Apply all pending diffs", Usage: "/diff-apply-all", Handler: rootHandleDiffApplyAll, TabFilter: []TabID{TabDiff}},
		{Name: "diff-revert", Description: "Revert selected diff", Usage: "/diff-revert", Handler: rootHandleDiffRevert, TabFilter: []TabID{TabDiff}},
		{Name: "diff-view", Description: "Toggle diff view mode", Usage: "/diff-view", Handler: rootHandleDiffView, TabFilter: []TabID{TabDiff}},
	} {
		reg.Register(cmd)
	}
}

func RegisterEucloCommands(reg *CommandRegistry) {
	if reg == nil {
		return
	}
	for _, cmd := range []Command{
		{Name: "add", Description: "Add file to context", Usage: "/add <path>", Handler: rootHandleAdd, TabFilter: []TabID{TabChat}},
		{Name: "remove", Description: "Remove file from context", Usage: "/remove <path>", Handler: rootHandleRemove, TabFilter: []TabID{TabChat}},
		{Name: "context", Description: "Show current context", Usage: "/context", Handler: rootHandleContext, TabFilter: []TabID{TabChat}},
		{Name: "clear", Description: "Clear chat history", Usage: "/clear", Handler: rootHandleClear, TabFilter: []TabID{TabChat}},
		{Name: "approve", Description: "Approve pending changes", Usage: "/approve", Handler: rootHandleApprove, TabFilter: []TabID{TabChat}},
		{Name: "reject", Description: "Reject pending changes", Usage: "/reject", Handler: rootHandleReject, TabFilter: []TabID{TabChat}},
		{Name: "diff", Description: "Toggle diff expansion", Usage: "/diff [index|path]", Handler: rootHandleDiff, TabFilter: []TabID{TabChat}},
		{Name: "parallel", Description: "Toggle parallel runs", Usage: "/parallel on|off", Handler: rootHandleParallel, TabFilter: []TabID{TabChat}},
		{Name: "commit", Description: "Commit modified files to git", Usage: "/commit [message]", Handler: rootHandleCommit, TabFilter: []TabID{TabChat}},
		{Name: "local-review", Description: "Show git diff stat for current changes", Usage: "/local-review", Handler: rootHandleLocalReview, TabFilter: []TabID{TabChat}},
		{Name: "checkpoint", Description: "Save a named session checkpoint", Usage: "/checkpoint [label]", Handler: rootHandleCheckpoint, TabFilter: []TabID{TabChat}},
		{Name: "compact", Description: "Compress chat history to a summary", Usage: "/compact", Handler: rootHandleCompact, TabFilter: []TabID{TabChat}},
	} {
		reg.Register(cmd)
	}
}

func registerPlannerCommands(_ *CommandRegistry) {
	// Planner-specific commands to be added here as they are implemented.
}

func init() {
	rootCommandRegistry = NewCommandRegistry()
	registerUniversalCommands(rootCommandRegistry)
	registerPlannerCommands(rootCommandRegistry)
}

// executeCommand dispatches a command by name.
func executeCommand(m *RootModel, name string, args []string) (*RootModel, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	reg := m.cmdReg
	if reg == nil {
		reg = rootCommandRegistry
	}
	cmd, ok := reg.Lookup(name)
	if !ok {
		m.addSystemMessage(fmt.Sprintf("Unknown command: /%s. Try /help.", name))
		return m, nil
	}
	return cmd.Handler(m, args)
}

// listCommandsSorted returns all commands sorted alphabetically (used by InputBar palette).
func listCommandsSorted() []Command {
	return rootCommandRegistry.All()
}

// --- handlers ---

func rootHandleHelp(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 0 {
		if cmd, ok := rootCommandRegistry.Lookup(args[0]); ok {
			m.addSystemMessage(fmt.Sprintf("%s - %s\nUsage: %s", cmd.Name, cmd.Description, cmd.Usage))
			return m, nil
		}
	}
	var b strings.Builder
	b.WriteString("Available commands:\n\n")
	for _, cmd := range rootCommandRegistry.All() {
		fmt.Fprintf(&b, "  %s - %s\n", cmd.Usage, cmd.Description)
	}
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleWorkspace(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		workspace := ""
		if m.sharedSess != nil {
			workspace = m.sharedSess.Workspace
		}
		if workspace == "" && m.runtime != nil {
			workspace = m.runtime.SessionInfo().Workspace
		}
		if workspace == "" {
			m.addSystemMessage("Workspace: (unset)")
		} else {
			m.addSystemMessage(fmt.Sprintf("Workspace: %s", workspace))
		}
		m.setActiveTab(TabWelcome)
		return m, nil
	}
	workspace := strings.TrimSpace(args[0])
	if workspace == "" {
		m.addSystemMessage("Usage: /workspace [path]")
		return m, nil
	}
	if m.sharedSess != nil {
		m.sharedSess.Workspace = workspace
	}
	m.scope = workspace
	if m.inputBar != nil {
		m.inputBar.SetWorkspace(workspace)
	}
	if m.store != nil {
		m.store = NewSessionStore(workspace)
	}
	if m.baseSurface != nil {
		m.baseSurface.SetStore(m.store)
		m.baseSurface.Refresh()
	}
	m.setActiveTab(TabWelcome)
	m.addSystemMessage(fmt.Sprintf("Workspace set to %s", workspace))
	if m.session != nil {
		return m, m.session.Init()
	}
	return m, nil
}

func rootHandlePermissions(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	_ = m.switchActiveAgent("none")
	m.setActiveTab(TabSandbox)
	if m.baseSurface != nil {
		m.baseSurface.FocusFilescopes()
	}
	m.addSystemMessage("Opened sandbox permissions")
	return m, nil
}

func rootHandleSecurityGuard(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	_ = m.switchActiveAgent("none")
	m.setActiveTab(TabSecurityGuard)
	if m.baseSurface != nil {
		m.baseSurface.OpenSecurityGuard()
	}
	m.addSystemMessage("Opened SecurityGuard filters")
	return m, nil
}

func rootHandleFileScopes(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	_ = m.switchActiveAgent("none")
	m.setActiveTab(TabSandbox)
	if m.baseSurface != nil {
		m.baseSurface.FocusFilescopes()
	}
	m.addSystemMessage("Focused file scopes")
	return m, nil
}

func rootHandleDoctor(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	_ = m.switchActiveAgent("none")
	m.setActiveTab(TabDoctor)
	if m.baseSurface != nil {
		m.baseSurface.OpenDoctor()
	}
	workspace := ""
	model := ""
	provider := ""
	if m.sharedSess != nil {
		workspace = m.sharedSess.Workspace
		model = m.sharedSess.Model
		provider = m.sharedSess.Provider
	}
	if m.runtime != nil {
		info := m.runtime.SessionInfo()
		if workspace == "" {
			workspace = info.Workspace
		}
		if model == "" {
			model = info.Model
		}
		if provider == "" {
			provider = info.Provider
		}
	}
	m.addSystemMessage(fmt.Sprintf("Doctor: workspace=%s provider=%s model=%s", workspace, provider, model))
	return m, nil
}

func rootHandleModel(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		current := ""
		if m.sharedSess != nil {
			current = m.sharedSess.Model
		}
		available := ""
		if m.runtime != nil {
			if models, err := m.runtime.InferenceModels(context.Background()); err == nil && len(models) > 0 {
				available = fmt.Sprintf("\nAvailable: %s", strings.Join(models, ", "))
			}
		}
		_ = m.switchActiveAgent("none")
		m.setActiveTab(TabAIProvider)
		if m.baseSurface != nil {
			m.baseSurface.OpenAIProvider()
		}
		m.addSystemMessage(fmt.Sprintf("Current model: %s%s", current, available))
		return m, nil
	}
	model := strings.TrimSpace(args[0])
	if model == "" {
		m.addSystemMessage("Usage: /model [model_name]")
		return m, nil
	}
	if m.runtime != nil {
		if err := m.runtime.SaveModel(model); err != nil {
			m.addSystemMessage(fmt.Sprintf("Model save failed: %v", err))
			return m, nil
		}
	}
	if m.sharedSess != nil {
		m.sharedSess.Model = model
	}
	_ = m.switchActiveAgent("none")
	m.setActiveTab(TabAIProvider)
	if m.baseSurface != nil {
		m.baseSurface.OpenAIProvider()
	}
	m.addSystemMessage(fmt.Sprintf("Model set to %s", model))
	return m, nil
}

func rootHandleModelProvider(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	_ = m.switchActiveAgent("none")
	m.setActiveTab(TabAIProvider)
	if m.baseSurface != nil {
		m.baseSurface.OpenAIProvider()
	}
	info := ""
	if m.runtime != nil {
		session := m.runtime.SessionInfo()
		info = fmt.Sprintf("provider=%s model=%s", session.Provider, session.Model)
	}
	m.addSystemMessage("Opened model provider configuration" + func() string {
		if info == "" {
			return ""
		}
		return ": " + info
	}())
	return m, nil
}

func rootHandleKeybindings(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	_ = m.switchActiveAgent("none")
	m.setActiveTab(TabKeybindings)
	if m.baseSurface != nil {
		m.baseSurface.OpenKeybindings()
	}
	m.addSystemMessage("Opened keybindings")
	return m, nil
}

func rootHandleAdd(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		m.addSystemMessage("Usage: /add <path>")
		return m, nil
	}
	path := args[0]
	if m.runtime != nil {
		if err := m.runtime.AddFileToContext(path); err != nil {
			m.addSystemMessage(fmt.Sprintf("Error adding file: %v", err))
		} else {
			m.addSystemMessage(fmt.Sprintf("Added to context: %s", path))
			// Also add to shared context for immediate UI update
			if m.sharedCtx != nil {
				m.sharedCtx.AddFile(path)
			}
			// Update chat sidebar if visible
			if sidebar, ok := m.chat.(ChatSidebarController); ok {
				_ = sidebar.AddFileToSidebar(path)
			}
		}
	} else if m.chat != nil {
		return m, m.chat.AddFile(path)
	}
	return m, nil
}

func rootHandleRemove(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		m.addSystemMessage("Usage: /remove <path>")
		return m, nil
	}
	path := args[0]
	if m.runtime != nil {
		if err := m.runtime.DropFileFromContext(path); err != nil {
			m.addSystemMessage(fmt.Sprintf("Error removing file: %v", err))
		} else {
			m.addSystemMessage(fmt.Sprintf("Removed from context: %s", path))
			// Also remove from shared context for immediate UI update
			if m.sharedCtx != nil {
				m.sharedCtx.RemoveFile(path)
			}
			// Update chat sidebar if visible
			if sidebar, ok := m.chat.(ChatSidebarController); ok {
				sidebar.RemoveFileFromSidebar(path)
			}
		}
	} else if m.sharedCtx != nil {
		m.sharedCtx.RemoveFile(path)
		m.addSystemMessage(fmt.Sprintf("Removed from context: %s", path))
		// Update chat sidebar if visible
		if sidebar, ok := m.chat.(ChatSidebarController); ok {
			sidebar.RemoveFileFromSidebar(path)
		}
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
		fmt.Fprintf(&b, "  • %s\n", f)
	}
	fmt.Fprintf(&b, "\nTokens: %d / %d", m.sharedCtx.UsedTokens, m.sharedCtx.MaxTokens)
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleClear(m *RootModel, _ []string) (*RootModel, tea.Cmd) {
	if m.chat != nil {
		m.chat.ClearMessages()
		m.addSystemMessage("History cleared")
	}
	return m, nil
}

func rootHandleQueueTask(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 || strings.TrimSpace(strings.Join(args, " ")) == "" {
		m.addSystemMessage("Usage: /queue <instruction>")
		return m, nil
	}
	if m.tasks == nil {
		m.addSystemMessage("task queue unavailable")
		return m, nil
	}
	desc := strings.TrimSpace(strings.Join(args, " "))
	m.tasks.AddTask(TaskItem{
		Description: desc,
		Status:      TaskPending,
	})
	if m.session != nil {
		m.session.SyncQueuedTasks(m.tasks.Items())
	}
	m.setActiveTab(TabAIProvider)
	return m, m.dequeueNextTask()
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
	messages := m.chat.Messages()
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
			fmt.Fprintf(&b, "  %d) %s (%s)\n", i+1, c.Path, state)
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
		idx := index
		m.chat.MutateMessages(func(msgs []Message) {
			msgs[idx].Content.Changes[pos].Expanded = !msgs[idx].Content.Changes[pos].Expanded
		})
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
			fmt.Fprintf(&b, "  %d) %s\n", i+1, changes[i].Path)
		}
		m.addSystemMessage(b.String())
	} else {
		idx := index
		match := matches[0]
		m.chat.MutateMessages(func(msgs []Message) {
			msgs[idx].Content.Changes[match].Expanded = !msgs[idx].Content.Changes[match].Expanded
		})
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
		msgs = m.chat.Messages()
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
		fmt.Fprintf(&b, " - %s %s (%s)\n", req.ID, req.Permission.Action, req.Justification)
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
	m.addSystemMessage(fmt.Sprintf("Set mode to: %s", args[0]))
	return m, nil
}

func rootHandleAgent(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) == 0 {
		current := m.activeAgentName()
		available := ""
		if list := m.availableAgents(); len(list) > 0 {
			available = fmt.Sprintf("\nAvailable: %s", strings.Join(list, ", "))
		}
		m.addSystemMessage(fmt.Sprintf("Current agent: %s%s", current, available))
		return m, nil
	}
	name := args[0]
	if err := m.switchActiveAgent(name); err != nil {
		m.addSystemMessage(fmt.Sprintf("Agent switch failed: %v", err))
		return m, nil
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
		if m.chat.AllowParallel() {
			state = "on"
		}
		m.addSystemMessage(fmt.Sprintf("Parallel runs: %s", state))
		return m, nil
	}
	switch strings.ToLower(args[0]) {
	case "on", "true", "yes":
		m.chat.SetAllowParallel(true)
		m.addSystemMessage("Parallel runs enabled")
	case "off", "false", "no":
		m.chat.SetAllowParallel(false)
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

func rootHandleWorkflows(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.runtime == nil {
		m.addSystemMessage("Runtime unavailable")
		return m, nil
	}
	limit := 10
	if len(args) > 0 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(args[0])); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	workflows, err := m.runtime.ListWorkflows(limit)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Workflow lookup failed: %v", err))
		return m, nil
	}
	if len(workflows) == 0 {
		m.addSystemMessage("No workflows found")
		return m, nil
	}
	var b strings.Builder
	b.WriteString("Persisted workflows:\n")
	for _, workflow := range workflows {
		fmt.Fprintf(&b, " - %s status=%s", workflow.WorkflowID, workflow.Status)
		if workflow.CursorStepID != "" {
			fmt.Fprintf(&b, " cursor=%s", workflow.CursorStepID)
		}
		if !workflow.UpdatedAt.IsZero() {
			fmt.Fprintf(&b, " updated=%s", workflow.UpdatedAt.Format("2006-01-02 15:04:05"))
		}
		b.WriteByte('\n')
	}
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleWorkflow(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.runtime == nil {
		m.addSystemMessage("Runtime unavailable")
		return m, nil
	}
	if len(args) == 0 {
		m.addSystemMessage("Usage: /workflow <workflow-id>")
		return m, nil
	}
	details, err := m.runtime.GetWorkflow(strings.TrimSpace(args[0]))
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Workflow lookup failed: %v", err))
		return m, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Workflow %s\n", details.Workflow.WorkflowID)
	fmt.Fprintf(&b, "Status: %s\n", details.Workflow.Status)
	if details.Workflow.CursorStepID != "" {
		fmt.Fprintf(&b, "Cursor: %s\n", details.Workflow.CursorStepID)
	}
	fmt.Fprintf(&b, "Instruction: %s\n", details.Workflow.Instruction)
	if len(details.Steps) > 0 {
		b.WriteString("\nSteps:\n")
		for _, step := range details.Steps {
			fmt.Fprintf(&b, " - %s status=%s: %s\n", step.StepID, step.Status, step.Description)
		}
	}
	if len(details.Events) > 0 {
		b.WriteString("\nRecent events:\n")
		for _, event := range details.Events {
			fmt.Fprintf(&b, " - %s step=%s %s\n", event.EventType, event.StepID, event.Message)
		}
	}
	if len(details.Delegations) > 0 {
		b.WriteString("\nDelegations:\n")
		for _, delegation := range details.Delegations {
			target := delegation.TargetCapabilityID
			if target == "" {
				target = delegation.TargetProviderID
			}
			fmt.Fprintf(&b, " - %s state=%s target=%s", delegation.DelegationID, delegation.State, target)
			if delegation.TargetSessionID != "" {
				fmt.Fprintf(&b, " session=%s", delegation.TargetSessionID)
			}
			if delegation.InsertionAction != "" {
				fmt.Fprintf(&b, " insertion=%s", delegation.InsertionAction)
			}
			b.WriteByte('\n')
		}
	}
	if len(details.LinkedResources) > 0 {
		b.WriteString("\nLinked resources:\n")
		for _, ref := range details.LinkedResources {
			fmt.Fprintf(&b, " - %s\n", ref)
		}
	}
	m.addSystemMessage(b.String())
	return m, nil
}

func rootHandleRerun(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.chat == nil || m.runtime == nil {
		return m, nil
	}
	if len(args) < 2 {
		m.addSystemMessage("Usage: /rerun <workflow-id> <step-id>")
		return m, nil
	}
	details, err := m.runtime.GetWorkflow(strings.TrimSpace(args[0]))
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Workflow lookup failed: %v", err))
		return m, nil
	}
	meta := map[string]any{
		"workflow_id":        details.Workflow.WorkflowID,
		"rerun_from_step_id": strings.TrimSpace(args[1]),
	}
	cmd, _ := m.chat.StartRunWithMetadata(details.Workflow.Instruction, meta)
	return m, cmd
}

func rootHandleCancelWorkflow(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.runtime == nil {
		m.addSystemMessage("Runtime unavailable")
		return m, nil
	}
	if len(args) == 0 {
		m.addSystemMessage("Usage: /cancelwf <workflow-id>")
		return m, nil
	}
	workflowID := strings.TrimSpace(args[0])
	if err := m.runtime.CancelWorkflow(workflowID); err != nil {
		m.addSystemMessage(fmt.Sprintf("Workflow cancel failed: %v", err))
		return m, nil
	}
	m.addSystemMessage(fmt.Sprintf("Workflow %s marked canceled", workflowID))
	return m, nil
}

func rootHandleResume(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if m.chat == nil {
		return m, nil
	}
	if len(args) == 0 {
		m.addSystemMessage("Usage: /resume <workflow-id> | /resume latest")
		return m, nil
	}
	mode := ""
	if m.sharedSess != nil {
		mode = strings.TrimSpace(m.sharedSess.Mode)
	}
	if mode != "" && mode != "architect" {
		m.addSystemMessage("Resume is intended for architect mode. Set /mode architect first if needed.")
		return m, nil
	}
	meta := map[string]any{}
	prompt := strings.TrimSpace(m.chat.LastPrompt())
	target := strings.TrimSpace(args[0])
	if strings.EqualFold(target, "latest") {
		workflows, err := m.runtime.ListWorkflows(1)
		if err != nil || len(workflows) == 0 {
			m.addSystemMessage("No workflows available to resume")
			return m, nil
		}
		target = workflows[0].WorkflowID
	} else {
		target = strings.TrimSpace(args[0])
	}
	details, err := m.runtime.GetWorkflow(target)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Workflow lookup failed: %v", err))
		return m, nil
	}
	meta["workflow_id"] = details.Workflow.WorkflowID
	if prompt == "" {
		prompt = details.Workflow.Instruction
	}
	cmd, _ := m.chat.StartRunWithMetadata(prompt, meta)
	return m, cmd
}

// approveHITLRootCmd approves a HITL request with the given scope.
func approveHITLRootCmd(svc HITLServiceIface, requestID string, scope policy.GrantScope) tea.Cmd {
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
		if err := rt.SaveToolPolicy(toolName, agentspec.AgentPermissionAllow); err != nil {
			return chatSystemMsg{Text: fmt.Sprintf("Failed to save policy for %s: %v", toolName, err)}
		}
		return chatSystemMsg{Text: fmt.Sprintf("Policy for '%s' saved to manifest (always allow)", toolName)}
	}
}

// rootHandleService handles service management commands
func rootHandleService(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) < 1 {
		m.addSystemMessage("Usage: /service <stop|restart|restart-all> <id>")
		return m, nil
	}

	action := strings.ToLower(args[0])
	if m.runtime == nil {
		m.addSystemMessage("Runtime unavailable")
		return m, nil
	}
	m.setActiveTab(TabAIProvider)

	switch action {
	case "stop":
		if len(args) < 2 {
			m.addSystemMessage("Usage: /service stop <service-id>")
			return m, nil
		}
		serviceID := args[1]
		return m, func() tea.Msg {
			err := m.runtime.StopService(serviceID)
			if err != nil {
				return chatSystemMsg{Text: fmt.Sprintf("Failed to stop service %s: %v", serviceID, err)}
			}
			return chatSystemMsg{Text: fmt.Sprintf("Service %s stopped", serviceID)}
		}

	case "restart":
		if len(args) < 2 {
			m.addSystemMessage("Usage: /service restart <service-id>")
			return m, nil
		}
		serviceID := args[1]
		return m, func() tea.Msg {
			err := m.runtime.RestartService(context.Background(), serviceID)
			if err != nil {
				return chatSystemMsg{Text: fmt.Sprintf("Failed to restart service %s: %v", serviceID, err)}
			}
			return chatSystemMsg{Text: fmt.Sprintf("Service %s restarted", serviceID)}
		}

	case "restart-all":
		return m, func() tea.Msg {
			err := m.runtime.RestartAllServices(context.Background())
			if err != nil {
				return chatSystemMsg{Text: fmt.Sprintf("Failed to restart all services: %v", err)}
			}
			return chatSystemMsg{Text: "All services restarted"}
		}

	default:
		m.addSystemMessage("Unknown service action. Use: stop, restart, restart-all")
		return m, nil
	}
}

// denyHITLRootCmd denies a HITL request.
func denyHITLRootCmd(svc HITLServiceIface, requestID string) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return hitlResolvedMsg{requestID: requestID, approved: false, err: fmt.Errorf("hitl service unavailable")}
		}
		err := svc.DenyHITL(requestID, "denied in TUI")
		return hitlResolvedMsg{requestID: requestID, approved: false, err: err}
	}
}

// ── Contextual command handlers ──────────────────────────────────────────

func rootHandleSave(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	if m.baseSurface != nil {
		if s, ok := m.baseSurface.(interface{ Save() tea.Cmd }); ok {
			m.addSystemMessage("Saving sandbox manifest...")
			return m, s.Save()
		}
	}
	m.addSystemMessage("Save not available on this surface")
	return m, nil
}

func rootHandleBackend(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	if len(args) > 0 {
		m.addSystemMessage(fmt.Sprintf("Backend switch to %s requested", args[0]))
		return m, nil
	}
	m.setActiveTab(TabSandbox)
	m.addSystemMessage("Use /backend <type> to switch backend")
	return m, nil
}

func rootHandleTestLatency(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	m.setActiveTab(TabAIProvider)
	m.addSystemMessage("Testing provider latency... (not yet implemented as palette command)")
	return m, nil
}

func rootHandleSaveModel(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	m.setActiveTab(TabAIProvider)
	m.addSystemMessage("Use /model <name> to set model")
	return m, nil
}

func rootHandleTestRule(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	m.setActiveTab(TabSecurityGuard)
	m.addSystemMessage("Testing security rule... (not yet implemented as palette command)")
	return m, nil
}

func rootHandleRestartAll(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	if m.runtime == nil {
		m.addSystemMessage("Runtime unavailable")
		return m, nil
	}
	return m, func() tea.Msg {
		err := m.runtime.RestartAllServices(context.Background())
		if err != nil {
			return chatSystemMsg{Text: fmt.Sprintf("Failed to restart all services: %v", err)}
		}
		return chatSystemMsg{Text: "All services restarted"}
	}
}

func rootHandleResetBindings(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	m.setActiveTab(TabKeybindings)
	m.addSystemMessage("Reset bindings requested (not yet implemented as palette command)")
	return m, nil
}

func rootHandleDiffApplyAll(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	m.setActiveTab(TabDiff)
	m.addSystemMessage("Apply all diffs requested (not yet implemented as palette command)")
	return m, nil
}

func rootHandleDiffRevert(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	m.setActiveTab(TabDiff)
	m.addSystemMessage("Revert diff requested (not yet implemented as palette command)")
	return m, nil
}

func rootHandleDiffView(m *RootModel, args []string) (*RootModel, tea.Cmd) {
	_ = args
	m.setActiveTab(TabDiff)
	m.addSystemMessage("Toggle diff view requested (not yet implemented as palette command)")
	return m, nil
}
