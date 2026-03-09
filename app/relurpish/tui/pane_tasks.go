package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TaskQueueCompleteMsg is emitted when a queued task run finishes.
type TaskQueueCompleteMsg struct{ RunID string }

// TasksPane shows queued tasks and their execution status.
type TasksPane struct {
	feed             *Feed
	items            []TaskItem
	sel              int
	notifQ           *NotificationQueue
	runtime          RuntimeAdapter
	width            int
	height           int
	inspectKind      taskInspectKind
	inspectSel       int
	inspectErr       string
	workflows        []WorkflowInfo
	workflowDetail   *WorkflowDetails
	capabilityDetail *CapabilityDetail
	promptDetail     *PromptDetail
	resourceDetail   *ResourceDetail
	providerDetail   *LiveProviderDetail
	sessionDetail    *LiveProviderSessionDetail
	approvalDetail   *ApprovalDetail
	capabilities     []CapabilityInfo
	prompts          []PromptInfo
	resources        []ResourceInfo
	providers        []LiveProviderInfo
	sessions         []LiveProviderSessionInfo
	approvals        []ApprovalInfo
}

type taskInspectKind string

const (
	taskInspectNone         taskInspectKind = ""
	taskInspectWorkflows    taskInspectKind = "workflows"
	taskInspectCapabilities taskInspectKind = "capabilities"
	taskInspectPrompts      taskInspectKind = "prompts"
	taskInspectResources    taskInspectKind = "resources"
	taskInspectProviders    taskInspectKind = "providers"
	taskInspectSessions     taskInspectKind = "sessions"
	taskInspectApprovals    taskInspectKind = "approvals"
)

// NewTasksPane creates an empty tasks pane.
func NewTasksPane(rt RuntimeAdapter, notifQ *NotificationQueue) *TasksPane {
	return &TasksPane{
		feed:    NewFeed(),
		notifQ:  notifQ,
		runtime: rt,
	}
}

// SetSize resizes the pane.
func (p *TasksPane) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.feed.SetSize(w, max(1, h))
}

// AddTask appends a task to the queue.
func (p *TasksPane) AddTask(item TaskItem) {
	if item.ID == "" {
		item.ID = generateID()
	}
	if item.Status == "" {
		item.Status = TaskPending
	}
	p.items = append(p.items, item)
	p.rebuildFeed()
}

// MarkInProgress marks a task as running and stores its run ID.
func (p *TasksPane) MarkInProgress(taskID, runID string) {
	for i := range p.items {
		if p.items[i].ID == taskID {
			p.items[i].Status = TaskInProgress
			p.items[i].RunID = runID
			break
		}
	}
	p.rebuildFeed()
}

// MarkComplete marks a task done by run ID.
func (p *TasksPane) MarkComplete(runID string) {
	for i := range p.items {
		if p.items[i].RunID == runID {
			p.items[i].Status = TaskCompleted
			if p.notifQ != nil {
				p.notifQ.Push(NotificationItem{
					Kind:      NotifKindTaskDone,
					Msg:       fmt.Sprintf("Task done: %s", p.items[i].Description),
					CreatedAt: time.Now(),
				})
			}
		}
	}
	p.rebuildFeed()
}

// NextPending returns the next pending task, if any.
func (p *TasksPane) NextPending() (TaskItem, bool) {
	for _, item := range p.items {
		if item.Status == TaskPending {
			return item, true
		}
	}
	return TaskItem{}, false
}

// Update handles tab-specific keys.
func (p *TasksPane) Update(msg tea.Msg) (*TasksPane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if p.inspectKind != taskInspectNone {
			switch msg.String() {
			case "esc", "q":
				p.closeInspector()
			case "up":
				if p.inspectSel > 0 {
					p.inspectSel--
					p.refreshSelection()
				}
			case "down":
				if p.inspectSel < p.inspectItemCount()-1 {
					p.inspectSel++
					p.refreshSelection()
				}
			case "w":
				p.openWorkflowInspector()
			case "c":
				p.openCapabilityInspector()
			case "m":
				p.openPromptInspector()
			case "r":
				if p.inspectKind == taskInspectWorkflows && p.workflowDetail != nil {
					p.openWorkflowResourceInspector()
				} else {
					p.openResourceInspector(nil)
				}
			case "p":
				p.openProviderInspector()
			case "s":
				p.openSessionInspector()
			case "a":
				p.openApprovalInspector()
			}
			return p, nil
		}
		switch msg.String() {
		case "up":
			if p.sel > 0 {
				p.sel--
			}
		case "down":
			if p.sel < len(p.items)-1 {
				p.sel++
			}
		case "w":
			p.openWorkflowInspector()
		case "c":
			p.openCapabilityInspector()
		case "m":
			p.openPromptInspector()
		case "r":
			p.openResourceInspector(nil)
		case "p":
			p.openProviderInspector()
		case "s":
			p.openSessionInspector()
		case "a":
			p.openApprovalInspector()
		}
	case tea.MouseMsg:
		f, cmd := p.feed.Update(msg)
		p.feed = f
		return p, cmd
	}
	return p, nil
}

// View renders the tasks list.
func (p *TasksPane) View() string {
	if p.inspectKind != taskInspectNone {
		return p.renderInspector()
	}
	if len(p.items) == 0 {
		return welcomeStyle.Render("No tasks queued. Type a task description and press Enter.\n\nTasks inspection: [w] workflows  [c] capabilities  [m] prompts  [r] resources  [p] providers  [s] sessions  [a] approvals")
	}
	var b strings.Builder
	for i, item := range p.items {
		icon := "☐"
		style := taskPendingStyle
		switch item.Status {
		case TaskCompleted:
			icon = "✓"
			style = taskDoneStyle
		case TaskInProgress:
			icon = "●"
			style = taskRunningStyle
		}
		line := fmt.Sprintf("%s  %s", icon, style.Render(item.Description))
		if item.Agent != "" {
			line += dimStyle.Render(fmt.Sprintf("  [%s]", item.Agent))
		}
		if i == p.sel {
			line = panelItemActiveStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[w] workflows  [c] capabilities  [m] prompts  [r] resources  [p] providers  [s] sessions  [a] approvals"))
	return b.String()
}

func (p *TasksPane) rebuildFeed() {
	// Tasks pane uses direct rendering, not a Feed of Messages.
	_ = p.feed
}

// HandleInputSubmit adds a new task from the input bar.
func (p *TasksPane) HandleInputSubmit(value string) tea.Cmd {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	p.AddTask(TaskItem{
		Description: strings.TrimSpace(value),
		Status:      TaskPending,
	})
	return nil
}

func (p *TasksPane) openWorkflowInspector() {
	p.inspectKind = taskInspectWorkflows
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
	if p.runtime == nil {
		p.inspectErr = "runtime unavailable"
		p.workflows = nil
		return
	}
	workflows, err := p.runtime.ListWorkflows(20)
	if err != nil {
		p.inspectErr = err.Error()
		p.workflows = nil
		return
	}
	p.workflows = workflows
	p.refreshSelection()
}

func (p *TasksPane) openCapabilityInspector() {
	p.inspectKind = taskInspectCapabilities
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.promptDetail = nil
	p.resourceDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
	if p.runtime == nil {
		p.inspectErr = "runtime unavailable"
		p.capabilities = nil
		return
	}
	p.capabilities = p.runtime.ListCapabilities()
	p.refreshSelection()
}

func (p *TasksPane) openPromptInspector() {
	p.inspectKind = taskInspectPrompts
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.promptDetail = nil
	p.resourceDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
	if p.runtime == nil {
		p.inspectErr = "runtime unavailable"
		p.prompts = nil
		return
	}
	p.prompts = p.runtime.ListPrompts()
	p.refreshSelection()
}

func (p *TasksPane) openResourceInspector(workflowRefs []string) {
	p.inspectKind = taskInspectResources
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.promptDetail = nil
	p.resourceDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
	if p.runtime == nil {
		p.inspectErr = "runtime unavailable"
		p.resources = nil
		return
	}
	p.resources = p.runtime.ListResources(workflowRefs)
	p.refreshSelection()
}

func (p *TasksPane) openWorkflowResourceInspector() {
	refs := []string(nil)
	if p.workflowDetail != nil {
		refs = append(refs, p.workflowDetail.LinkedResources...)
	}
	p.openResourceInspector(refs)
}

func (p *TasksPane) openProviderInspector() {
	p.inspectKind = taskInspectProviders
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.promptDetail = nil
	p.resourceDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
	if p.runtime == nil {
		p.inspectErr = "runtime unavailable"
		p.providers = nil
		return
	}
	p.providers = p.runtime.ListLiveProviders()
	p.refreshSelection()
}

func (p *TasksPane) openSessionInspector() {
	p.inspectKind = taskInspectSessions
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.promptDetail = nil
	p.resourceDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
	if p.runtime == nil {
		p.inspectErr = "runtime unavailable"
		p.sessions = nil
		return
	}
	p.sessions = p.runtime.ListLiveSessions()
	p.refreshSelection()
}

func (p *TasksPane) openApprovalInspector() {
	p.inspectKind = taskInspectApprovals
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.promptDetail = nil
	p.resourceDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
	if p.runtime == nil {
		p.inspectErr = "runtime unavailable"
		p.approvals = nil
		return
	}
	p.approvals = p.runtime.ListApprovals()
	p.refreshSelection()
}

func (p *TasksPane) closeInspector() {
	p.inspectKind = taskInspectNone
	p.inspectSel = 0
	p.inspectErr = ""
	p.workflowDetail = nil
	p.capabilityDetail = nil
	p.promptDetail = nil
	p.resourceDetail = nil
	p.providerDetail = nil
	p.sessionDetail = nil
	p.approvalDetail = nil
}

func (p *TasksPane) inspectItemCount() int {
	switch p.inspectKind {
	case taskInspectWorkflows:
		return len(p.workflows)
	case taskInspectCapabilities:
		return len(p.capabilities)
	case taskInspectPrompts:
		return len(p.prompts)
	case taskInspectResources:
		return len(p.resources)
	case taskInspectProviders:
		return len(p.providers)
	case taskInspectSessions:
		return len(p.sessions)
	case taskInspectApprovals:
		return len(p.approvals)
	default:
		return 0
	}
}

func (p *TasksPane) refreshSelection() {
	if p.inspectSel < 0 {
		p.inspectSel = 0
	}
	switch p.inspectKind {
	case taskInspectWorkflows:
		if p.inspectSel >= len(p.workflows) {
			p.inspectSel = max(0, len(p.workflows)-1)
		}
		p.workflowDetail = nil
		if p.runtime == nil || len(p.workflows) == 0 {
			return
		}
		details, err := p.runtime.GetWorkflow(p.workflows[p.inspectSel].WorkflowID)
		if err != nil {
			p.inspectErr = err.Error()
			return
		}
		p.inspectErr = ""
		p.workflowDetail = details
	case taskInspectCapabilities:
		if p.inspectSel >= p.inspectItemCount() {
			p.inspectSel = max(0, p.inspectItemCount()-1)
		}
		p.capabilityDetail = nil
		if p.runtime == nil || len(p.capabilities) == 0 {
			return
		}
		detail, err := p.runtime.GetCapabilityDetail(p.capabilities[p.inspectSel].ID)
		if err != nil {
			p.inspectErr = err.Error()
			return
		}
		p.inspectErr = ""
		p.capabilityDetail = detail
	case taskInspectPrompts:
		if p.inspectSel >= p.inspectItemCount() {
			p.inspectSel = max(0, p.inspectItemCount()-1)
		}
		p.promptDetail = nil
		if p.runtime == nil || len(p.prompts) == 0 {
			return
		}
		detail, err := p.runtime.GetPromptDetail(p.prompts[p.inspectSel].PromptID)
		if err != nil {
			p.inspectErr = err.Error()
			return
		}
		p.inspectErr = ""
		p.promptDetail = detail
	case taskInspectResources:
		if p.inspectSel >= p.inspectItemCount() {
			p.inspectSel = max(0, p.inspectItemCount()-1)
		}
		p.resourceDetail = nil
		if p.runtime == nil || len(p.resources) == 0 {
			return
		}
		detail, err := p.runtime.GetResourceDetail(p.resources[p.inspectSel].ResourceID)
		if err != nil {
			p.inspectErr = err.Error()
			return
		}
		p.inspectErr = ""
		p.resourceDetail = detail
	case taskInspectProviders:
		if p.inspectSel >= p.inspectItemCount() {
			p.inspectSel = max(0, p.inspectItemCount()-1)
		}
		p.providerDetail = nil
		if p.runtime == nil || len(p.providers) == 0 {
			return
		}
		detail, err := p.runtime.GetLiveProviderDetail(p.providers[p.inspectSel].ProviderID)
		if err != nil {
			p.inspectErr = err.Error()
			return
		}
		p.inspectErr = ""
		p.providerDetail = detail
	case taskInspectSessions:
		if p.inspectSel >= p.inspectItemCount() {
			p.inspectSel = max(0, p.inspectItemCount()-1)
		}
		p.sessionDetail = nil
		if p.runtime == nil || len(p.sessions) == 0 {
			return
		}
		detail, err := p.runtime.GetLiveSessionDetail(p.sessions[p.inspectSel].SessionID)
		if err != nil {
			p.inspectErr = err.Error()
			return
		}
		p.inspectErr = ""
		p.sessionDetail = detail
	case taskInspectApprovals:
		if p.inspectSel >= p.inspectItemCount() {
			p.inspectSel = max(0, p.inspectItemCount()-1)
		}
		p.approvalDetail = nil
		if p.runtime == nil || len(p.approvals) == 0 {
			return
		}
		detail, err := p.runtime.GetApprovalDetail(p.approvals[p.inspectSel].ID)
		if err != nil {
			p.inspectErr = err.Error()
			return
		}
		p.inspectErr = ""
		p.approvalDetail = detail
	default:
		if p.inspectSel >= p.inspectItemCount() {
			p.inspectSel = max(0, p.inspectItemCount()-1)
		}
	}
}

func (p *TasksPane) renderInspector() string {
	boxWidth := p.width - 4
	if boxWidth <= 0 {
		boxWidth = 80
	}
	if boxWidth > 100 {
		boxWidth = 100
	}
	content := panelStyle.Width(boxWidth).Render(p.inspectContent())
	if p.width == 0 || p.height == 0 {
		return content
	}
	return lipgloss.Place(
		p.width,
		max(1, p.height),
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

func (p *TasksPane) inspectContent() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render(fmt.Sprintf("Tasks Inspector: %s", p.inspectTitle())))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("[w] workflows  [c] capabilities  [m] prompts  [r] resources  [p] providers  [s] sessions  [a] approvals  [esc] close"))
	b.WriteString("\n\n")
	if p.inspectErr != "" {
		b.WriteString(notifErrorStyle.Render(p.inspectErr))
		b.WriteString("\n\n")
	}
	items := p.inspectListLines()
	if len(items) == 0 {
		b.WriteString(dimStyle.Render("No items available"))
		return b.String()
	}
	for _, line := range items {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("\n")
	b.WriteString(sectionHeaderStyle.Render("Details"))
	b.WriteString("\n")
	b.WriteString(p.inspectDetail())
	return b.String()
}

func (p *TasksPane) inspectTitle() string {
	switch p.inspectKind {
	case taskInspectWorkflows:
		return "Workflows"
	case taskInspectCapabilities:
		return "Capabilities"
	case taskInspectPrompts:
		return "Prompts"
	case taskInspectResources:
		return "Resources"
	case taskInspectProviders:
		return "Providers"
	case taskInspectSessions:
		return "Sessions"
	case taskInspectApprovals:
		return "Approvals"
	default:
		return "Inspector"
	}
}

func (p *TasksPane) inspectListLines() []string {
	lines := []string{}
	switch p.inspectKind {
	case taskInspectWorkflows:
		for i, workflow := range p.workflows {
			line := fmt.Sprintf("%s  %s [%s]", workflow.WorkflowID, workflow.Instruction, workflow.Status)
			lines = append(lines, p.inspectLine(i, line))
		}
	case taskInspectCapabilities:
		for i, capability := range p.capabilities {
			line := fmt.Sprintf("%s  %s [%s]", capability.Name, capability.Kind, capability.RuntimeFamily)
			lines = append(lines, p.inspectLine(i, line))
		}
	case taskInspectPrompts:
		for i, prompt := range p.prompts {
			line := fmt.Sprintf("%s  [%s] %s", prompt.Meta.Title, prompt.Meta.RuntimeFamily, fallback(prompt.ProviderID, "local"))
			lines = append(lines, p.inspectLine(i, line))
		}
	case taskInspectResources:
		for i, resource := range p.resources {
			line := fmt.Sprintf("%s  [%s] %s", resource.Meta.Title, resource.Meta.Kind, fallback(resource.Meta.Source, "local"))
			lines = append(lines, p.inspectLine(i, line))
		}
	case taskInspectProviders:
		for i, provider := range p.providers {
			line := fmt.Sprintf("%s  [%s] %s", provider.ProviderID, provider.Kind, provider.Meta.State)
			lines = append(lines, p.inspectLine(i, line))
		}
	case taskInspectSessions:
		for i, session := range p.sessions {
			line := fmt.Sprintf("%s  provider=%s  %s", session.SessionID, session.ProviderID, session.Meta.State)
			lines = append(lines, p.inspectLine(i, line))
		}
	case taskInspectApprovals:
		for i, approval := range p.approvals {
			line := fmt.Sprintf("%s  [%s] %s", approval.ID, approval.Kind, approval.Action)
			lines = append(lines, p.inspectLine(i, line))
		}
	}
	return lines
}

func (p *TasksPane) inspectLine(idx int, content string) string {
	if idx == p.inspectSel {
		return panelItemActiveStyle.Render("> " + content)
	}
	return panelItemStyle.Render("  " + content)
}

func (p *TasksPane) inspectDetail() string {
	switch p.inspectKind {
	case taskInspectWorkflows:
		return p.workflowDetailText()
	case taskInspectCapabilities:
		if p.capabilityDetail == nil {
			return dimStyle.Render("No capability detail loaded")
		}
		capability := p.capabilityDetail
		return strings.Join([]string{
			fmt.Sprintf("ID: %s", capability.Meta.ID),
			fmt.Sprintf("Kind: %s", capability.Meta.Kind),
			fmt.Sprintf("Name: %s", capability.Meta.Title),
			fmt.Sprintf("Runtime: %s", capability.Meta.RuntimeFamily),
			fmt.Sprintf("Trust: %s", capability.Meta.TrustClass),
			fmt.Sprintf("Source: %s", fallback(capability.Meta.Source, "n/a")),
			fmt.Sprintf("Exposure: %s", capability.Exposure),
			fmt.Sprintf("Provider: %s", fallback(capability.ProviderID, "n/a")),
			fmt.Sprintf("Scope: %s", fallback(capability.Meta.Scope, "n/a")),
			fmt.Sprintf("Callable: %t", capability.Callable),
			fmt.Sprintf("Availability: %s", capability.Availability),
			fmt.Sprintf("Risk classes: %s", joinOrNA(capability.RiskClasses)),
			fmt.Sprintf("Effect classes: %s", joinOrNA(capability.EffectClasses)),
			fmt.Sprintf("Tags: %s", joinOrNA(capability.Tags)),
			fmt.Sprintf("Coordination role: %s", fallback(capability.CoordinationRole, "n/a")),
			fmt.Sprintf("Coordination task types: %s", joinOrNA(capability.CoordinationTaskTypes)),
			fmt.Sprintf("Description: %s", fallback(capability.Description, "n/a")),
		}, "\n")
	case taskInspectPrompts:
		if p.promptDetail == nil {
			return dimStyle.Render("No prompt detail loaded")
		}
		return p.promptDetailText()
	case taskInspectResources:
		if p.resourceDetail == nil {
			return dimStyle.Render("No resource detail loaded")
		}
		return p.resourceDetailText()
	case taskInspectProviders:
		if p.providerDetail == nil {
			return dimStyle.Render("No provider detail loaded")
		}
		provider := p.providerDetail
		return strings.Join([]string{
			fmt.Sprintf("Provider: %s", provider.Meta.ID),
			fmt.Sprintf("Kind: %s", provider.Meta.Kind),
			fmt.Sprintf("Trust baseline: %s", fallback(provider.TrustBaseline, "n/a")),
			fmt.Sprintf("Recoverability: %s", fallback(provider.Recoverability, "n/a")),
			fmt.Sprintf("Health: %s", fallback(provider.Meta.State, "unknown")),
			fmt.Sprintf("Captured: %s", fallback(provider.Meta.CapturedAt, "n/a")),
			fmt.Sprintf("Configured source: %s", fallback(provider.ConfiguredFrom, "n/a")),
			fmt.Sprintf("Capabilities: %s", joinOrNA(provider.CapabilityIDs)),
			fmt.Sprintf("Metadata: %s", joinOrNA(provider.Metadata)),
		}, "\n")
	case taskInspectSessions:
		if p.sessionDetail == nil {
			return dimStyle.Render("No session detail loaded")
		}
		session := p.sessionDetail
		lines := []string{
			fmt.Sprintf("Session: %s", session.Meta.ID),
			fmt.Sprintf("Provider: %s", session.ProviderID),
			fmt.Sprintf("Workflow: %s", fallback(session.WorkflowID, "n/a")),
			fmt.Sprintf("Task: %s", fallback(session.TaskID, "n/a")),
			fmt.Sprintf("Trust: %s", fallback(session.Meta.TrustClass, "n/a")),
			fmt.Sprintf("Recoverability: %s", fallback(session.Recoverability, "n/a")),
			fmt.Sprintf("Health: %s", fallback(session.Meta.State, "unknown")),
			fmt.Sprintf("Captured: %s", fallback(session.Meta.CapturedAt, "n/a")),
			fmt.Sprintf("Capabilities: %s", joinOrNA(session.CapabilityIDs)),
		}
		if len(session.MetadataSummary) > 0 {
			lines = append(lines, fmt.Sprintf("Metadata: %s", strings.Join(session.MetadataSummary, ", ")))
		}
		return strings.Join(lines, "\n")
	case taskInspectApprovals:
		if p.approvalDetail == nil {
			return dimStyle.Render("No approval detail loaded")
		}
		approval := p.approvalDetail
		lines := []string{
			fmt.Sprintf("Approval: %s", approval.Meta.ID),
			fmt.Sprintf("Kind: %s", approval.Kind),
			fmt.Sprintf("Permission type: %s", approval.PermissionType),
			fmt.Sprintf("Action: %s", approval.Action),
			fmt.Sprintf("Resource: %s", fallback(approval.Resource, "n/a")),
			fmt.Sprintf("Risk: %s", fallback(approval.Risk, "n/a")),
			fmt.Sprintf("Scope: %s", fallback(approval.Scope, "n/a")),
			fmt.Sprintf("State: %s", fallback(approval.Meta.State, "pending")),
			fmt.Sprintf("Requested: %s", approval.RequestedAt.Format("2006-01-02 15:04:05")),
			fmt.Sprintf("Justification: %s", fallback(approval.Justification, "n/a")),
		}
		if len(approval.Metadata) > 0 {
			lines = append(lines, fmt.Sprintf("Metadata: %s", joinStringMap(approval.Metadata)))
		}
		return strings.Join(lines, "\n")
	default:
		return dimStyle.Render("No inspector active")
	}
}

func (p *TasksPane) workflowDetailText() string {
	if p.workflowDetail == nil {
		if len(p.workflows) == 0 {
			return dimStyle.Render("No workflow selected")
		}
		return dimStyle.Render("Loading workflow details...")
	}
	detail := p.workflowDetail
	lines := []string{
		fmt.Sprintf("Workflow: %s", detail.Workflow.WorkflowID),
		fmt.Sprintf("Status: %s", detail.Workflow.Status),
		fmt.Sprintf("Task: %s", fallback(detail.Workflow.TaskID, "n/a")),
		fmt.Sprintf("Cursor: %s", fallback(detail.Workflow.CursorStepID, "n/a")),
		fmt.Sprintf("Instruction: %s", fallback(detail.Workflow.Instruction, "n/a")),
		fmt.Sprintf("Delegations: %d", len(detail.Delegations)),
		fmt.Sprintf("Artifacts: %d", len(detail.WorkflowArtifacts)),
		fmt.Sprintf("Persisted provider snapshots: %d", len(detail.Providers)),
		fmt.Sprintf("Persisted provider sessions: %d", len(detail.ProviderSessions)),
		fmt.Sprintf("Linked resources: %s", joinOrNA(detail.LinkedResources)),
	}
	if len(detail.ResourceDetails) > 0 {
		summaries := make([]string, 0, len(detail.ResourceDetails))
		for _, resource := range detail.ResourceDetails {
			summaries = append(summaries, fallback(resource.Summary, resource.URI))
		}
		lines = append(lines, fmt.Sprintf("Resource summaries: %s", strings.Join(summaries, ", ")))
		lines = append(lines, "Press [r] to browse linked workflow resources")
	}
	return strings.Join(lines, "\n")
}

func (p *TasksPane) promptDetailText() string {
	if p.promptDetail == nil {
		return dimStyle.Render("No prompt selected")
	}
	prompt := p.promptDetail
	lines := []string{
		fmt.Sprintf("Prompt: %s", prompt.Meta.Title),
		fmt.Sprintf("ID: %s", prompt.PromptID),
		fmt.Sprintf("Runtime: %s", fallback(prompt.Meta.RuntimeFamily, "n/a")),
		fmt.Sprintf("Trust: %s", fallback(prompt.Meta.TrustClass, "n/a")),
		fmt.Sprintf("Source: %s", fallback(prompt.Meta.Source, "n/a")),
		fmt.Sprintf("Provider: %s", fallback(prompt.ProviderID, "n/a")),
		fmt.Sprintf("Description: %s", fallback(prompt.Description, "n/a")),
	}
	if len(prompt.Metadata) > 0 {
		lines = append(lines, fmt.Sprintf("Metadata: %s", strings.Join(prompt.Metadata, ", ")))
	}
	for i, message := range prompt.Messages {
		lines = append(lines, fmt.Sprintf("Message %d role: %s", i+1, fallback(message.Role, "n/a")))
		for _, block := range message.Content {
			lines = append(lines, renderStructuredContentPreview(block)...)
		}
	}
	return strings.Join(lines, "\n")
}

func (p *TasksPane) resourceDetailText() string {
	if p.resourceDetail == nil {
		return dimStyle.Render("No resource selected")
	}
	resource := p.resourceDetail
	lines := []string{
		fmt.Sprintf("Resource: %s", resource.Meta.Title),
		fmt.Sprintf("ID: %s", resource.ResourceID),
		fmt.Sprintf("Kind: %s", resource.Meta.Kind),
		fmt.Sprintf("Source: %s", fallback(resource.Meta.Source, "n/a")),
		fmt.Sprintf("Provider: %s", fallback(resource.ProviderID, "n/a")),
		fmt.Sprintf("Trust: %s", fallback(resource.Meta.TrustClass, "n/a")),
		fmt.Sprintf("Description: %s", fallback(resource.Description, "n/a")),
	}
	if resource.WorkflowResource {
		lines = append(lines, fmt.Sprintf("Workflow URI: %s", resource.WorkflowURI))
	}
	if len(resource.Metadata) > 0 {
		lines = append(lines, fmt.Sprintf("Metadata: %s", strings.Join(resource.Metadata, ", ")))
	}
	for _, block := range resource.Contents {
		lines = append(lines, renderStructuredContentPreview(block)...)
	}
	return strings.Join(lines, "\n")
}

func renderStructuredContentPreview(block StructuredContentBlock) []string {
	lines := []string{fmt.Sprintf("[%s] %s", block.Type, fallback(block.Summary, "content"))}
	if body := strings.TrimSpace(block.Body); body != "" {
		for _, line := range strings.Split(body, "\n") {
			lines = append(lines, "  "+line)
		}
	}
	if len(block.Provenance) > 0 {
		lines = append(lines, "  provenance: "+joinStringMap(block.Provenance))
	}
	return lines
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func joinOrNA(values []string) string {
	if len(values) == 0 {
		return "n/a"
	}
	return strings.Join(values, ", ")
}

func joinStringMap(values map[string]string) string {
	if len(values) == 0 {
		return "n/a"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	// stable detail output
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return strings.Join(parts, ", ")
}
