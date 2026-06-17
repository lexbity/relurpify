package thoughtrecipe

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

func TestCompilePredicate_Is(t *testing.T) {
	p := Predicate{Subject: "state.intent", Op: PredOpIs, Value: PredicateValue{StringVal: "review"}}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.intent", "review", contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected state.intent=review to match is 'review'")
	}

	env.SetWorkingValueWithClass("state.intent", "refactor", contextdata.MemoryClassTask)
	if cond(nil, env) {
		t.Fatal("expected state.intent=refactor to NOT match is 'review'")
	}
}

func TestCompilePredicate_Is_MissingKey(t *testing.T) {
	p := Predicate{Subject: "state.missing", Op: PredOpIs, Value: PredicateValue{StringVal: "anything"}}
	env := contextdata.NewEnvelope("t", "s")
	cond := compilePredicate(p)
	if cond(nil, env) {
		t.Fatal("expected missing key to NOT match is")
	}
}

func TestCompilePredicate_Contains(t *testing.T) {
	p := Predicate{Subject: "state.items", Op: PredOpContains, Value: PredicateValue{StringVal: "target"}}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.items", []string{"alpha", "target", "omega"}, contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected state.items containing 'target' to match contains")
	}

	env.SetWorkingValueWithClass("state.items", []string{"alpha", "beta"}, contextdata.MemoryClassTask)
	if cond(nil, env) {
		t.Fatal("expected state.items without 'target' to NOT match contains")
	}
}

func TestCompilePredicate_Contains_String(t *testing.T) {
	p := Predicate{Subject: "state.text", Op: PredOpContains, Value: PredicateValue{StringVal: "keyword"}}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.text", "this contains keyword here", contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected string containing 'keyword' to match contains")
	}
}

func TestCompilePredicate_Missing(t *testing.T) {
	p := Predicate{Subject: "state.optional", Op: PredOpMissing}

	env := contextdata.NewEnvelope("t", "s")
	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected missing key to match missing")
	}

	env.SetWorkingValueWithClass("state.optional", "present", contextdata.MemoryClassTask)
	if cond(nil, env) {
		t.Fatal("expected present key to NOT match missing")
	}
}

func TestCompilePredicate_Missing_FalsyValue(t *testing.T) {
	p := Predicate{Subject: "state.flag", Op: PredOpMissing}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.flag", false, contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	// Old semantics: truthy(false) = false, so missing returns !false = true.
	// A falsy present value is treated as "missing" by the truthy convention.
	if !cond(nil, env) {
		t.Fatal("expected falsy value false to match missing (truthy semantics)")
	}
}

func TestCompilePredicate_Present(t *testing.T) {
	p := Predicate{Subject: "state.defined", Op: PredOpPresent}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.defined", "value", contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected present key to match present")
	}

	env2 := contextdata.NewEnvelope("t2", "s2")
	if cond(nil, env2) {
		t.Fatal("expected missing key to NOT match present")
	}
}

func TestCompilePredicate_ConfidenceBelow(t *testing.T) {
	p := Predicate{Subject: "state.intent", Op: PredOpConfidenceLT, Value: PredicateValue{Percent: 70}}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.intent_confidence", 50, contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected confidence 50 to be below 70")
	}

	env.SetWorkingValueWithClass("state.intent_confidence", 80, contextdata.MemoryClassTask)
	if cond(nil, env) {
		t.Fatal("expected confidence 80 to NOT be below 70")
	}
}

func TestCompilePredicate_ConfidenceBelow_DotNotation(t *testing.T) {
	p := Predicate{Subject: "state.intent", Op: PredOpConfidenceLT, Value: PredicateValue{Percent: 50}}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.intent.confidence", 30, contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected confidence via .confidence suffix to be detected")
	}
}

func TestCompilePredicate_ConfidenceBelow_FloatPercent(t *testing.T) {
	p := Predicate{Subject: "state.intent", Op: PredOpConfidenceLT, Value: PredicateValue{Percent: 60}}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.intent.confidence", 0.45, contextdata.MemoryClassTask)

	cond := compilePredicate(p)
	if !cond(nil, env) {
		t.Fatal("expected float confidence 0.45 (45%) to be below 60")
	}
}

func TestCompilePredicate_ConfidenceBelow_MissingConfidence(t *testing.T) {
	p := Predicate{Subject: "state.unknown", Op: PredOpConfidenceLT, Value: PredicateValue{Percent: 50}}

	env := contextdata.NewEnvelope("t", "s")
	cond := compilePredicate(p)
	if cond(nil, env) {
		t.Fatal("expected missing confidence to NOT match")
	}
}

func TestCompilePredicate_UnknownOp_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown PredicateOp")
		}
	}()
	compilePredicate(Predicate{Subject: "x", Op: PredOpInvalid})
}

func TestCompilePredicate_ClosuresArePure(t *testing.T) {
	p := Predicate{Subject: "state.val", Op: PredOpIs, Value: PredicateValue{StringVal: "hello"}}

	env := contextdata.NewEnvelope("t", "s")
	env.SetWorkingValueWithClass("state.val", "hello", contextdata.MemoryClassTask)

	cond := compilePredicate(p)

	// Call multiple times — must give same result (no captured mutable state).
	for i := 0; i < 5; i++ {
		if !cond(nil, env) {
			t.Fatalf("iteration %d: expected consistent match for pure closure", i)
		}
	}
}


