package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"codeburg.org/lexbit/relurpify/telemetry"
)

// newFileTelemetry wires the runner's telemetry to the state-dir JSONL file —
// the same idiom as the app's telemetry files.
func newFileTelemetry(path string) (telemetry.Telemetry, error) {
	fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open telemetry log: %w", err)
	}
	return &fileTelemetry{fh: fh}, nil
}

type fileTelemetry struct {
	mu sync.Mutex
	fh *os.File
}

func (t *fileTelemetry) Emit(ev telemetry.Event) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = t.fh.Write(append(data, '\n'))
}
