package otelleg

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// LogRecord is one OTel log record decoded from an OTLP-JSON segment file. It
// is the read side of the local file leg: the TUI export, the dev-agent-cli
// selection report, and the test probe all consume this shape.
type LogRecord struct {
	// Time is the record's event timestamp.
	Time time.Time
	// Name is the eventName — the relurpify event-type string.
	Name string
	// Severity is the severity text (Info/Warn/Error).
	Severity string
	// Body is the record's message, if any.
	Body string
	// TraceID and SpanID are hex-encoded OTel identifiers, empty when the
	// record is trace-less.
	TraceID string
	SpanID  string
	// Attributes holds the record's attributes with native value types
	// (string, bool, int64, float64).
	Attributes map[string]any
}

// Attr returns the record attribute under key as a string, or "" when absent
// or of another type.
func (r LogRecord) Attr(key string) string {
	if v, ok := r.Attributes[key].(string); ok {
		return v
	}
	return ""
}

// ReadSegmentLogRecords decodes the log records of one segment file. Lines
// that are not OTLP-JSON (truncated writes, span/metric batches of other
// wirings, foreign content) are skipped — the file leg is append-only and a
// torn tail must never abort a reader.
func ReadSegmentLogRecords(path string) ([]LogRecord, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var records []LogRecord
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		records = appendLogBatch(records, line)
	}
	return records, scanner.Err()
}

// ReadSegmentDirLogRecords decodes the log records of every *.jsonl segment in
// dir, ordered by file name. Unreadable or foreign files are skipped with the
// same leniency as single-line parses.
func ReadSegmentDirLogRecords(dir string) ([]LogRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var records []LogRecord
	for _, name := range names {
		fileRecords, err := ReadSegmentLogRecords(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		records = append(records, fileRecords...)
	}
	return records, nil
}

// appendLogBatch decodes one OTLP-JSON line's log batch into records. A line
// carrying no log batch (spans, metrics, garbage) contributes nothing.
func appendLogBatch(records []LogRecord, line []byte) []LogRecord {
	var batch struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []struct {
					TimeUnixNano  string `json:"timeUnixNano"`
					SeverityText  string `json:"severityText"`
					EventName     string `json:"eventName"`
					TraceIDBase64 string `json:"traceId"`
					SpanIDBase64  string `json:"spanId"`
					Body          *struct {
						StringValue string `json:"stringValue"`
					} `json:"body"`
					Attributes []struct {
						Key   string         `json:"key"`
						Value map[string]any `json:"value"`
					} `json:"attributes"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}
	if err := json.Unmarshal(line, &batch); err != nil {
		return records
	}
	for _, resource := range batch.ResourceLogs {
		for _, scope := range resource.ScopeLogs {
			for _, raw := range scope.LogRecords {
				records = append(records, decodeLogRecord(raw))
			}
		}
	}
	return records
}

func decodeLogRecord(raw struct {
	TimeUnixNano  string `json:"timeUnixNano"`
	SeverityText  string `json:"severityText"`
	EventName     string `json:"eventName"`
	TraceIDBase64 string `json:"traceId"`
	SpanIDBase64  string `json:"spanId"`
	Body          *struct {
		StringValue string `json:"stringValue"`
	} `json:"body"`
	Attributes []struct {
		Key   string         `json:"key"`
		Value map[string]any `json:"value"`
	} `json:"attributes"`
}) LogRecord {
	rec := LogRecord{
		Name:       raw.EventName,
		Severity:   raw.SeverityText,
		TraceID:    base64IDToHex(raw.TraceIDBase64),
		SpanID:     base64IDToHex(raw.SpanIDBase64),
		Attributes: make(map[string]any, len(raw.Attributes)),
	}
	if raw.Body != nil {
		rec.Body = raw.Body.StringValue
	}
	if ns, err := strconv.ParseInt(raw.TimeUnixNano, 10, 64); err == nil && ns > 0 {
		rec.Time = time.Unix(0, ns).UTC()
	}
	for _, attr := range raw.Attributes {
		if value, ok := decodeAttrValue(attr.Value); ok {
			rec.Attributes[attr.Key] = value
		}
	}
	return rec
}

// decodeAttrValue projects one OTLP attribute value set to its native Go type.
func decodeAttrValue(value map[string]any) (any, bool) {
	if value == nil {
		return nil, false
	}
	if s, ok := value["stringValue"].(string); ok {
		return s, true
	}
	if b, ok := value["boolValue"].(bool); ok {
		return b, true
	}
	if d, ok := value["doubleValue"].(float64); ok {
		return d, true
	}
	// protojson encodes 64-bit integers as strings.
	if s, ok := value["intValue"].(string); ok {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
	}
	return nil, false
}

// base64IDToHex decodes a protojson bytes field (base64) to the hex form the
// rest of the system uses for trace/span ids. Invalid input decodes to "".
func base64IDToHex(encoded string) string {
	if encoded == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}
