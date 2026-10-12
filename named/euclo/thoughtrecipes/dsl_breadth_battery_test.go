package thoughtrecipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	executionprompt "codeburg.org/lexbit/relurpify/execution/prompt"
)

// batteryVocabulary accepts every declared name: the battery audits the
// recipe-side pipeline (parse → types → symbols → lower → graph), not the
// shipped capability corpus (the composition gate owns that).
type batteryVocabulary struct{}

func (batteryVocabulary) GetCapability(idOrName string) (descriptor.CapabilityDescriptor, bool) {
	return descriptor.CapabilityDescriptor{ID: idOrName}, true
}

func (batteryVocabulary) CallableCapabilityNames() []string { return nil }

// batteryPrompts claims every imported prompt exists: the corpus gate owns
// prompt-file existence; the battery owns the recipe-side import lowering.
type batteryPrompts struct{}

func (batteryPrompts) Get(id string) (*executionprompt.PromptConfig, bool) {
	return &executionprompt.PromptConfig{ID: id}, true
}

// TestDSLBreadthBattery loads the battery of author-facing recipes (one per
// construct family the language guide documents) through the production
// pipeline — parse, types, symbols, lower — and builds the executable graph
// for each, wired the way production wires it (capability vocabulary, prompt
// registry, grounding boundary). The shipped recipes exercise a fraction of
// the grammar; this battery is the construct-by-construct audit of what an
// end-user author can actually use.
func TestDSLBreadthBattery(t *testing.T) {
	dir := filepath.Join("testdata", "battery")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("battery dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no battery recipes found")
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".erpe") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			doc, err := ParseSource(name, string(source))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := NewTypeSystem(doc).Validate(); err != nil {
				t.Fatalf("types: %v", err)
			}
			symbols := NewSymbolTable(doc)
			symbols.WithCapabilityRegistry(batteryVocabulary{})
			symbols.WithPromptRegistry(batteryPrompts{})
			if err := symbols.Resolve(); err != nil {
				t.Fatalf("symbols: %v", err)
			}
			plan, err := LowerDocumentWithRegistry(doc, batteryVocabulary{})
			if err != nil {
				t.Fatalf("lower: %v", err)
			}
			var summary strings.Builder
			summary.WriteString("steps=" + itoa(len(plan.Steps)) + " routes=" + itoa(len(plan.Routes)))
			for _, step := range plan.Steps {
				summary.WriteString(" | kind=" + string(step.Kind) + " paradigm=" + step.Paradigm)
				if step.Stream != nil {
					summary.WriteString("+stream")
				}
				if step.PromptID != "" {
					summary.WriteString("+prompt:" + step.PromptID)
				}
				if len(step.CaptureBindings) > 0 {
					summary.WriteString("+captures")
				}
				if len(step.PipelineStages) > 0 {
					summary.WriteString("+stages")
				}
				if step.ChoiceSource != "" {
					summary.WriteString("+choices_from")
				}
				if step.ClarificationConfig != nil {
					summary.WriteString("+ask")
				}
			}
			t.Logf("lowered: %s", summary.String())
			// Graph build, wired as production wires it: grounding boundary
			// present. Failure here means the construct cannot become an
			// executable graph.
			deps := &paradigm.Deps{Grounder: buildProbeGroundingService(t)}
			graph, err := BuildThoughtRecipeGraph(plan, deps, nil)
			if err != nil {
				t.Fatalf("graph build: %v", err)
			}
			if graph == nil {
				t.Fatalf("graph nil")
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
