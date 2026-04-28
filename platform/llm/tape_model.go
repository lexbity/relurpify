package llm

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/platform/contracts"
)

type TapeMode string

const (
	TapeOff    TapeMode = "off"
	TapeRecord TapeMode = "record"
	TapeReplay TapeMode = "replay"
)

type tapeEntry struct {
	Timestamp   time.Time    `json:"timestamp"`
	Kind        string       `json:"kind"`
	Fingerprint string       `json:"fingerprint"`
	Request     tapeRequest  `json:"request"`
	Response    *LLMResponse `json:"response,omitempty"`
	Error       string       `json:"error,omitempty"`
}

type tapeRequest struct {
	Prompt    string      `json:"prompt,omitempty"`
	Messages  []Message   `json:"messages,omitempty"`
	ToolNames []string    `json:"tool_names,omitempty"`
	Options   *LLMOptions `json:"options,omitempty"`
	Header    *TapeHeader `json:"header,omitempty"`
}

type TapeHeader struct {
	ProviderID       string `json:"provider_id,omitempty"`
	Kind             string `json:"kind"`
	ModelName        string `json:"model_name"`
	ModelDigest      string `json:"model_digest,omitempty"`
	FrameworkVersion string `json:"framework_version,omitempty"`
	RecordedAt       string `json:"recorded_at"`
	SuiteName        string `json:"suite_name,omitempty"`
	CaseName         string `json:"case_name,omitempty"`
}

type TapeModel struct {
	inner LanguageModel
	mode  TapeMode
	path  string

	mu                   sync.Mutex
	file                 *os.File
	enc                  *json.Encoder
	loaded               []tapeEntry
	next                 int
	header               *TapeHeader
	firstReplayValidated bool
}

type TapeInspection struct {
	Path            string
	Header          *TapeHeader
	Legacy          bool
	FirstEntryKind  string
	FirstRecordedAt time.Time
	EntryCount      int
}

const staleTapeWarningThreshold = 30 * 24 * time.Hour

func NewTapeModel(inner LanguageModel, path string, mode string) (*TapeModel, error) {
	if inner == nil {
		return nil, errors.New("inner model required")
	}
	if path == "" {
		return nil, errors.New("tape path required")
	}
	m := TapeMode(mode)
	if m == "" {
		m = TapeOff
	}
	tm := &TapeModel{inner: inner, mode: m, path: path}
	switch tm.mode {
	case TapeOff:
		return tm, nil
	case TapeRecord:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, err
		}
		tm.file = f
		tm.enc = json.NewEncoder(f)
		return tm, nil
	case TapeReplay:
		entries, err := readTape(path)
		if err != nil {
			return nil, err
		}
		tm.loaded = entries
		return tm, nil
	default:
		return nil, fmt.Errorf("unknown tape mode %q", mode)
	}
}

func (t *TapeModel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file != nil {
		err := t.file.Close()
		t.file = nil
		t.enc = nil
		return err
	}
	return nil
}

func (t *TapeModel) ConfigureHeader(header TapeHeader) error {
	header.Kind = "_header"
	if strings.TrimSpace(header.RecordedAt) == "" {
		header.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(header.FrameworkVersion) == "" {
		header.FrameworkVersion = currentFrameworkVersion()
	}
	t.header = &header
	if t.mode == TapeRecord {
		return t.writeHeader()
	}
	if t.mode == TapeReplay {
		return t.validateHeader(header.ProviderID, header.ModelName, header.ModelDigest)
	}
	return nil
}

func (t *TapeModel) Generate(ctx context.Context, prompt string, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	req := tapeRequest{Prompt: prompt, Options: options}
	return t.roundTrip(ctx, "generate", req, func() (*contracts.LLMResponse, error) {
		return t.inner.Generate(ctx, prompt, options)
	})
}

func (t *TapeModel) GenerateStream(ctx context.Context, prompt string, options *contracts.LLMOptions) (<-chan string, error) {
	req := tapeRequest{Prompt: prompt, Options: options}
	fp := fingerprint("generate_stream", req)
	if t.mode == TapeReplay {
		if err := t.validateFirstReplayRequest("generate_stream", req); err != nil {
			return nil, err
		}
		entry, err := t.nextEntry("generate_stream", fp)
		if err != nil {
			return nil, err
		}
		out := make(chan string, 1)
		go func() {
			defer close(out)
			if entry.Response != nil && entry.Response.Text != "" {
				out <- entry.Response.Text
			}
		}()
		return out, nil
	}
	if t.mode == TapeOff {
		return t.inner.GenerateStream(ctx, prompt, options)
	}

	stream, err := t.inner.GenerateStream(ctx, prompt, options)
	if err != nil {
		t.append(tapeEntry{
			Timestamp:   time.Now().UTC(),
			Kind:        "generate_stream",
			Fingerprint: fp,
			Request:     req,
			Error:       err.Error(),
		})
		return nil, err
	}
	out := make(chan string)
	go func() {
		defer close(out)
		var buf string
		for token := range stream {
			buf += token
			out <- token
		}
		t.append(tapeEntry{
			Timestamp:   time.Now().UTC(),
			Kind:        "generate_stream",
			Fingerprint: fp,
			Request:     req,
			Response:    &contracts.LLMResponse{Text: buf, FinishReason: "stream"},
		})
	}()
	return out, nil
}

func (t *TapeModel) Chat(ctx context.Context, messages []contracts.Message, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	req := tapeRequest{Messages: messages, Options: options}
	return t.roundTrip(ctx, "chat", req, func() (*contracts.LLMResponse, error) {
		return t.inner.Chat(ctx, messages, options)
	})
}

func (t *TapeModel) ChatWithTools(ctx context.Context, messages []contracts.Message, tools []contracts.LLMToolSpec, options *contracts.LLMOptions) (*contracts.LLMResponse, error) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	req := tapeRequest{Messages: messages, ToolNames: names, Options: options}
	return t.roundTrip(ctx, "chat_with_tools", req, func() (*contracts.LLMResponse, error) {
		return t.inner.ChatWithTools(ctx, messages, tools, options)
	})
}

func (t *TapeModel) roundTrip(ctx context.Context, kind string, req tapeRequest, call func() (*contracts.LLMResponse, error)) (*contracts.LLMResponse, error) {
	fp := fingerprint(kind, req)
	if t.mode == TapeReplay {
		if err := t.validateFirstReplayRequest(kind, req); err != nil {
			return nil, err
		}
		entry, err := t.nextEntry(kind, fp)
		if err != nil {
			return nil, err
		}
		if entry.Error != "" {
			return nil, errors.New(entry.Error)
		}
		if entry.Response == nil {
			return &contracts.LLMResponse{}, nil
		}
		return entry.Response, nil
	}
	if t.mode == TapeOff {
		return call()
	}
	resp, err := call()
	entry := tapeEntry{
		Timestamp:   time.Now().UTC(),
		Kind:        kind,
		Fingerprint: fp,
		Request:     req,
		Response:    resp,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	t.append(entry)
	return resp, err
}

func (t *TapeModel) append(entry tapeEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.enc == nil {
		return
	}
	_ = t.enc.Encode(entry)
}

func (t *TapeModel) writeHeader() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.enc == nil || t.header == nil {
		return nil
	}
	entry := tapeEntry{
		Timestamp: time.Now().UTC(),
		Kind:      "_header",
		Request: tapeRequest{
			Header: t.header,
		},
	}
	if err := t.enc.Encode(entry); err != nil {
		return err
	}
	t.header = nil
	return nil
}

func (t *TapeModel) nextEntry(kind, fp string) (tapeEntry, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := t.next; i < len(t.loaded); i++ {
		entry := t.loaded[i]
		if entry.Kind == "_header" {
			t.next = i + 1
			continue
		}
		t.next = i + 1
		if entry.Kind == kind && entry.Fingerprint == fp {
			return entry, nil
		}
	}
	return tapeEntry{}, fmt.Errorf("tape exhausted or mismatch for %s fingerprint=%s", kind, fp)
}

func (t *TapeModel) validateHeader(currentProvider, currentModel, currentDigest string) error {
	inspection, err := inspectLoadedTape(t.path, t.loaded)
	if err != nil {
		return err
	}
	if inspection.Header == nil {
		log.Printf("WARNING: tape has no header - cannot verify model compatibility. Consider re-recording.")
		return nil
	}
	h := inspection.Header
	t.next = firstReplayIndex(t.loaded)
	if strings.TrimSpace(h.ModelName) != "" && strings.TrimSpace(h.ModelName) != strings.TrimSpace(currentModel) {
		return fmt.Errorf("tape recorded with model %q but running with %q - re-record tape", h.ModelName, currentModel)
	}
	if strings.TrimSpace(h.ModelDigest) != "" && strings.TrimSpace(currentDigest) != "" && strings.TrimSpace(h.ModelDigest) != strings.TrimSpace(currentDigest) {
		return fmt.Errorf("tape recorded with model digest %s but current digest is %s - model binary changed, re-record tape",
			shortDigest(h.ModelDigest), shortDigest(currentDigest))
	}
	if strings.TrimSpace(h.ProviderID) != "" && strings.TrimSpace(currentProvider) != "" && strings.TrimSpace(h.ProviderID) != strings.TrimSpace(currentProvider) {
		log.Printf("WARNING: tape recorded with provider %q but running with %q. Replay may intentionally switch providers.", h.ProviderID, currentProvider)
	}
	if age := time.Since(inspection.FirstRecordedAt); !inspection.FirstRecordedAt.IsZero() && age > staleTapeWarningThreshold {
		log.Printf("WARNING: golden tape for %q/%q was recorded %d days ago. Consider refreshing with: agenttest refresh --suite %s --case %s",
			firstTapeValue(h.SuiteName, "unknown-suite"),
			firstTapeValue(h.CaseName, "unknown-case"),
			int(age.Round(24*time.Hour)/(24*time.Hour)),
			firstTapeValue(h.SuiteName, "<suite-path>"),
			firstTapeValue(h.CaseName, "<case-name>"),
		)
	}
	return nil
}

func (t *TapeModel) validateFirstReplayRequest(kind string, req tapeRequest) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.firstReplayValidated {
		return nil
	}
	entry, ok := firstNonHeaderEntryFrom(t.loaded, t.next)
	if !ok {
		return errors.New("tape is empty")
	}
	t.firstReplayValidated = true
	if strings.TrimSpace(entry.Fingerprint) == "" {
		return nil
	}
	currentFP := fingerprint(kind, req)
	if currentFP == entry.Fingerprint {
		return nil
	}
	return fmt.Errorf("%s", t.firstReplayMismatchError(entry, kind))
}

func (t *TapeModel) firstReplayMismatchError(entry tapeEntry, currentKind string) string {
	suiteName := "unknown-suite"
	caseName := "unknown-case"
	if header := t.loadedHeader(); header != nil {
		suiteName = firstTapeValue(header.SuiteName, suiteName)
		caseName = firstTapeValue(header.CaseName, caseName)
	}
	refreshCmd := "agenttest refresh"
	if suiteName != "unknown-suite" && caseName != "unknown-case" {
		refreshCmd = fmt.Sprintf("agenttest refresh --suite %s --case %s", suiteName, caseName)
	}
	if entry.Kind != currentKind {
		return fmt.Sprintf("first request fingerprint mismatch - tape begins with %q but replay requested %q. Test inputs changed since recording. Re-record with: %s", entry.Kind, currentKind, refreshCmd)
	}
	return fmt.Sprintf("first request fingerprint mismatch - test inputs changed since recording. Re-record with: %s", refreshCmd)
}

func fingerprint(kind string, req tapeRequest) string {
	payload := struct {
		Kind string      `json:"kind"`
		Req  tapeRequest `json:"req"`
	}{
		Kind: kind,
		Req:  req,
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func readTape(path string) ([]tapeEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	var entries []tapeEntry
	for sc.Scan() {
		var e tapeEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func InspectTape(path string) (*TapeInspection, error) {
	entries, err := readTape(path)
	if err != nil {
		return nil, err
	}
	return inspectLoadedTape(path, entries)
}

func inspectLoadedTape(path string, entries []tapeEntry) (*TapeInspection, error) {
	if len(entries) == 0 {
		return nil, errors.New("tape is empty")
	}
	inspection := &TapeInspection{
		Path:       path,
		EntryCount: len(entries),
	}
	if entries[0].Kind == "_header" {
		inspection.Header = entries[0].Request.Header
		if inspection.Header == nil {
			log.Printf("WARNING: tape header entry missing header payload - cannot verify model compatibility. Consider re-recording.")
		}
	} else {
		inspection.Legacy = true
	}
	if first, ok := firstNonHeaderEntryFrom(entries, firstReplayIndex(entries)); ok {
		inspection.FirstEntryKind = first.Kind
		if inspection.Header != nil {
			if ts, err := time.Parse(time.RFC3339, inspection.Header.RecordedAt); err == nil {
				inspection.FirstRecordedAt = ts
			}
		}
		if inspection.FirstRecordedAt.IsZero() && !first.Timestamp.IsZero() {
			inspection.FirstRecordedAt = first.Timestamp
		}
	}
	return inspection, nil
}

func firstReplayIndex(entries []tapeEntry) int {
	if len(entries) > 0 && entries[0].Kind == "_header" {
		return 1
	}
	return 0
}

func firstNonHeaderEntryFrom(entries []tapeEntry, start int) (tapeEntry, bool) {
	for i := start; i < len(entries); i++ {
		if entries[i].Kind != "_header" {
			return entries[i], true
		}
	}
	return tapeEntry{}, false
}

func (t *TapeModel) loadedHeader() *TapeHeader {
	if len(t.loaded) == 0 || t.loaded[0].Kind != "_header" {
		return nil
	}
	return t.loaded[0].Request.Header
}

func firstTapeValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func currentFrameworkVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

func shortDigest(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "sha256:")
	if len(raw) > 12 {
		return raw[:12]
	}
	return raw
}
