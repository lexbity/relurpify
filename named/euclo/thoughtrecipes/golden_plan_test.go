package thoughtrecipe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoldenPlans(t *testing.T) {
	recipes := loadGoldenRecipes(t)
	for name, doc := range recipes {
		t.Run(name, func(t *testing.T) {
			plan, err := LowerDocument(doc)
			if err != nil {
				t.Fatalf("LowerDocument(%s) failed: %v", name, err)
			}
			goldenPath := goldenPath(t, name, "plan.json")
			got := marshalCanonicalJSON(t, plan)

			if update := os.Getenv("UPDATE_GOLDEN"); update != "" {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", goldenPath, err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (set UPDATE_GOLDEN=1 to create)", goldenPath, err)
			}
			if string(got) != string(want) {
				t.Fatalf("golden mismatch for %s\ngot:\n%s\nwant:\n%s\n(set UPDATE_GOLDEN=1 to re-baseline)",
					name, string(got), string(want))
			}
		})
	}
}

func TestGoldenPlansFailOnMutation(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe mutate_test
"Mutation test."

trigger as capability:
  may read workspace

agent reviewer uses react

run reviewer:
  goal "Original goal."
`)
	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatalf("LowerDocument failed: %v", err)
	}
	original := marshalCanonicalJSON(t, plan)
	if !strings.Contains(string(original), "Original goal.") {
		t.Fatal("mutation test: original JSON should contain original goal text")
	}

	plan.Steps[0].ID = "mutated.step.id"
	mutated := marshalCanonicalJSON(t, plan)
	if string(mutated) == string(original) {
		t.Fatal("mutation test: mutated JSON should differ from original")
	}
}

func goldenPath(t *testing.T, name, suffix string) string {
	t.Helper()
	return filepath.Join("testdata", "golden", name+"."+suffix)
}

func marshalCanonicalJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return data
}

func loadGoldenRecipes(t *testing.T) map[string]*ThoughtRecipeDocument {
	t.Helper()
	entries, err := os.ReadDir("testdata/recipes")
	if err != nil {
		t.Fatalf("read testdata/recipes: %v", err)
	}
	recipes := make(map[string]*ThoughtRecipeDocument)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if !IsAcceptedThoughtRecipeExtension(ext) {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		path := filepath.Join("testdata", "recipes", name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read recipe %s: %v", path, err)
		}
		doc, err := ParseSource("testdata/recipes/"+name, string(src))
		if err != nil {
			t.Fatalf("ParseSource(%s): %v", name, err)
		}
		recipes[base] = doc
	}
	if len(recipes) < 10 {
		t.Fatalf("expected at least 10 golden recipes, got %d", len(recipes))
	}
	return recipes
}
