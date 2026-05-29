package relurpicabilities

import (
	"context"
	"errors"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentspec"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/testsuite/testsupport"
)

func TestTestRunHandlerDescriptor(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{}
	handler := NewTestRunHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")

	desc := handler.Descriptor(ctx, envelope)

	if desc.ID != "euclo:cap.test_run" {
		t.Errorf("descriptor ID = %q, want %q", desc.ID, "euclo:cap.test_run")
	}

	if desc.Kind != agentspec.CapabilityKindTool {
		t.Errorf("descriptor Kind = %v, want %v", desc.Kind, agentspec.CapabilityKindTool)
	}

	if desc.RuntimeFamily != agentspec.CapabilityRuntimeFamilyRelurpic {
		t.Errorf("descriptor RuntimeFamily = %v, want %v", desc.RuntimeFamily, agentspec.CapabilityRuntimeFamilyRelurpic)
	}
}

func TestTestRunHandlerPassingTest(t *testing.T) {
	mockRunner := testsupport.FakeRunner(testsupport.FakeResponse{
		Stdout: "PASS: TestFoo\nPASS: TestBar\nok\tcodeburg.org/lexbit/relurpify\t0.002s",
	})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewTestRunHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"command": "go test ./...",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true")
	}

	passed, ok := result.Data["passed"].(bool)
	if !ok {
		t.Fatal("result.Data[\"passed\"] is not a bool")
	}
	if !passed {
		t.Errorf("passed = false, want true")
	}
}

func TestTestRunHandlerFailingTest(t *testing.T) {
	mockRunner := testsupport.FakeRunner(testsupport.FakeResponse{
		Stdout: "FAIL: TestFoo\n--- FAIL: TestBar (0.00s)\nFAIL",
		Stderr: "FAIL\tcodeburg.org/lexbit/relurpify\t0.002s",
	})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewTestRunHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"command": "go test ./...",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if !result.Success {
		t.Errorf("result.Success = false, want true (command executed)")
	}

	passed, ok := result.Data["passed"].(bool)
	if !ok {
		t.Fatal("result.Data[\"passed\"] is not a bool")
	}
	if passed {
		t.Errorf("passed = true, want false")
	}

	failedTests, ok := result.Data["failed_tests"].([]string)
	if !ok {
		t.Fatal("result.Data[\"failed_tests\"] is not a []string")
	}
	if len(failedTests) == 0 {
		t.Errorf("failed_tests is empty, want non-empty")
	}
}

func TestTestRunHandlerNilRunner(t *testing.T) {
	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: nil,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewTestRunHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"command": "go test ./...",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}

func TestTestRunHandlerCommandDenied(t *testing.T) {
	mockRunner := testsupport.FakeRunner(testsupport.FakeResponse{
		Err: errors.New("command denied by policy"),
	})

	wsEnv := agentenv.WorkspaceEnvironment{
		CommandRunner: mockRunner,
		CommandPolicy: allowCommandPolicy(),
	}
	handler := NewTestRunHandler(wsEnv)

	ctx := context.Background()
	envelope := contextdata.NewEnvelope("test-task", "test-session")
	args := map[string]interface{}{
		"command": "go test ./...",
	}

	result, err := handler.Invoke(ctx, envelope, args)
	if err != nil {
		t.Fatalf("Invoke returned error: %v", err)
	}

	if result.Success {
		t.Errorf("result.Success = true, want false")
	}

	errorMsg, ok := result.Data["error"].(string)
	if !ok {
		t.Fatal("result.Data[\"error\"] is not a string")
	}
	if errorMsg == "" {
		t.Errorf("error message is empty, want non-empty")
	}
}
