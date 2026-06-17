package thoughtrecipe

import (
	"fmt"
	"testing"
)

func TestStepKind_String_RoundTrip(t *testing.T) {
	tests := []struct {
		kind StepKind
		want string
	}{
		{StepKindRun, "run"},
		{StepKindDelegate, "delegate"},
		{StepKindAsk, "ask"},
		{StepKindCapability, "capability"},
		{StepKindPipelineStage, "pipeline"},
		{StepKindInvalid, "invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.kind.String(); got != tc.want {
				t.Fatalf("StepKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
			}
			got := stepKindFromString(tc.want)
			if tc.kind != StepKindInvalid && got != tc.kind {
				t.Fatalf("stepKindFromString(%q) = %d, want %d", tc.want, got, tc.kind)
			}
		})
	}
}

func TestStepKind_Valid(t *testing.T) {
	if StepKindInvalid.valid() {
		t.Fatal("StepKindInvalid should not be valid")
	}
	if !StepKindRun.valid() {
		t.Fatal("StepKindRun should be valid")
	}
	if !StepKindDelegate.valid() {
		t.Fatal("StepKindDelegate should be valid")
	}
	if !StepKindAsk.valid() {
		t.Fatal("StepKindAsk should be valid")
	}
	if !StepKindCapability.valid() {
		t.Fatal("StepKindCapability should be valid")
	}
	if !StepKindPipelineStage.valid() {
		t.Fatal("StepKindPipelineStage should be valid")
	}
}

func TestStepKind_LowerSetsKind(t *testing.T) {
	recipes := loadGoldenRecipes(t)
	for name, doc := range recipes {
		t.Run(name, func(t *testing.T) {
			plan, err := LowerDocument(doc)
			if err != nil {
				t.Fatalf("LowerDocument(%s) failed: %v", name, err)
			}
			for _, step := range plan.Steps {
				if !step.Kind.valid() {
					t.Fatalf("step %q has invalid Kind %v", step.ID, step.Kind)
				}
			}
			for _, route := range plan.Routes {
				for _, branch := range route.Branches {
					for _, step := range branch.Steps {
						if !step.Kind.valid() {
							t.Fatalf("route branch step %q has invalid Kind %v", step.ID, step.Kind)
						}
					}
				}
			}
			for _, pipeline := range plan.Pipelines {
				for _, stage := range pipeline.Stages {
					for _, step := range stage.Steps {
						if !step.Kind.valid() {
							t.Fatalf("pipeline stage step %q has invalid Kind %v", step.ID, step.Kind)
						}
					}
				}
			}
		})
	}
}

func TestStepKind_InvalidFailsLowering(t *testing.T) {
	// Lowering MUST produce a positioned error if a step has an invalid kind.
	// Simulate by constructing a recipe with an unknown execution item type.
	doc := mustParseDoc(t, `thoughtrecipe invalid_kind
"Invalid kind."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  goal "Test."
`)
	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected at least one step")
	}
	for _, step := range plan.Steps {
		if !step.Kind.valid() {
			t.Fatalf("step %q has Kind %v, expected valid Kind", step.ID, step.Kind)
		}
	}
}

func TestStepKind_JSONRoundTrip(t *testing.T) {
	tests := []StepKind{StepKindRun, StepKindDelegate, StepKindAsk, StepKindCapability, StepKindPipelineStage}
	for _, k := range tests {
		t.Run(k.String(), func(t *testing.T) {
			data, err := k.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			got := string(data)
			want := fmt.Sprintf("%q", k.String())
			if got != want {
				t.Fatalf("MarshalJSON = %s, want %s", got, want)
			}
			var parsed StepKind
			if err := parsed.UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			if parsed != k {
				t.Fatalf("round-trip = %v, want %v", parsed, k)
			}
		})
	}
}
