package tui

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionSection selects which view is active in the session pane.
type SessionSection int

const (
	SectionFiles SessionSection = iota
	SectionChanges
)

// SessionPane displays workspace files and session changes.
type SessionPane struct {
	section SessionSection

	// Files section
	allFiles []FileEntry
	filtered []FileEntry
	fileSel  int
	filter   string
	loading  bool
	loadErr  error

	// Changes section
	changes   []FileChange
	changeSel int

	context *AgentContext
	session *Session
	runtime RuntimeAdapter
	width   int
	height  int
}

// NewSessionPane creates a SessionPane.
func NewSessionPane(ctx *AgentContext, sess *Session, rt RuntimeAdapter) *SessionPane {
	return &SessionPane{
		context: ctx,
		session: sess,
		runtime: rt,
	}
}

// Init starts the file index build.
func (p *SessionPane) Init() tea.Cmd {
	root := "."
	if p.session != nil && p.session.Workspace != "" {
		root = p.session.Workspace
	}
	p.loading = true
	return fileIndexCmd(root)
}

// SetSize resizes the pane.
func (p *SessionPane) SetSize(w, h int) { p.width = w; p.height = h }

// SyncChanges updates the changes list, called by root after each run.
func (p *SessionPane) SyncChanges(changes []FileChange) {
	p.changes = append([]FileChange(nil), changes...)
	if p.changeSel >= len(p.changes) {
		p.changeSel = 0
	}
}

// SyncContext re-syncs the context reference (no-op if pointer unchanged).
func (p *SessionPane) SyncContext(ctx *AgentContext) {
	if ctx != nil {
		p.context = ctx
	}
}

// Update handles navigation and async messages.
func (p *SessionPane) Update(msg tea.Msg) (*SessionPane, tea.Cmd) {
	switch msg := msg.(type) {
	case fileIndexMsg:
		p.loading = false
		if msg.err != nil {
			p.loadErr = msg.err
			return p, nil
		}
		p.allFiles = msg.files
		p.applyFilter()

	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if p.section == SectionFiles {
				p.section = SectionChanges
			} else {
				p.section = SectionFiles
			}

		case "up":
			if p.section == SectionChanges {
				if p.changeSel > 0 {
					p.changeSel--
				}
			} else {
				if p.fileSel > 0 {
					p.fileSel--
				}
			}

		case "down":
			if p.section == SectionChanges {
				if p.changeSel < len(p.changes)-1 {
					p.changeSel++
				}
			} else {
				if p.fileSel < len(p.filtered)-1 {
					p.fileSel++
				}
			}

		case "enter":
			if p.section == SectionFiles && p.fileSel < len(p.filtered) {
				e := p.filtered[p.fileSel]
				if p.context != nil {
					if err := p.context.AddFile(e.Path); err == nil {
						return p, func() tea.Msg {
							return chatSystemMsg{text: fmt.Sprintf("Added: %s", e.DisplayPath)}
						}
					}
				}
			}

		case "y", "Y":
			if p.section == SectionChanges && p.changeSel < len(p.changes) {
				p.changes[p.changeSel].Status = StatusApproved
			}

		case "n", "N":
			if p.section == SectionChanges && p.changeSel < len(p.changes) {
				p.changes[p.changeSel].Status = StatusRejected
			}

		case "e":
			path := ""
			if p.section == SectionFiles && p.fileSel < len(p.filtered) {
				path = p.filtered[p.fileSel].Path
			} else if p.section == SectionChanges && p.changeSel < len(p.changes) {
				path = p.changes[p.changeSel].Path
			}
			if path != "" {
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				return p, tea.ExecProcess(exec.Command(editor, path), func(err error) tea.Msg {
					if err != nil {
						return chatSystemMsg{text: fmt.Sprintf("Editor error: %v", err)}
					}
					return nil
				})
			}
		}
	}
	return p, nil
}

// HandleFilterInput updates the file filter from the input bar.
func (p *SessionPane) HandleFilterInput(query string) {
	p.filter = strings.TrimSpace(query)
	p.fileSel = 0
	p.applyFilter()
}

func (p *SessionPane) applyFilter() {
	const maxRows = 20
	p.filtered = filterFileEntries(p.allFiles, p.filter, maxRows)
	sort.Slice(p.filtered, func(i, j int) bool {
		if p.filtered[i].Score != p.filtered[j].Score {
			return p.filtered[i].Score > p.filtered[j].Score
		}
		return p.filtered[i].DisplayPath < p.filtered[j].DisplayPath
	})
	if p.fileSel >= len(p.filtered) {
		p.fileSel = 0
	}
}

// View renders the active section.
func (p *SessionPane) View() string {
	if p.section == SectionChanges {
		return p.viewChanges()
	}
	return p.viewFiles()
}

func (p *SessionPane) viewFiles() string {
	if p.loading {
		return dimStyle.Render("Indexing workspace files...")
	}
	if p.loadErr != nil {
		return notifErrorStyle.Render(fmt.Sprintf("File index error: %v", p.loadErr))
	}
	var b strings.Builder
	header := "Workspace Files"
	if p.filter != "" {
		header += "  " + dimStyle.Render(fmt.Sprintf("filter: %q", p.filter))
	}
	b.WriteString(sectionHeaderStyle.Render(header))
	b.WriteString("\n\n")
	if len(p.filtered) == 0 {
		b.WriteString(dimStyle.Render("No matching files"))
	} else {
		for i, e := range p.filtered {
			line := renderFileEntryLine(e)
			if i == p.fileSel {
				line = panelItemActiveStyle.Render(line)
			} else {
				line = panelItemStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}
	if p.context != nil && len(p.context.Files) > 0 {
		b.WriteString("\n" + sectionHeaderStyle.Render("Context") + "\n")
		for _, f := range p.context.Files {
			b.WriteString(dimStyle.Render("  • ") + filePathStyle.Render(f) + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter=add to context  e=open in editor  tab=view changes"))
	return b.String()
}

func (p *SessionPane) viewChanges() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("Session Changes"))
	b.WriteString("\n\n")
	if len(p.changes) == 0 {
		b.WriteString(dimStyle.Render("No changes in this session yet"))
		b.WriteString("\n\n" + dimStyle.Render("tab=view files"))
		return b.String()
	}
	for i, c := range p.changes {
		statusIcon, statusRender := changeStatusDisplay(c.Status)
		changeType := string(c.Type)
		if changeType == "" {
			changeType = "modify"
		}
		line := statusRender(statusIcon) + "  " +
			filePathStyle.Render(c.Path) +
			"  " + dimStyle.Render("("+changeType+")")
		if c.LinesAdded > 0 || c.LinesRemoved > 0 {
			line += dimStyle.Render(fmt.Sprintf("  +%d/-%d", c.LinesAdded, c.LinesRemoved))
		}
		if i == p.changeSel {
			line = panelItemActiveStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("y=approve  n=reject  e=open in editor  tab=view files"))
	return b.String()
}

func changeStatusDisplay(s ChangeStatus) (string, func(string) string) {
	wrap := func(st lipgloss.Style) func(string) string {
		return func(v string) string { return st.Render(v) }
	}
	switch s {
	case StatusApproved:
		return "✓", wrap(taskDoneStyle)
	case StatusRejected:
		return "✗", wrap(lipgloss.NewStyle().Foreground(lipgloss.Color("1")))
	default:
		return "?", wrap(dimStyle)
	}
}
