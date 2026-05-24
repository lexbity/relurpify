package tui

import (
	"errors"
	"testing"
)

func TestPTYSafePassesThroughNilError(t *testing.T) {
	err := PTYSafe(func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
}

func TestPTYSafePassesThroughError(t *testing.T) {
	want := errors.New("some error")
	got := PTYSafe(func() error {
		return want
	})
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestPTYSafeRecoversPanic(t *testing.T) {
	err := PTYSafe(func() error {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected error from panic, got nil")
	}
}

func TestPTYSafeRecoversNilPanic(t *testing.T) {
	err := PTYSafe(func() error {
		panic(nil)
	})
	if err == nil {
		t.Fatal("expected error from panic(nil), got nil")
	}
}
