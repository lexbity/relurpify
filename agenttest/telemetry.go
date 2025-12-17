package agenttest

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/lexcodex/relurpify/framework"
)

func ReadTelemetryJSONL(path string) ([]framework.Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var events []framework.Event
	sc := bufio.NewScanner(f)
	// Allow large lines (debug prompt logging).
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var ev framework.Event
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

func CountToolCalls(events []framework.Event) (total int, byTool map[string]int) {
	byTool = make(map[string]int)
	for _, ev := range events {
		if ev.Type != framework.EventToolCall {
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
