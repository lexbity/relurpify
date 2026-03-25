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
	if len(delta.StateWrites) != 1 {
		t.Fatalf("expected 1 dirty state value, got %d", len(delta.StateWrites))
	}
	if len(delta.SideEffects.VariableWrites) != 1 {
		t.Fatalf("expected 1 dirty variable value, got %d", len(delta.SideEffects.VariableWrites))
	}
	if len(delta.SideEffects.KnowledgeWrites) != 1 {
		t.Fatalf("expected 1 dirty knowledge value, got %d", len(delta.SideEffects.KnowledgeWrites))
	}
	if !delta.SideEffects.HistoryChanged || !delta.SideEffects.CompressedChanged || !delta.SideEffects.PhaseChanged {
		t.Fatalf("expected history/compressed/phase mutations to be tracked: %+v", delta)
	}

	clone := ctx.Clone()
	cloneDelta := clone.DirtyDelta()
	if len(cloneDelta.StateWrites) != 0 || len(cloneDelta.SideEffects.VariableWrites) != 0 || len(cloneDelta.SideEffects.KnowledgeWrites) != 0 {
		t.Fatalf("expected cloned context to start clean, got %+v", cloneDelta)
	}
	if cloneDelta.SideEffects.HistoryChanged || cloneDelta.SideEffects.CompressedChanged || cloneDelta.SideEffects.LogChanged || cloneDelta.SideEffects.PhaseChanged {
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

func TestContextCloneStateOverlayFreezesParentViewAtCloneTime(t *testing.T) {
	original := NewContext()
	original.Set("shared", "base")

	clone := original.Clone()
	original.Set("shared", "parent-updated")

	if got := clone.GetString("shared"); got != "base" {
		t.Fatalf("expected clone to retain cloned parent view, got %q", got)
	}

	clone.Set("shared", "clone-updated")
	if got := original.GetString("shared"); got != "parent-updated" {
		t.Fatalf("expected parent to retain parent update, got %q", got)
	}
}

func TestContextCloneVariableAndKnowledgeOverlayFreezeParentViewAtCloneTime(t *testing.T) {
	original := NewContext()
	original.SetVariable("temp", map[string]interface{}{"value": "base"})
	original.SetKnowledge("fact", map[string]interface{}{"value": "base"})

	clone := original.Clone()
	original.SetVariable("temp", map[string]interface{}{"value": "parent-updated"})
	original.SetKnowledge("fact", map[string]interface{}{"value": "parent-updated"})

	temp, _ := clone.GetVariable("temp")
	tempMap := temp.(map[string]interface{})
	tempMap["value"] = "clone-updated"
	fact, _ := clone.GetKnowledge("fact")
	factMap := fact.(map[string]interface{})
	factMap["value"] = "clone-updated"

	origTemp, _ := original.GetVariable("temp")
	if got := origTemp.(map[string]interface{})["value"]; got != "parent-updated" {
		t.Fatalf("expected parent variable to retain parent update, got %#v", got)
	}
	origFact, _ := original.GetKnowledge("fact")
	if got := origFact.(map[string]interface{})["value"]; got != "parent-updated" {
		t.Fatalf("expected parent knowledge to retain parent update, got %#v", got)
	}
}
