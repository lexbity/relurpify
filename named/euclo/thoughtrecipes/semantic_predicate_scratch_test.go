package thoughtrecipe

import (
	"testing"

	"codeburg.org/lexbit/relurpify/context/contextdata"
)

// TestSemanticPredicateScratchMatchesState is the R-7 detector: the predicate
// matcher resolves `state.x` and `scratch.x` subjects identically, because the
// namespace is dispatched at subject-resolution time only and the compiled
// matcher never branches on it. For identical values, the two namespaces must
// yield identical results across the whole predicate grammar.
func TestSemanticPredicateScratchMatchesState(t *testing.T) {
	type predicateCase struct {
		name  string
		pred  func(subject string) Predicate
		setup func(env *contextdata.Envelope, key string)
		want  bool
	}
	cases := []predicateCase{
		{
			name: "is",
			pred: func(subject string) Predicate {
				return Predicate{Subject: subject, Op: PredOpIs, Value: PredicateValue{StringVal: "ready"}}
			},
			setup: func(env *contextdata.Envelope, key string) {
				contextdata.SetTyped(env, key, "ready")
			},
			want: true,
		},
		{
			name: "contains",
			pred: func(subject string) Predicate {
				return Predicate{Subject: subject, Op: PredOpContains, Value: PredicateValue{StringVal: "rea"}}
			},
			setup: func(env *contextdata.Envelope, key string) {
				contextdata.SetTyped(env, key, "ready")
			},
			want: true,
		},
		{
			name: "missing",
			pred: func(subject string) Predicate {
				return Predicate{Subject: subject, Op: PredOpMissing}
			},
			setup: func(env *contextdata.Envelope, key string) {
				contextdata.SetTyped(env, key, "")
			},
			want: true,
		},
		{
			name: "present",
			pred: func(subject string) Predicate {
				return Predicate{Subject: subject, Op: PredOpPresent}
			},
			setup: func(env *contextdata.Envelope, key string) {
				contextdata.SetTyped(env, key, "ready")
			},
			want: true,
		},
		{
			name: "confidence below",
			pred: func(subject string) Predicate {
				return Predicate{Subject: subject, Op: PredOpConfidenceLT, Value: PredicateValue{Percent: 60}}
			},
			setup: func(env *contextdata.Envelope, key string) {
				contextdata.SetTyped(env, key, 40)
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, subject := range []string{"state.v", "scratch.v"} {
				t.Run(subject, func(t *testing.T) {
					env := contextdata.NewEnvelope("task", "session")
					tc.setup(env, subject)
					condition := compilePredicate(tc.pred(subject))
					if got := condition(nil, env); got != tc.want {
						t.Fatalf("predicate on %s = %v, want %v", subject, got, tc.want)
					}
				})
			}
		})
	}
}
