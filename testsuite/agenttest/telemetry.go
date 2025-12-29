package agenttest

import (
	"bufio"
	"encoding/json"
	"github.com/lexcodex/relurpify/framework/core"
	"os"
)

func ReadTelemetryJSONL(path string) ([]core.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []core.Event
	sc := bufio.NewScanner(f)
	// Allow large lines (debug prompt logging).
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var ev core.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func CountToolCalls(events []core.Event) (total int, byTool map[string]int) {
	byTool = make(map[string]int)
	for _, ev := range events {
		if ev.Type != core.EventToolCall {
			continue
		}
		total++
		tool, _ := ev.Metadata["tool"].(string)
		if tool != "" {
			byTool[tool]++
		}
	}
	return total, byTool
}
