package registry

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/ports"
)

const (
	Injectedpanicfortesting_invoke_panic_test = "injected panic for testing"
	Output_invoke_panic_test                  = "output"
	TestTool_invoke_panic_test                = "test_tool"
	Unexpectederrorv_invoke_panic_test        = "unexpected error: %v"
)

var errSentinel = errors.New("sentinel error")

func TestPanicInToolExecuteReturnsError(t *testing.T) {
	result, err := recoverToolPanic(func() (*ports.ToolResult, error) {
		return &ports.ToolResult{Success: true}, nil
	})
	if err != nil {
		t.Fatalf(Unexpectederrorv_invoke_panic_test, err)
	}
	if result.Success != true {
		t.Fatalf("expected Success=true, got %+v", result)
	}

	result, err = recoverToolPanic(func() (*ports.ToolResult, error) {
		panic(Injectedpanicfortesting_invoke_panic_test)
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
		panic(Injectedpanicfortesting_invoke_panic_test)
	})

	logged := logBuf.String()
	if !strings.Contains(logged, "tool panic recovered") {
		t.Fatalf("expected log to contain 'tool panic recovered', got: %q", logged)
	}
	if !strings.Contains(logged, Injectedpanicfortesting_invoke_panic_test) {
		t.Fatalf("expected log to contain panic value, got: %q", logged)
	}
}

func TestPanicWithNilReturnDoesNotCrash(t *testing.T) {
	result, err := recoverToolPanic(func() (*ports.ToolResult, error) {
		defer func() { panic("deferred panic after sentinel return") }()
		return nil, errSentinel
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
		return &ports.ToolResult{Success: true, Data: map[string]any{Output_invoke_panic_test: "ok"}}, nil
	})
	if err != nil {
		t.Fatalf(Unexpectederrorv_invoke_panic_test, err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.Success {
		t.Fatal("expected Success=true")
	}
	if result.Data[Output_invoke_panic_test] != "ok" {
		t.Fatalf("expected output='ok', got %v", result.Data[Output_invoke_panic_test])
	}
}

func TestRecoverToolPanicWithError(t *testing.T) {
	expectedErr := "some error"
	result, err := recoverToolPanic(func() (*ports.ToolResult, error) {
		return &ports.ToolResult{Success: false, Error: expectedErr}, nil
	})
	if err != nil {
		t.Fatalf(Unexpectederrorv_invoke_panic_test, err)
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
		descriptor.CapabilityDescriptor{ID: TestTool_invoke_panic_test, Name: TestTool_invoke_panic_test, Kind: agentspec.CapabilityKindTool},
		&ports.ToolResult{Success: true, Data: nil},
	)
	if err != nil {
		t.Fatalf("RecordResult with nil Data should not error: %v", err)
	}
}

func TestRecordResultNilResultNoPanic(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	err := detector.RecordResult(
		descriptor.CapabilityDescriptor{ID: TestTool_invoke_panic_test, Name: TestTool_invoke_panic_test, Kind: agentspec.CapabilityKindTool},
		nil,
	)
	if err != nil {
		t.Fatalf("RecordResult with nil result should not error: %v", err)
	}
}

func TestRecordResultNilMetadataNoPanic(t *testing.T) {
	detector := NewDoomLoopDetector(DefaultDoomLoopConfig())
	err := detector.RecordResult(
		descriptor.CapabilityDescriptor{ID: TestTool_invoke_panic_test, Name: TestTool_invoke_panic_test, Kind: agentspec.CapabilityKindTool},
		&ports.ToolResult{Success: true, Data: map[string]any{"path": "/tmp/test"}, Metadata: nil},
	)
	if err != nil {
		t.Fatalf("RecordResult with nil Metadata should not error: %v", err)
	}
}
