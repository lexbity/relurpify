package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEucloCatalogSelectorsAndMetadata(t *testing.T) {
	catalog := newEucloCatalog()

	ask, err := catalog.ShowCapability("euclo:chat.ask")
	if err != nil {
		t.Fatal(err)
	}
	if ask.PrimaryOwner != "chat" {
		t.Fatalf("chat.ask primary owner = %q, want chat", ask.PrimaryOwner)
	}
	if ask.PreferredTestLayer != "baseline" || ask.ExecutionClass != "baseline_safe" {
		t.Fatalf("unexpected chat.ask layer metadata: %+v", ask)
	}
	if !hasString(ask.ExpectedArtifactKinds, "euclo.analyze") {
		t.Fatalf("expected chat.ask to advertise analyze artifacts, got %+v", ask.ExpectedArtifactKinds)
	}

	compilePlan, err := catalog.ShowCapability("euclo:archaeology.compile-plan")
	if err != nil {
		t.Fatal(err)
	}
	if compilePlan.PreferredTestLayer != "journey" || compilePlan.ExecutionClass != "journey_only" {
		t.Fatalf("unexpected compile-plan layer metadata: %+v", compilePlan)
	}
	if !hasString(compilePlan.AllowedTestLayers, "benchmark") {
		t.Fatalf("expected compile-plan to remain benchmark eligible: %+v", compilePlan.AllowedTestLayers)
	}

	prefixMatches, err := catalog.SelectCapabilities("euclo:chat.")
	if err != nil {
		t.Fatal(err)
	}
	if len(prefixMatches) < 3 {
		t.Fatalf("expected chat prefix to match multiple capabilities, got %d", len(prefixMatches))
	}

	modeMatches, err := catalog.SelectCapabilities("mode:planning")
	if err != nil {
		t.Fatal(err)
	}
	if len(modeMatches) == 0 {
		t.Fatal("expected planning mode selector to return entries")
	}

	triggerMatches, err := catalog.SelectCapabilities("trigger:alternatives")
	if err != nil {
		t.Fatal(err)
	}
	if len(triggerMatches) != 1 || triggerMatches[0].ID != "euclo:design.alternatives" {
		t.Fatalf("expected trigger selector to resolve design.alternatives, got %+v", triggerMatches)
	}

	trigger, ok := catalog.ResolveTrigger("planning", "alternatives")
	if !ok || trigger == nil {
		t.Fatal("expected planning trigger resolution to match alternatives")
	}
	if trigger.CapabilityID != "euclo:design.alternatives" || trigger.PhaseJump != "generate" {
		t.Fatalf("unexpected trigger resolution: %+v", trigger)
	}

	triggers := catalog.ListTriggers("planning")
	if len(triggers) == 0 {
		t.Fatal("expected planning triggers to be present")
	}
	found := false
	for _, entry := range triggers {
		if entry.CapabilityID == "euclo:design.alternatives" {
			found = true
			if entry.ModeIntentFamily != "planning" {
				t.Fatalf("unexpected planning trigger family: %+v", entry)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected planning trigger bindings to include design.alternatives")
	}
}

func TestEucloCatalogSnapshot(t *testing.T) {
	catalog := newEucloCatalog()
	snapshot := eucloCatalogSnapshot{
		Capabilities: []eucloCatalogSnapshotCapability{
			filterCatalogCapability(catalog, "euclo:chat.ask"),
			filterCatalogCapability(catalog, "euclo:archaeology.compile-plan"),
			filterCatalogCapability(catalog, "euclo:design.alternatives"),
			filterCatalogCapability(catalog, "euclo:trace.analyze"),
		},
		Triggers: []eucloCatalogSnapshotTrigger{
			filterCatalogTrigger(catalog, "planning", "alternatives"),
			filterCatalogTrigger(catalog, "planning", "just plan it"),
		},
	}

	got, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "euclo", "catalog_snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Fatalf("catalog snapshot mismatch\nwant:\n%s\ngot:\n%s", string(want), string(got))
	}
}

type eucloCatalogSnapshot struct {
	Capabilities []eucloCatalogSnapshotCapability `json:"capabilities"`
	Triggers     []eucloCatalogSnapshotTrigger    `json:"triggers"`
}

type eucloCatalogSnapshotCapability struct {
	ID                         string   `json:"id"`
	PrimaryOwner               string   `json:"primary_owner"`
	ModeFamily                 string   `json:"mode_family"`
	ExecutionClass             string   `json:"execution_class"`
	PreferredTestLayer         string   `json:"preferred_test_layer"`
	ExpectedArtifactKinds      []string `json:"expected_artifact_kinds,omitempty"`
	SupportedTransitionTargets []string `json:"supported_transition_targets,omitempty"`
}

type eucloCatalogSnapshotTrigger struct {
	Mode         string   `json:"mode"`
	Phrases      []string `json:"phrases"`
	CapabilityID string   `json:"capability_id,omitempty"`
	PhaseJump    string   `json:"phase_jump,omitempty"`
}

func filterCatalogCapability(catalog *EucloCatalog, id string) eucloCatalogSnapshotCapability {
	entry, _ := catalog.CapabilityByID(id)
	if entry == nil {
		return eucloCatalogSnapshotCapability{ID: id}
	}
	return eucloCatalogSnapshotCapability{
		ID:                         entry.ID,
		PrimaryOwner:               entry.PrimaryOwner,
		ModeFamily:                 entry.ModeFamily,
		ExecutionClass:             entry.ExecutionClass,
		PreferredTestLayer:         entry.PreferredTestLayer,
		ExpectedArtifactKinds:      append([]string(nil), entry.ExpectedArtifactKinds...),
		SupportedTransitionTargets: append([]string(nil), entry.SupportedTransitionTargets...),
	}
}

func filterCatalogTrigger(catalog *EucloCatalog, mode, phrase string) eucloCatalogSnapshotTrigger {
	for _, entry := range catalog.ListTriggers(mode) {
		for _, p := range entry.Phrases {
			if strings.EqualFold(strings.TrimSpace(p), strings.TrimSpace(phrase)) {
				return eucloCatalogSnapshotTrigger{
					Mode:         entry.Mode,
					Phrases:      append([]string(nil), entry.Phrases...),
					CapabilityID: entry.CapabilityID,
					PhaseJump:    entry.PhaseJump,
				}
			}
		}
	}
	return eucloCatalogSnapshotTrigger{Mode: mode, Phrases: []string{phrase}}
}

func hasString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestEucloCapabilityShowWritesDetail(t *testing.T) {
	catalog := newEucloCatalog()
	entry, err := catalog.ShowCapability("euclo:trace.analyze")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := writeCapabilityDetail(&out, entry); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "primary_owner") || !strings.Contains(text, "trace.analyze") {
		t.Fatalf("expected detail output to include canonical fields, got:\n%s", text)
	}
}
