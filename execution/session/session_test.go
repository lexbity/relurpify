package session

import (
	"context"
	"errors"
	"testing"
)

func TestSessionClose_NilSafe(t *testing.T) {
	var s *WorkspaceSession
	err := s.Close(context.Background())
	if err != nil {
		t.Errorf("Close on nil session: %v", err)
	}
}

func TestSessionClose_Idempotent(t *testing.T) {
	callCount := 0
	s := &WorkspaceSession{
		ID: "test",
		closeFn: func(ctx context.Context) error {
			callCount++
			return nil
		},
	}

	ctx := context.Background()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("closeFn called %d times, want 1", callCount)
	}

	// Second call must be no-op.
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if callCount != 1 {
		t.Errorf("closeFn called %d times after second Close, want 1", callCount)
	}
}

func TestSessionClose_ReturnsError(t *testing.T) {
	want := errors.New("close error")
	s := &WorkspaceSession{
		ID: "test",
		closeFn: func(ctx context.Context) error {
			return want
		},
	}

	err := s.Close(context.Background())
	if !errors.Is(err, want) {
		t.Errorf("Close returned %v, want %v", err, want)
	}
}

func TestSessionClose_NilCloseFn(t *testing.T) {
	s := &WorkspaceSession{
		ID:      "test",
		closeFn: nil,
	}
	err := s.Close(context.Background())
	if err != nil {
		t.Errorf("Close with nil closeFn: %v", err)
	}
}

func TestSessionClose_SecondCloseReturnsNil(t *testing.T) {
	// After the first Close reports an error, the second Close must be
	// a no-op returning nil (the sync.Once is already spent).
	callCount := 0
	s := &WorkspaceSession{
		ID: "test",
		closeFn: func(ctx context.Context) error {
			callCount++
			return errors.New("close error")
		},
	}

	err1 := s.Close(context.Background())
	err2 := s.Close(context.Background())

	if callCount != 1 {
		t.Errorf("closeFn called %d times, want 1", callCount)
	}
	if err1 == nil {
		t.Error("first Close should return error")
	}
	if err2 != nil {
		t.Errorf("second Close should return nil, got %v", err2)
	}
}

func TestOpenModeValues(t *testing.T) {
	if OpenModeDefault != 0 {
		t.Errorf("OpenModeDefault = %d, want 0", OpenModeDefault)
	}
	if OpenModeEmbeddedAgent != 1 {
		t.Errorf("OpenModeEmbeddedAgent = %d, want 1", OpenModeEmbeddedAgent)
	}
}
