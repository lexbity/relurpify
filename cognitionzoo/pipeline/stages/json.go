package stages

import "strings"

// extractJSON returns the first JSON object/array payload found in raw text.
func extractJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	startObj := strings.Index(raw, "{")
	startArr := strings.Index(raw, "[")
	start := -1
	switch {
	case startObj >= 0 && startArr >= 0:
		if startObj < startArr {
			start = startObj
		} else {
			start = startArr
		}
	case startObj >= 0:
		start = startObj
	case startArr >= 0:
		start = startArr
	default:
		return raw
	}
	endObj := strings.LastIndex(raw, "}")
	endArr := strings.LastIndex(raw, "]")
	end := endObj
	if endArr > end {
		end = endArr
	}
	if start >= 0 && end >= start {
		return raw[start : end+1]
	}
	return raw
}
