package tui

import (
	"sort"
	"strings"
	"unicode"
)

type inputDraft struct {
	raw           string
	prefix        string
	command       string
	args          []string
	current       string
	trailingSpace bool
	commandMode   bool
	filterMode    bool
	promptLabel   string
	paletteQuery  string
}

func parseInputDraft(raw string, activeTab TabID, explicitSearch bool) inputDraft {
	draft := inputDraft{raw: raw}
	trimmed := strings.TrimLeftFunc(raw, unicode.IsSpace)
	if trimmed == "" {
		draft.promptLabel = defaultPromptLabel(activeTab, explicitSearch)
		if explicitSearch {
			draft.filterMode = true
		}
		return draft
	}

	prefix := string(trimmed[0])
	if prefix == "/" {
		draft.prefix = prefix
		draft.commandMode = true
		draft.promptLabel = "command"
		body := strings.TrimLeft(trimmed[1:], " ")
		draft.trailingSpace = strings.HasSuffix(body, " ")
		tokens := strings.Fields(body)
		if len(tokens) > 0 {
			draft.command = tokens[0]
			draft.args = append([]string(nil), tokens[1:]...)
			if !draft.trailingSpace {
				draft.current = tokens[len(tokens)-1]
			}
		}
		draft.paletteQuery = body
	} else {
		draft.promptLabel = "request to Euclo"
		draft.current = raw
	}

	if draft.promptLabel == "" {
		draft.promptLabel = defaultPromptLabel(activeTab, explicitSearch)
	}
	return draft
}

func defaultPromptLabel(_ TabID, _ bool) string { return "request to Euclo" }

func parseCommandLine(raw string) (prefix string, name string, args []string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", nil, false
	}
	if !isCommandPrefix(rune(trimmed[0])) {
		return "", "", nil, false
	}
	prefix = string(trimmed[0])
	if prefix != "/" {
		return prefix, "", nil, false
	}
	body := strings.TrimSpace(trimmed[1:])
	if body == "" {
		return prefix, "", nil, false
	}
	fields, err := commandWords(body)
	if err != nil || len(fields) == 0 {
		return prefix, "", nil, false
	}
	name = fields[0]
	if len(fields) > 1 {
		args = append([]string(nil), fields[1:]...)
	}
	return prefix, name, args, true
}

func isCommandPrefix(r rune) bool { return r == '/' }

func commandPaletteItems(reg *CommandRegistry, query string, tabID TabID, subTabID SubTabID) []commandItem {
	var candidates []Command
	if reg != nil {
		candidates = reg.Match("", tabID, subTabID)
	}
	items := make([]commandItem, 0, len(candidates))
	for _, cmd := range candidates {
		score := 0
		if query != "" {
			if !strings.HasPrefix(cmd.Name, query) {
				continue
			}
			if cmd.Name == query {
				score = 2
			}
		}
		if len(cmd.SubTabFilter) > 0 {
			score++
		} else if len(cmd.TabFilter) > 0 {
			score++
		}
		items = append(items, commandItem{
			Name:        cmd.Name,
			Usage:       cmd.Usage,
			Description: cmd.Description,
			Score:       score,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Name < items[j].Name
		}
		return items[i].Score > items[j].Score
	})
	if len(items) > commandPaletteRows {
		items = items[:commandPaletteRows]
	}
	return items
}
