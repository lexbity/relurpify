package contextdata_test

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

func TestTypedOverlay_RoundTrip_String(t *testing.T) {
	overlay := contextdata.NewTypedOverlay[string]("test.key")
	env := contextdata.NewEnvelope("task-1", "")

	if _, ok := overlay.Get(env); ok {
		t.Fatal("expected absent before Set")
	}

	overlay.Set(env, "hello")

	got, ok := overlay.Get(env)
	if !ok {
		t.Fatal("expected present after Set")
	}
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestTypedOverlay_RoundTrip_Pointer(t *testing.T) {
	type payload struct{ N int }
	overlay := contextdata.NewTypedOverlay[*payload]("test.ptr")
	env := contextdata.NewEnvelope("task-1", "")

	want := &payload{N: 42}
	overlay.Set(env, want)

	got, ok := overlay.Get(env)
	if !ok {
		t.Fatal("expected present after Set")
	}
	if got != want {
		t.Fatalf("pointer identity lost: got %p, want %p", got, want)
	}
}

func TestTypedOverlay_NilEnv_IsNoOp(t *testing.T) {
	overlay := contextdata.NewTypedOverlay[string]("test.key")

	overlay.Set(nil, "should not panic")

	v, ok := overlay.Get(nil)
	if ok || v != "" {
		t.Fatal("expected (zero, false) for nil envelope")
	}
}

func TestTypedOverlay_TypeMismatch_ReturnsFalse(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	// Write an int under the key, read it back as string.
	intOverlay := contextdata.NewTypedOverlay[int]("test.key")
	strOverlay := contextdata.NewTypedOverlay[string]("test.key")

	intOverlay.Set(env, 99)

	_, ok := strOverlay.Get(env)
	if ok {
		t.Fatal("expected false when stored type does not match requested type")
	}
}

func TestTypedOverlay_Key_And_Class(t *testing.T) {
	overlay := contextdata.NewTypedOverlay[int]("my.key")
	if overlay.Key() != "my.key" {
		t.Fatalf("Key() = %q, want %q", overlay.Key(), "my.key")
	}
	if overlay.Class() != contextdata.MemoryClassTask {
		t.Fatalf("Class() = %q, want MemoryClassTask", overlay.Class())
	}
}

func TestTypedOverlayWithClass_Session(t *testing.T) {
	overlay := contextdata.NewTypedOverlayWithClass[string]("my.key", contextdata.MemoryClassSession)
	if overlay.Class() != contextdata.MemoryClassSession {
		t.Fatalf("Class() = %q, want MemoryClassSession", overlay.Class())
	}
}

func TestGetTyped_RoundTrip(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	contextdata.SetTyped(env, "typed.int", 7)

	got, ok := contextdata.GetTyped[int](env, "typed.int")
	if !ok {
		t.Fatal("expected present")
	}
	if got != 7 {
		t.Fatalf("got %d, want 7", got)
	}
}

func TestGetTyped_NilEnv(t *testing.T) {
	v, ok := contextdata.GetTyped[string](nil, "any.key")
	if ok || v != "" {
		t.Fatal("expected (zero, false) for nil envelope")
	}
}

func TestGetTyped_AbsentKey(t *testing.T) {
	env := contextdata.NewEnvelope("task-1", "")
	_, ok := contextdata.GetTyped[bool](env, "absent.key")
	if ok {
		t.Fatal("expected false for absent key")
	}
}

func TestSetTyped_NilEnv_IsNoOp(t *testing.T) {
	// Must not panic.
	contextdata.SetTyped[string](nil, "any.key", "value")
}
