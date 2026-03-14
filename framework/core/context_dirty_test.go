package core

import "testing"

func TestContextDirtyDeltaTracksMutationsAndCloneStartsClean(t *testing.T) {
	ctx := NewContext()
	ctx.Set("state.key", map[string]interface{}{"nested": "value"})
	ctx.SetVariable("temp.key", 42)
	ctx.SetKnowledge("fact.key", "known")
	ctx.AddInteraction("assistant", "hello", nil)
	ctx.AppendCompressedContext(CompressedContext{Summary: "compressed"})
	ctx.SetExecutionPhase("executing")

	delta := ctx.DirtyDelta()
	if len(delta.StateValues) != 1 {
		t.Fatalf("expected 1 dirty state value, got %d", len(delta.StateValues))
	}
	if len(delta.VariableValues) != 1 {
		t.Fatalf("expected 1 dirty variable value, got %d", len(delta.VariableValues))
	}
	if len(delta.KnowledgeValues) != 1 {
		t.Fatalf("expected 1 dirty knowledge value, got %d", len(delta.KnowledgeValues))
	}
	if !delta.HistoryChanged || !delta.CompressedChanged || !delta.PhaseChanged {
		t.Fatalf("expected history/compressed/phase mutations to be tracked: %+v", delta)
	}

	clone := ctx.Clone()
	cloneDelta := clone.DirtyDelta()
	if len(cloneDelta.StateValues) != 0 || len(cloneDelta.VariableValues) != 0 || len(cloneDelta.KnowledgeValues) != 0 {
		t.Fatalf("expected cloned context to start clean, got %+v", cloneDelta)
	}
	if cloneDelta.HistoryChanged || cloneDelta.CompressedChanged || cloneDelta.LogChanged || cloneDelta.PhaseChanged {
		t.Fatalf("expected cloned context mutation flags to be clear, got %+v", cloneDelta)
	}
}

func TestContextCloneUsesCopyOnWriteForTopLevelState(t *testing.T) {
	original := NewContext()
	original.Set("shared", "base")
	original.SetVariable("temp", "base")
	original.SetKnowledge("fact", "base")

	clone := original.Clone()
	clone.Set("shared", "clone")
	clone.SetVariable("temp", "clone")
	clone.SetKnowledge("fact", "clone")

	if got := original.GetString("shared"); got != "base" {
		t.Fatalf("expected original state to remain base, got %q", got)
	}
	if got, _ := original.GetVariable("temp"); got != "base" {
		t.Fatalf("expected original variable to remain base, got %#v", got)
	}
	if got, _ := original.GetKnowledge("fact"); got != "base" {
		t.Fatalf("expected original knowledge to remain base, got %#v", got)
	}

	original.Set("shared", "original")
	original.SetVariable("temp", "original")
	original.SetKnowledge("fact", "original")

	if got := clone.GetString("shared"); got != "clone" {
		t.Fatalf("expected clone state to remain clone, got %q", got)
	}
	if got, _ := clone.GetVariable("temp"); got != "clone" {
		t.Fatalf("expected clone variable to remain clone, got %#v", got)
	}
	if got, _ := clone.GetKnowledge("fact"); got != "clone" {
		t.Fatalf("expected clone knowledge to remain clone, got %#v", got)
	}
}

func TestContextCloneDeepCopiesNestedStateValues(t *testing.T) {
	original := NewContext()
	original.Set("nested", map[string]interface{}{
		"outer": map[string]interface{}{"value": "base"},
	})

	clone := original.Clone()
	raw, ok := clone.Get("nested")
	if !ok {
		t.Fatal("expected nested state in clone")
	}
	nested, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map, got %T", raw)
	}
	outer, ok := nested["outer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected outer nested map, got %T", nested["outer"])
	}
	outer["value"] = "clone-mutated"

	origRaw, ok := original.Get("nested")
	if !ok {
		t.Fatal("expected nested state in original")
	}
	origNested := origRaw.(map[string]interface{})
	origOuter := origNested["outer"].(map[string]interface{})
	if got := origOuter["value"]; got != "base" {
		t.Fatalf("expected original nested value to remain base, got %#v", got)
	}
}
