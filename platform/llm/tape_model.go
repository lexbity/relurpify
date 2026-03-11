package llm

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lexcodex/relurpify/framework/core"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TapeMode string

const (
	TapeOff    TapeMode = "off"
	TapeRecord TapeMode = "record"
	TapeReplay TapeMode = "replay"
)

type tapeEntry struct {
	Timestamp   time.Time         `json:"timestamp"`
	Kind        string            `json:"kind"`
	Fingerprint string            `json:"fingerprint"`
	Request     tapeRequest       `json:"request"`
	Response    *core.LLMResponse `json:"response,omitempty"`
	Error       string            `json:"error,omitempty"`
}

type tapeRequest struct {
	Prompt    string           `json:"prompt,omitempty"`
	Messages  []core.Message   `json:"messages,omitempty"`
	ToolNames []string         `json:"tool_names,omitempty"`
	Options   *core.LLMOptions `json:"options,omitempty"`
}

type TapeModel struct {
	inner core.LanguageModel
	mode  TapeMode
	path  string

	mu     sync.Mutex
	file   *os.File
	enc    *json.Encoder
	loaded []tapeEntry
	next   int
}

func NewTapeModel(inner core.LanguageModel, path string, mode string) (*TapeModel, error) {
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
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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

func (t *TapeModel) Generate(ctx context.Context, prompt string, options *core.LLMOptions) (*core.LLMResponse, error) {
	req := tapeRequest{Prompt: prompt, Options: options}
	return t.roundTrip(ctx, "generate", req, func() (*core.LLMResponse, error) {
		return t.inner.Generate(ctx, prompt, options)
	})
}

func (t *TapeModel) GenerateStream(ctx context.Context, prompt string, options *core.LLMOptions) (<-chan string, error) {
	req := tapeRequest{Prompt: prompt, Options: options}
	fp := fingerprint("generate_stream", req)
	if t.mode == TapeReplay {
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
			Response:    &core.LLMResponse{Text: buf, FinishReason: "stream"},
		})
	}()
	return out, nil
}

func (t *TapeModel) Chat(ctx context.Context, messages []core.Message, options *core.LLMOptions) (*core.LLMResponse, error) {
	req := tapeRequest{Messages: messages, Options: options}
	return t.roundTrip(ctx, "chat", req, func() (*core.LLMResponse, error) {
		return t.inner.Chat(ctx, messages, options)
	})
}

func (t *TapeModel) ChatWithTools(ctx context.Context, messages []core.Message, tools []core.LLMToolSpec, options *core.LLMOptions) (*core.LLMResponse, error) {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	req := tapeRequest{Messages: messages, ToolNames: names, Options: options}
	return t.roundTrip(ctx, "chat_with_tools", req, func() (*core.LLMResponse, error) {
		return t.inner.ChatWithTools(ctx, messages, tools, options)
	})
}

func (t *TapeModel) roundTrip(ctx context.Context, kind string, req tapeRequest, call func() (*core.LLMResponse, error)) (*core.LLMResponse, error) {
	fp := fingerprint(kind, req)
	if t.mode == TapeReplay {
		entry, err := t.nextEntry(kind, fp)
		if err != nil {
			return nil, err
		}
		if entry.Error != "" {
			return nil, errors.New(entry.Error)
		}
		if entry.Response == nil {
			return &core.LLMResponse{}, nil
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

func (t *TapeModel) nextEntry(kind, fp string) (tapeEntry, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := t.next; i < len(t.loaded); i++ {
		entry := t.loaded[i]
		t.next = i + 1
		if entry.Kind == kind && entry.Fingerprint == fp {
			return entry, nil
		}
	}
	return tapeEntry{}, fmt.Errorf("tape exhausted or mismatch for %s fingerprint=%s", kind, fp)
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
