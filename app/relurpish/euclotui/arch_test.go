package euclotui

import (
	"os"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// bannedIdentifiers are identifiers that the euclotui package must never define
// or import — they were removed during the stepper rewrite.
var bannedIdentifiers = []string{
	"\nPhaseIdle",
	"\nPhaseIntake",
	"\nPhasePlan",
	"\nPhaseExecute",
	"\nPhaseVerify",
	"\nPhaseDone",
}

func TestNoReDerivedPhaseEnum(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		src := string(data)
		for _, banned := range bannedIdentifiers {
			if strings.Contains(src, banned) {
				t.Errorf("file %s contains banned identifier %q (old Phase enum)", entry.Name(), strings.TrimSpace(banned))
			}
		}
	}
}

func TestSurfaceTypesUsedForRecipeView(t *testing.T) {
	_ = surface.RecipeProjection{}
	_ = surface.ProjectedStep{}
	_ = surface.StepRuntime{}
	_ = surface.MacroPhase(0)
}
