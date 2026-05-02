package prompt

import (
	"fmt"
	"strconv"
	"strings"
)

// rawBlock holds a block before metadata conversion.
type rawBlock struct {
	name    string
	tildes  map[string]string
	content string
	fileIdx int // position in the block slice (for stable sort tie-breaking)
}

// parseBlocks splits the block section into rawBlock entries.
// Blocks begin at "# Heading" lines. ## and deeper headings are reserved.
func parseBlocks(lines []string) []rawBlock {
	var blocks []rawBlock
	var cur *rawBlock
	inTildes := true

	for i, line := range lines {
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			rb := rawBlock{
				name:    strings.TrimSpace(strings.TrimPrefix(line, "# ")),
				tildes:  make(map[string]string),
				fileIdx: i,
			}
			blocks = append(blocks, rb)
			cur = &blocks[len(blocks)-1]
			inTildes = true
			continue
		}
		if cur == nil {
			continue
		}
		if inTildes && strings.HasPrefix(line, "~ ") {
			kv := strings.TrimSpace(strings.TrimPrefix(line, "~ "))
			colon := strings.Index(kv, ":")
			if colon >= 0 {
				k := strings.TrimSpace(kv[:colon])
				v := strings.TrimSpace(kv[colon+1:])
				cur.tildes[k] = v
			}
			continue
		}
		if inTildes && strings.TrimSpace(line) != "" {
			inTildes = false
		}
		if !inTildes {
			cur.content += line + "\n"
		}
	}

	// Re-assign fileIdx to be position in the resulting blocks slice.
	for i := range blocks {
		blocks[i].fileIdx = i
	}

	return blocks
}

// buildBlocks converts rawBlocks into PromptBlocks.
// promptID is used only for warning messages.
func buildBlocks(raw []rawBlock, promptID string) ([]PromptBlock, []string) {
	var blocks []PromptBlock
	var warnings []string

	for _, rb := range raw {
		b := PromptBlock{
			ID:      blockIDFromName(rb.name),
			Name:    rb.name,
			Content: strings.TrimRight(rb.content, "\n"),
			Order:   OrderMiddle,
			From:    SourceStatic,
		}

		for k, v := range rb.tildes {
			switch k {
			case "kind":
				b.Kind = v
			case "order":
				order, err := parseOrderValue(v)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("[%s/%s] %v", promptID, b.ID, err))
				} else {
					b.Order = order
				}
			case "when":
				expr, err := compileExpression(v)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("[%s/%s] invalid when expression: %v", promptID, b.ID, err))
				} else {
					b.When = expr
				}
			case "from":
				switch v {
				case "static":
					b.From = SourceStatic
				case "provider":
					b.From = SourceProvider
				default:
					warnings = append(warnings, fmt.Sprintf("[%s/%s] unknown from value: %s", promptID, b.ID, v))
				}
			case "provider":
				b.Provider = v
			case "locked":
				b.Locked = v == "true"
			default:
				warnings = append(warnings, fmt.Sprintf("[%s/%s] unknown block metadata key: %s", promptID, b.ID, k))
			}
		}

		blocks = append(blocks, b)
	}

	return blocks, warnings
}

// blockIDFromName derives a block id from its heading name.
func blockIDFromName(name string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// parseOrderValue converts a named or numeric order string to an integer.
func parseOrderValue(s string) (int, error) {
	if v, ok := ParseNamedOrder(s); ok {
		return v, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return OrderMiddle, fmt.Errorf("invalid order value %q (use early/middle/late/last or 1-999)", s)
	}
	if n < 1 || n > 999 {
		return OrderMiddle, fmt.Errorf("order %d out of range [1, 999]", n)
	}
	return n, nil
}
