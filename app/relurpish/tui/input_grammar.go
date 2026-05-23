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
	switch prefix {
	case "/", ":":
		draft.prefix = prefix
		draft.commandMode = true
		draft.promptLabel = commandPromptLabel(prefix)
		body := strings.TrimLeft(trimmed[1:], " ")
		draft.trailingSpace = strings.HasSuffix(body, " ")
		tokens := strings.Fields(body)
		if len(tokens) > 0 {
			draft.command = tokens[0]
			if len(tokens) > 1 {
				draft.args = append([]string(nil), tokens[1:]...)
			}
			if !draft.trailingSpace {
				draft.current = tokens[len(tokens)-1]
			}
		}
		if len(tokens) == 1 && !draft.trailingSpace {
			draft.paletteQuery = tokens[0]
		}
	case ">":
		draft.prefix = prefix
		draft.promptLabel = "prompt"
		body := strings.TrimLeft(trimmed[1:], " ")
		draft.current = body
	case "?":
		draft.prefix = prefix
		draft.filterMode = true
		draft.promptLabel = "search"
		body := strings.TrimLeft(trimmed[1:], " ")
		draft.current = body
		draft.paletteQuery = body
	default:
		if activeTab == TabChat {
			draft.promptLabel = "prompt"
		} else {
			draft.promptLabel = "filter"
			draft.filterMode = true
		}
		draft.current = trimmed
		if explicitSearch {
			draft.filterMode = true
			if draft.promptLabel == "" {
				draft.promptLabel = "search"
			}
		}
	}

	if draft.promptLabel == "" {
		draft.promptLabel = defaultPromptLabel(activeTab, explicitSearch)
	}
	return draft
}

func defaultPromptLabel(activeTab TabID, explicitSearch bool) string {
	if explicitSearch {
		return "search"
	}
	if activeTab == TabChat {
		return "prompt"
	}
	return "filter"
}

func commandPromptLabel(prefix string) string {
	switch prefix {
	case ":":
		return "shell"
	case "/":
		return "slash"
	default:
		return "command"
	}
}

func parseCommandLine(raw string) (prefix string, name string, args []string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", nil, false
	}
	if !isCommandPrefix(rune(trimmed[0])) {
		return "", "", nil, false
	}
	prefix = string(trimmed[0])
	if prefix != "/" && prefix != ":" {
		return prefix, "", nil, false
	}
	body := strings.TrimSpace(trimmed[1:])
	if body == "" {
		return prefix, "", nil, false
	}
	fields := strings.Fields(body)
	if len(fields) == 0 {
		return prefix, "", nil, false
	}
	name = fields[0]
	if len(fields) > 1 {
		args = append([]string(nil), fields[1:]...)
	}
	return prefix, name, args, true
}

func isCommandPrefix(r rune) bool {
	switch r {
	case '/', ':', '>', '?':
		return true
	default:
		return false
	}
}

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
	} else {
		candidates = listCommandsSorted()
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
