package telemetry

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// DefaultTelemetryRetention is the NFR-10 default retention window for JSONL
// telemetry files: 7 days, configurable per workspace.
const DefaultTelemetryRetention = 7 * 24 * time.Hour

// PruneJSONL drops events older than the retention window from a newline-
// delimited JSON telemetry file, rewriting the file atomically. It is the
// NFR-10 retention mechanism: call it at open time so a session's telemetry
// file never grows past one retention window of traffic.
//
// Failure semantics are deliberately lenient — retention must never break
// telemetry:
//   - a missing file is a no-op;
//   - lines whose timestamp cannot be parsed are kept (dropping a log on a
//     parse failure loses diagnostics, which is worse than retaining a line);
//   - a rewrite failure keeps the original file and returns the error so the
//     caller can log a warning and continue.
func PruneJSONL(path string, retention time.Duration, now time.Time) error {
	if retention <= 0 || now.IsZero() || path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	cutoff := now.Add(-retention)
	var survivors [][]byte
	var dropped int
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		ts := decodeTimestamp(line)
		// Blank, malformed, and timestamp-less lines are kept: dropping a log on
		// a parse failure loses diagnostics, which is worse than retaining a
		// line within an approximate retention window.
		if ts.IsZero() || ts.After(cutoff) {
			survivors = append(survivors, append([]byte(nil), line...))
			continue
		}
		dropped++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if dropped == 0 {
		return nil
	}

	tmp := path + ".prune.tmp"
	tmpFile, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writeErr := writeJSONLines(tmpFile, survivors)
	closeErr := tmpFile.Close()
	if writeErr != nil {
		_ = os.Remove(tmp)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, path)
}

// decodeTimestamp extracts the event timestamp from one JSONL line. Zero is
// returned (→ kept) when the line is blank, malformed, or timestamp-less.
func decodeTimestamp(line []byte) (t time.Time) {
	if len(line) == 0 {
		return time.Time{}
	}
	var probe struct {
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return time.Time{}
	}
	return probe.Timestamp
}

func writeJSONLines(file *os.File, lines [][]byte) error {
	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := writer.Write(line); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	return writer.Flush()
}
