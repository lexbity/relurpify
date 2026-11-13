package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// commandWords accepts bare arguments and JSON-quoted strings. It performs no
// shell expansion, substitutions, or evaluation. Quoted values decode once.
func commandWords(value string) ([]string, error) {
	var words []string
	for value = strings.TrimLeftFunc(value, unicode.IsSpace); value != ""; value = strings.TrimLeftFunc(value, unicode.IsSpace) {
		if value[0] != '"' {
			end := strings.IndexFunc(value, unicode.IsSpace)
			if end < 0 {
				end = len(value)
			}
			word := value[:end]
			if strings.ContainsRune(word, '"') {
				return nil, fmt.Errorf("quote must start an argument")
			}
			words = append(words, word)
			value = value[end:]
			continue
		}
		end := 1
		for end < len(value) {
			if value[end] == '\\' {
				end += 2
				continue
			}
			if value[end] == '"' {
				break
			}
			end++
		}
		if end >= len(value) {
			return nil, fmt.Errorf("unterminated quoted argument")
		}
		var word string
		if err := json.Unmarshal([]byte(value[:end+1]), &word); err != nil {
			return nil, fmt.Errorf("invalid JSON-quoted argument: %w", err)
		}
		value = value[end+1:]
		if value != "" && !unicode.IsSpace([]rune(value)[0]) {
			return nil, fmt.Errorf("arguments must be separated by whitespace")
		}
		words = append(words, word)
	}
	return words, nil
}
