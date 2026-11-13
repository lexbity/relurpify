package tui

import (
	"sort"
	"strings"
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
	trimmed := strings.TrimLeft(raw, " ")
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
		if len(tokens) == 1 && !draft.trailingSpace {
			draft.paletteQuery = tokens[0]
		}
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

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

func replaceTokenAtPrefix(raw, prefix, replacement string) string {
	if prefix == "" {
		return replacement
	}
	body := strings.TrimLeft(raw[len(prefix):], " ")
	if body == "" {
		return prefix + replacement + " "
	}
	idx := strings.IndexFunc(body, func(r rune) bool {
		return r == ' ' || r == '\t'
	})
	if idx < 0 {
		return prefix + replacement + " "
	}
	return prefix + replacement + body[idx:]
}

func commandPaletteItems(reg *CommandRegistry, query string, tabID TabID, subTabID SubTabID) []commandItem {
	var candidates []Command
	if reg != nil {
		candidates = reg.Match("", tabID, subTabID)
	}
	items := make([]commandItem, 0, len(candidates))
	for _, cmd := range candidates {
		score := 0
		if query != "" {
			if ok, s := fuzzyMatchScore(query, cmd.Name); ok {
				score = s
			} else if ok2, s2 := fuzzyMatchScore(query, cmd.Usage); ok2 {
				score = s2 - 2
			} else {
				continue
			}
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
