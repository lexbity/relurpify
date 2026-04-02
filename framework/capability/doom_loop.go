package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/lexcodex/relurpify/framework/core"
)

type DoomLoopKind string

const (
	DoomLoopIdenticalCall DoomLoopKind = "identical_call"
	DoomLoopOscillating   DoomLoopKind = "oscillating"
	DoomLoopErrorFixation DoomLoopKind = "error_fixation"
	DoomLoopProgressStall DoomLoopKind = "progress_stall"
)

type DoomLoopError struct {
	Kind      DoomLoopKind
	Evidence  []string
	CallCount int
}

type RecoveryGuidanceRequest struct {
	Title       string
	Description string
	Context     map[string]any
}

type RecoveryGuidanceDecision struct {
	ChoiceID string
}

type RecoveryGuidanceBroker interface {
	RequestRecovery(ctx context.Context, req RecoveryGuidanceRequest) (*RecoveryGuidanceDecision, error)
}

func (e *DoomLoopError) Error() string {
	if e == nil {
		return "doom loop detected"
	}
	return fmt.Sprintf("doom loop detected (%s) after %d calls", e.Kind, e.CallCount)
}

type DoomLoopConfig struct {
	IdenticalCallThreshold int
	OscillationWindowSize  int
	ErrorFixationThreshold int
	ProgressStallThreshold int
}

func DefaultDoomLoopConfig() DoomLoopConfig {
	return DoomLoopConfig{
		IdenticalCallThreshold: 3,
		OscillationWindowSize:  6,
		ErrorFixationThreshold: 4,
		ProgressStallThreshold: 30,
	}
}

type DoomLoopDetector struct {
	cfg              DoomLoopConfig
	mu               sync.Mutex
	callHistory      []callRecord
	modifiedPaths    map[string]struct{}
	noProgressCount  int
	lastErrorStreak  int
	lastErrorMessage string
}

type callRecord struct {
	capabilityID string
	argsHash     string
	resultError  string
	modifiedPath string
}

func NewDoomLoopDetector(cfg DoomLoopConfig) *DoomLoopDetector {
	if cfg.IdenticalCallThreshold <= 0 {
		cfg.IdenticalCallThreshold = DefaultDoomLoopConfig().IdenticalCallThreshold
	}
	if cfg.OscillationWindowSize <= 0 {
		cfg.OscillationWindowSize = DefaultDoomLoopConfig().OscillationWindowSize
	}
	if cfg.ErrorFixationThreshold <= 0 {
		cfg.ErrorFixationThreshold = DefaultDoomLoopConfig().ErrorFixationThreshold
	}
	if cfg.ProgressStallThreshold <= 0 {
		cfg.ProgressStallThreshold = DefaultDoomLoopConfig().ProgressStallThreshold
	}
	return &DoomLoopDetector{
		cfg:           cfg,
		modifiedPaths: make(map[string]struct{}),
	}
}

func (d *DoomLoopDetector) Check(desc core.CapabilityDescriptor, args map[string]any) error {
	if d == nil {
		return nil
	}
	record := callRecord{
		capabilityID: desc.ID,
		argsHash:     hashArgs(args),
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	preview := append(append([]callRecord(nil), d.callHistory...), record)
	if err := d.detectIdentical(preview); err != nil {
		return err
	}
	if err := d.detectOscillation(preview); err != nil {
		return err
	}
	if d.lastErrorStreak >= d.cfg.ErrorFixationThreshold && d.lastErrorMessage != "" {
		return &DoomLoopError{
			Kind:      DoomLoopErrorFixation,
			Evidence:  collectTailErrors(d.callHistory, d.cfg.ErrorFixationThreshold),
			CallCount: d.lastErrorStreak,
		}
	}
	if d.noProgressCount >= d.cfg.ProgressStallThreshold {
		return &DoomLoopError{
			Kind:      DoomLoopProgressStall,
			Evidence:  collectTailCalls(d.callHistory, d.cfg.ProgressStallThreshold),
			CallCount: d.noProgressCount,
		}
	}
	d.callHistory = preview
	d.trimHistoryLocked()
	return nil
}

func (d *DoomLoopDetector) RecordResult(desc core.CapabilityDescriptor, result *core.ToolResult) error {
	if d == nil {
		return nil
	}
	record := callRecord{
		capabilityID: desc.ID,
		resultError:  normalizeResultError(result),
		modifiedPath: extractModifiedPath(result),
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if n := len(d.callHistory); n > 0 && d.callHistory[n-1].capabilityID == desc.ID &&
		d.callHistory[n-1].resultError == "" && d.callHistory[n-1].modifiedPath == "" {
		d.callHistory[n-1].resultError = record.resultError
		d.callHistory[n-1].modifiedPath = record.modifiedPath
	} else {
		d.callHistory = append(d.callHistory, record)
		d.trimHistoryLocked()
	}

	if record.resultError != "" {
		if record.resultError == d.lastErrorMessage {
			d.lastErrorStreak++
		} else {
			d.lastErrorMessage = record.resultError
			d.lastErrorStreak = 1
		}
	} else {
		d.lastErrorMessage = ""
		d.lastErrorStreak = 0
	}

	if record.modifiedPath != "" {
		if _, seen := d.modifiedPaths[record.modifiedPath]; seen {
			d.noProgressCount++
		} else {
			d.modifiedPaths[record.modifiedPath] = struct{}{}
			d.noProgressCount = 0
		}
	} else if successfulCoordinationProgress(desc, result) {
		d.noProgressCount = 0
	} else {
		d.noProgressCount++
	}
	return nil
}

func (d *DoomLoopDetector) Record(desc core.CapabilityDescriptor, result *core.ToolResult) error {
	return d.RecordResult(desc, result)
}

func (d *DoomLoopDetector) Reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callHistory = nil
	d.modifiedPaths = make(map[string]struct{})
	d.noProgressCount = 0
	d.lastErrorStreak = 0
	d.lastErrorMessage = ""
}

func (d *DoomLoopDetector) detectIdentical(history []callRecord) error {
	n := d.cfg.IdenticalCallThreshold
	if len(history) < n {
		return nil
	}
	tail := history[len(history)-n:]
	first := tail[0]
	if first.capabilityID == "" || first.argsHash == "" {
		return nil
	}
	for _, record := range tail[1:] {
		if record.capabilityID != first.capabilityID || record.argsHash != first.argsHash {
			return nil
		}
	}
	return &DoomLoopError{
		Kind:      DoomLoopIdenticalCall,
		Evidence:  collectCallKeys(tail),
		CallCount: n,
	}
}

func (d *DoomLoopDetector) detectOscillation(history []callRecord) error {
	n := d.cfg.OscillationWindowSize
	if n < 4 || len(history) < n || n%2 != 0 {
		return nil
	}
	tail := history[len(history)-n:]
	a := tail[0]
	b := tail[1]
	if a.capabilityID == "" || b.capabilityID == "" {
		return nil
	}
	if a.capabilityID == b.capabilityID && a.argsHash == b.argsHash {
		return nil
	}
	for i, record := range tail {
		expected := a
		if i%2 == 1 {
			expected = b
		}
		if record.capabilityID != expected.capabilityID || record.argsHash != expected.argsHash {
			return nil
		}
	}
	return &DoomLoopError{
		Kind:      DoomLoopOscillating,
		Evidence:  collectCallKeys(tail),
		CallCount: n,
	}
}

func (d *DoomLoopDetector) trimHistoryLocked() {
	limit := d.cfg.ProgressStallThreshold
	if d.cfg.OscillationWindowSize > limit {
		limit = d.cfg.OscillationWindowSize
	}
	if d.cfg.ErrorFixationThreshold > limit {
		limit = d.cfg.ErrorFixationThreshold
	}
	if d.cfg.IdenticalCallThreshold > limit {
		limit = d.cfg.IdenticalCallThreshold
	}
	if limit <= 0 || len(d.callHistory) <= limit {
		return
	}
	d.callHistory = append([]callRecord(nil), d.callHistory[len(d.callHistory)-limit:]...)
}

func hashArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("marshal-error:%T", args)
	}
	return string(data)
}

func normalizeResultError(result *core.ToolResult) string {
	if result == nil {
		return ""
	}
	errText := strings.TrimSpace(result.Error)
	if len(errText) > 120 {
		errText = errText[:120]
	}
	return errText
}

func extractModifiedPath(result *core.ToolResult) string {
	if result == nil {
		return ""
	}
	for _, key := range []string{"modified_path", "path", "file_path", "target"} {
		if path, ok := extractStringFromMap(result.Data, key); ok {
			return path
		}
		if path, ok := extractStringFromMap(result.Metadata, key); ok {
			return path
		}
	}
	if paths, ok := result.Data["modified_paths"].([]string); ok && len(paths) > 0 {
		return strings.TrimSpace(paths[0])
	}
	if raw, ok := result.Data["modified_paths"].([]any); ok {
		for _, item := range raw {
			if path, ok := item.(string); ok && strings.TrimSpace(path) != "" {
				return strings.TrimSpace(path)
			}
		}
	}
	return ""
}

func successfulCoordinationProgress(desc core.CapabilityDescriptor, result *core.ToolResult) bool {
	if result == nil || strings.TrimSpace(result.Error) != "" {
		return false
	}
	if desc.Coordination == nil || !desc.Coordination.Target {
		return false
	}
	if len(result.Data) > 0 {
		return true
	}
	if len(result.Metadata) > 0 {
		return true
	}
	return false
}

func extractStringFromMap(m map[string]interface{}, key string) (string, bool) {
	if len(m) == 0 {
		return "", false
	}
	raw, ok := m[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return strings.TrimSpace(value), true
}

func collectCallKeys(records []callRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, fmt.Sprintf("%s:%s", record.capabilityID, record.argsHash))
	}
	return out
}

func collectTailCalls(records []callRecord, n int) []string {
	if len(records) < n {
		n = len(records)
	}
	return collectCallKeys(records[len(records)-n:])
}

func collectTailErrors(records []callRecord, n int) []string {
	if len(records) < n {
		n = len(records)
	}
	out := make([]string, 0, n)
	for _, record := range records[len(records)-n:] {
		if record.resultError == "" {
			continue
		}
		out = append(out, record.resultError)
	}
	return out
}

func describeLoop(err DoomLoopError) string {
	if len(err.Evidence) == 0 {
		return err.Error()
	}
	return fmt.Sprintf("%s. Evidence: %s", err.Error(), strings.Join(err.Evidence, " | "))
}
