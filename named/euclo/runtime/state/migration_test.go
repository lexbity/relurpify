package state

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	runtimepkg "github.com/lexcodex/relurpify/named/euclo/runtime"
)

// TestLoadFromContext_EmptyContext verifies that LoadFromContext on empty context returns zero-valued state.
func TestLoadFromContext_EmptyContext(t *testing.T) {
	ctx := core.NewContext()
	s := LoadFromContext(ctx)

	if s == nil {
		t.Fatal("expected non-nil state")
	}
	if !s.IsZero() {
		t.Error("expected IsZero to be true for empty context")
	}
	if s.VerificationPolicy.PolicyID != "" {
		t.Errorf("expected empty PolicyID, got %q", s.VerificationPolicy.PolicyID)
	}
	if s.Mode != "" {
		t.Errorf("expected empty Mode, got %q", s.Mode)
	}
}

// TestLoadFromContext_NilContext verifies that LoadFromContext with nil context returns zero-valued state.
func TestLoadFromContext_NilContext(t *testing.T) {
	s := LoadFromContext(nil)
	if s == nil {
		t.Fatal("expected non-nil state")
	}
	if !s.IsZero() {
		t.Error("expected IsZero to be true for nil context")
	}
}

// TestLoadFlushRoundTrip verifies that LoadFromContext followed by FlushToContext produces same values.
func TestLoadFlushRoundTrip(t *testing.T) {
	// Create a context with some data
	ctx1 := core.NewContext()
	now := time.Now().UTC()

	// Set various values
	SetVerificationPolicy(ctx1, runtimepkg.VerificationPolicy{
		PolicyID: "code/default",
		ModeID:   "code",
	})
	SetMode(ctx1, "code")
	SetExecutionProfile(ctx1, "default")
	SetUnitOfWork(ctx1, runtimepkg.UnitOfWork{ExecutionDescriptor: runtimepkg.ExecutionDescriptor{ModeID: "code"}, ID: "uow-1"})
	SetBehaviorTrace(ctx1, Trace{
		PrimaryCapabilityID: "euclo:chat.implement",
		Path:                "test_path",
	})

	// Load into state
	s := LoadFromContext(ctx1)

	// Verify loaded values
	if s.VerificationPolicy.PolicyID != "code/default" {
		t.Errorf("PolicyID: expected %q, got %q", "code/default", s.VerificationPolicy.PolicyID)
	}
	if s.Mode != "code" {
		t.Errorf("Mode: expected %q, got %q", "code", s.Mode)
	}
	if s.UnitOfWork.ID != "uow-1" {
		t.Errorf("UnitOfWork.ID: expected %q, got %q", "uow-1", s.UnitOfWork.ID)
	}
	if s.BehaviorTrace.PrimaryCapabilityID != "euclo:chat.implement" {
		t.Errorf("BehaviorTrace.PrimaryCapabilityID: expected %q, got %q", "euclo:chat.implement", s.BehaviorTrace.PrimaryCapabilityID)
	}

	// Flush to new context
	ctx2 := core.NewContext()
	s.FlushToContext(ctx2)

	// Verify flushed values
	policy, ok := GetVerificationPolicy(ctx2)
	if !ok {
		t.Error("expected VerificationPolicy to be set in ctx2")
	}
	if policy.PolicyID != "code/default" {
		t.Errorf("flushed PolicyID: expected %q, got %q", "code/default", policy.PolicyID)
	}

	mode, ok := GetMode(ctx2)
	if !ok || mode != "code" {
		t.Errorf("flushed Mode: expected %q, got %q, ok=%v", "code", mode, ok)
	}

	uow, ok := GetUnitOfWork(ctx2)
	if !ok {
		t.Error("expected UnitOfWork to be set in ctx2")
	}
	if uow.ID != "uow-1" {
		t.Errorf("flushed UnitOfWork.ID: expected %q, got %q", "uow-1", uow.ID)
	}

	// Ignore time comparison for this test - we just care about round-trip works
	_ = now
}

// TestLoadFromContext_LegacyMapRecoveryTrace verifies that a context written with raw keys (legacy) is correctly read.
func TestLoadFromContext_LegacyMapRecoveryTrace(t *testing.T) {
	ctx := core.NewContext()
	// Simulate legacy code writing a map
	legacyMap := map[string]any{
		"status":        "repair_exhausted",
		"attempt_count": 3,
		"max_attempts":  5,
	}
	ctx.Set(KeyRecoveryTrace, legacyMap)

	s := LoadFromContext(ctx)
	if s.RecoveryTrace.Status != "repair_exhausted" {
		t.Errorf("RecoveryTrace.Status: expected %q, got %q", "repair_exhausted", s.RecoveryTrace.Status)
	}
	if s.RecoveryTrace.AttemptCount != 3 {
		t.Errorf("RecoveryTrace.AttemptCount: expected %d, got %d", 3, s.RecoveryTrace.AttemptCount)
	}
}

// TestFlushToContext_UnknownKeysNotDisturbed verifies that FlushToContext doesn't disturb unknown keys.
func TestFlushToContext_UnknownKeysNotDisturbed(t *testing.T) {
	ctx := core.NewContext()
	// Set an unknown key
	ctx.Set("unknown.custom.key", "custom-value")

	// Create and flush state
	s := NewEucloExecutionState()
	s.Mode = "code"
	s.FlushToContext(ctx)

	// Check unknown key is still there
	if val, ok := ctx.Get("unknown.custom.key"); !ok || val != "custom-value" {
		t.Error("expected unknown key to be preserved")
	}

	// Check known key was set
	if mode, ok := GetMode(ctx); !ok || mode != "code" {
		t.Errorf("expected Mode to be 'code', got %q, ok=%v", mode, ok)
	}
}

// TestFlushToContext_PartialStateOnlyWritesNonZero verifies that FlushToContext only writes non-zero fields.
func TestFlushToContext_PartialStateOnlyWritesNonZero(t *testing.T) {
	ctx := core.NewContext()

	// Create state with only some fields set
	s := NewEucloExecutionState()
	s.Mode = "code" // Only set mode
	// Leave everything else at zero values

	s.FlushToContext(ctx)

	// Mode should be set
	if mode, ok := GetMode(ctx); !ok || mode != "code" {
		t.Errorf("expected Mode to be 'code', got %q, ok=%v", mode, ok)
	}

	// PolicyID should NOT be set (it was zero)
	if _, ok := GetVerificationPolicy(ctx); ok {
		t.Error("expected VerificationPolicy to NOT be set (was zero)")
	}

	// UnitOfWork should NOT be set (it was zero - ID is empty)
	if _, ok := GetUnitOfWork(ctx); ok {
		t.Error("expected UnitOfWork to NOT be set (was zero)")
	}
}

// TestEucloExecutionState_IsZero verifies IsZero returns correct values.
func TestEucloExecutionState_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		state    *EucloExecutionState
		expected bool
	}{
		{
			name:     "nil state",
			state:    nil,
			expected: true,
		},
		{
			name:     "new empty state",
			state:    NewEucloExecutionState(),
			expected: true,
		},
		{
			name: "state with mode set",
			state: func() *EucloExecutionState {
				s := NewEucloExecutionState()
				s.Mode = "code"
				return s
			}(),
			expected: false,
		},
		{
			name: "state with unit of work ID set",
			state: func() *EucloExecutionState {
				s := NewEucloExecutionState()
				s.UnitOfWork.ID = "uow-1"
				return s
			}(),
			expected: false,
		},
		{
			name: "state with assurance class set",
			state: func() *EucloExecutionState {
				s := NewEucloExecutionState()
				s.AssuranceClass = runtimepkg.AssuranceClassVerifiedSuccess
				return s
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.IsZero()
			if got != tt.expected {
				t.Errorf("IsZero() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

// TestFlushToContext_NilContext verifies FlushToContext is a no-op with nil context.
func TestFlushToContext_NilContext(t *testing.T) {
	s := NewEucloExecutionState()
	s.Mode = "code"
	// Should not panic
	s.FlushToContext(nil)
}

// TestFlushToContext_NilState verifies FlushToContext is a no-op with nil state.
func TestFlushToContext_NilState(t *testing.T) {
	ctx := core.NewContext()
	var s *EucloExecutionState = nil
	// Should not panic
	s.FlushToContext(ctx)
}
