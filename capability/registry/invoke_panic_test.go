package registry

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
)

func TestPanicInToolExecuteReturnsError(t *testing.T) {
	result, err := recoverToolPanic(func() (*ports.ToolResult, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result from nil,nil return, got %+v", result)
	}

	result, err = recoverToolPanic(func() (*ports.ToolResult, error) {
		panic("injected panic for testing")
	})
	if err != nil {
		t.Fatalf("recoverToolPanic should not propagate the error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil after panic recovery")
	}
	if result.Success {
		t.Fatal("expected Success=false after panic recovery")
	}
	if !strings.Contains(result.Error, "tool panicked") {
		t.Fatalf("expected error message about panic, got: %q", result.Error)
	}
}

func TestPanicInToolExecuteLogsStack(t *testing.T) {
	var logBuf bytes.Buffer
	origOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(origOutput)

	_, _ = recoverToolPanic(func() (*ports.ToolResult, error) {
		panic("injected panic for testing")
	})

	logged := logBuf.String()
	if !strings.Contains(logged, "tool panic recovered") {
		t.Fatalf("expected log to contain 'tool panic recovered', got: %q", logged)
	}
	if !strings.Contains(logged, "injected panic for testing") {
		t.Fatalf("expected log to contain panic value, got: %q", logged)
	}
}

func TestPanicWithNilReturnDoesNotCrash(t *testing.T) {
	result, err := recoverToolPanic(func() (*ports.ToolResult, error) {
		defer func() { panic("deferred panic after nil,nil return") }()
		return nil, nil
	})
	if err != nil {
		t.Fatalf("recoverToolPanic should not propagate error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil after panic recovery")
	}
	if !strings.Contains(result.Error, "tool panicked") {
		t.Fatalf("expected error message about panic, got: %q", result.Error)
	}
}

func TestRecoverToolPanicNoPanic(t *testing.T) {
	result, err := recoverToolPanic(func() (*ports.ToolResult, error) {
		return &ports.ToolResult{Success: true, Data: map[string]any{"output": "ok"}}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.Success {
		t.Fatal("expected Success=true")
	}
	if result.Data["output"] != "ok" {
		t.Fatalf("expected output='ok', got %v", result.Data["output"])
	}
}

func TestRecoverToolPanicWithError(t *testing.T) {
	expectedErr := "some error"
	result, err := recoverToolPanic(func() (*ports.ToolResult, error) {
		return &ports.ToolResult{Success: false, Error: expectedErr}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Success {
		t.Fatal("expected Success=false")
	}
	if result.Error != expectedErr {
		t.Fatalf("expected error=%q, got %q", expectedErr, result.Error)
	}
}

func TestHashArgsDeterministicWithDifferentMapOrder(t *testing.T) {
	a := map[string]any{"b": 2, "a": 1, "c": []any{3, 4}}
	b := map[string]any{"c": []any{3, 4}, "a": 1, "b": 2}
	ha := hashArgs(a)
	hb := hashArgs(b)
	if ha != hb {
		t.Fatalf("deterministic HashArgs failed:\n  a=%q\n  b=%q", ha, hb)
	}
}

func TestHashArgsEmptyMap(t *testing.T) {
	h := hashArgs(map[string]any{})
	if h != "{}" {
		t.Fatalf("expected '{}', got %q", h)
	}
}

func TestRecordResultNilDataNoPanic(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	err := detector.RecordResult(
		descriptor.CapabilityDescriptor{ID: "test_tool", Name: "test_tool", Kind: agentspec.CapabilityKindTool},
		&ports.ToolResult{Success: true, Data: nil},
	)
	if err != nil {
		t.Fatalf("RecordResult with nil Data should not error: %v", err)
	}
}

func TestRecordResultNilResultNoPanic(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	err := detector.RecordResult(
		descriptor.CapabilityDescriptor{ID: "test_tool", Name: "test_tool", Kind: agentspec.CapabilityKindTool},
		nil,
	)
	if err != nil {
		t.Fatalf("RecordResult with nil result should not error: %v", err)
	}
}

func TestRecordResultNilMetadataNoPanic(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	err := detector.RecordResult(
		descriptor.CapabilityDescriptor{ID: "test_tool", Name: "test_tool", Kind: agentspec.CapabilityKindTool},
		&ports.ToolResult{Success: true, Data: map[string]any{"path": "/tmp/test"}, Metadata: nil},
	)
	if err != nil {
		t.Fatalf("RecordResult with nil Metadata should not error: %v", err)
	}
}
