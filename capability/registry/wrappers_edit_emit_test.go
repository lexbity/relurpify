package registry

import (
	"context"
	"strings"
	"sync"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/capability/handler"
	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
	fwtelemetry "codeburg.org/lexbit/relurpify/telemetry"
)

// captureTelemetry implements fwtelemetry.Telemetry by recording emitted events.
type captureTelemetry struct {
	mu     sync.Mutex
	events []fwtelemetry.Event
}

func (c *captureTelemetry) Emit(ev fwtelemetry.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

// fakeWriteHandler is a stub capability handler for write-class invocations.
type fakeWriteHandler struct {
	handler.CapabilityHandler
	desc descriptor.CapabilityDescriptor
}

func (h *fakeWriteHandler) Descriptor(context.Context, ports.State) descriptor.CapabilityDescriptor {
	return h.desc
}

func (h *fakeWriteHandler) Invoke(_ context.Context, _ ports.State, _ map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true}, nil
}

// fakeReadHandler is a stub for read-class capabilities.
type fakeReadHandler struct {
	handler.CapabilityHandler
}

func (h *fakeReadHandler) Descriptor(context.Context, ports.State) descriptor.CapabilityDescriptor {
	return descriptor.CapabilityDescriptor{
		ID:            "file_read",
		EffectClasses: []classification.EffectClass{classification.EffectClassProcessSpawn},
	}
}

func (h *fakeReadHandler) Invoke(_ context.Context, _ ports.State, _ map[string]any) (*ports.ToolResult, error) {
	return &ports.ToolResult{Success: true, Data: map[string]any{"content": "hello"}}, nil
}

func TestEditEmit_WriteClassEmitsEventToolEdited(t *testing.T) {
	capture := &captureTelemetry{}
	reg := &CapabilityRegistry{telemetry: capture}

	handler := reg.wrapCapabilityHandlerPrepared(
		&fakeWriteHandler{
			desc: descriptor.CapabilityDescriptor{
				ID:            "file_edit",
				EffectClasses: []classification.EffectClass{classification.EffectClassFilesystemMutation},
			},
		},
		descriptor.CapabilityDescriptor{ID: "file_edit"},
		descriptorProfile{},
	)

	ih := handler.(instrumentCapabilityHandler)
	args := map[string]any{
		"path":       "demo.txt",
		"old_string": "hello",
		"new_string": "world",
	}
	result, err := ih.Invoke(context.Background(), nil, args)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}

	var found bool
	for _, ev := range capture.events {
		if ev.Type == fwtelemetry.EventToolEdited {
			found = true
			if path, _ := ev.Metadata["path"].(string); path != "demo.txt" {
				t.Fatalf("path = %q, want demo.txt", path)
			}
			if origin, _ := ev.Metadata["origin"].(string); origin != "file_edit" {
				t.Fatalf("origin = %q, want file_edit", origin)
			}
		}
	}
	if !found {
		t.Fatal("expected EventToolEdited event, got none")
	}
}

func TestEditEmit_WriteClassEmittedForFileWrite(t *testing.T) {
	capture := &captureTelemetry{}
	reg := &CapabilityRegistry{telemetry: capture}

	handler := reg.wrapCapabilityHandlerPrepared(
		&fakeWriteHandler{
			desc: descriptor.CapabilityDescriptor{
				ID:            "file_write",
				EffectClasses: []classification.EffectClass{classification.EffectClassFilesystemMutation},
			},
		},
		descriptor.CapabilityDescriptor{ID: "file_write"},
		descriptorProfile{},
	)

	ih := handler.(instrumentCapabilityHandler)
	args := map[string]any{
		"path":    "out.txt",
		"content": "new content\n",
	}
	result, err := ih.Invoke(context.Background(), nil, args)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}

	var found bool
	for _, ev := range capture.events {
		if ev.Type == fwtelemetry.EventToolEdited {
			found = true
			if path, _ := ev.Metadata["path"].(string); path != "out.txt" {
				t.Fatalf("path = %q", path)
			}
		}
	}
	if !found {
		t.Fatal("expected EventToolEdited for file_write")
	}
}

func TestEditEmit_ReadClassEmitsNoToolEdited(t *testing.T) {
	capture := &captureTelemetry{}
	reg := &CapabilityRegistry{telemetry: capture}

	handler := reg.wrapCapabilityHandlerPrepared(
		&fakeReadHandler{},
		descriptor.CapabilityDescriptor{ID: "file_read"},
		descriptorProfile{},
	)

	ih := handler.(instrumentCapabilityHandler)
	result, err := ih.Invoke(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}

	for _, ev := range capture.events {
		if ev.Type == fwtelemetry.EventToolEdited {
			t.Fatalf("unexpected EventToolEdited for read-class capability: %+v", ev)
		}
	}
}

func TestEditEmit_WriteClassPreviewRedacted(t *testing.T) {
	capture := &captureTelemetry{}
	reg := &CapabilityRegistry{telemetry: capture}

	handler := reg.wrapCapabilityHandlerPrepared(
		&fakeWriteHandler{
			desc: descriptor.CapabilityDescriptor{
				ID:            "file_write",
				EffectClasses: []classification.EffectClass{classification.EffectClassFilesystemMutation},
			},
		},
		descriptor.CapabilityDescriptor{ID: "file_write"},
		descriptorProfile{},
	)

	ih := handler.(instrumentCapabilityHandler)
	secret := "SECRET_API_KEY=sk-1234567890abcdef" //nolint:gosec // test fixture intentionally contains secret-shaped text for emit/redact coverage
	args := map[string]any{
		"path":    "config.txt",
		"content": secret,
	}
	_, err := ih.Invoke(context.Background(), nil, args)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	for _, ev := range capture.events {
		if ev.Type == fwtelemetry.EventToolEdited {
			if preview, ok := ev.Metadata["preview"].(string); ok {
				if strings.Contains(preview, "sk-") {
					t.Fatal("preview should not contain unredacted secret content (sk- pattern)")
				}
			}
		}
	}
}
