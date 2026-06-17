package thoughtrecipe

import (
	"testing"
)

func TestScope_OmittedDeniesAll(t *testing.T) {
	var s ResolvedToolScope // zero value — unresolved
	if s.IsResolved() {
		t.Fatal("zero value ResolvedToolScope should not be resolved")
	}
	if s.Permits("anything") {
		t.Fatal("zero value ResolvedToolScope must deny every tool (A-6, FR-3)")
	}
	if s.AllowedToolNames() != nil {
		t.Fatal("zero value ResolvedToolScope should return nil AllowedToolNames")
	}
}

func TestScope_DenyAllToolScope(t *testing.T) {
	s := DenyAllToolScope()
	if !s.IsResolved() {
		t.Fatal("DenyAllToolScope should be resolved")
	}
	if s.Permits("file_write") {
		t.Fatal("DenyAllToolScope must deny every tool")
	}
	if got := s.AllowedToolNames(); got != nil {
		t.Fatal("DenyAllToolScope should return nil AllowedToolNames")
	}
}

func TestScope_AllowTools(t *testing.T) {
	s := AllowTools([]string{"file_write", "file_read"})
	if !s.IsResolved() {
		t.Fatal("AllowTools scope should be resolved")
	}
	if !s.Permits("file_write") {
		t.Fatal("AllowTools should permit listed tools")
	}
	if !s.Permits("file_read") {
		t.Fatal("AllowTools should permit listed tools")
	}
	if s.Permits("file_search") {
		t.Fatal("AllowTools should deny unlisted tools")
	}
	got := s.AllowedToolNames()
	if len(got) != 2 || got[0] != "file_write" || got[1] != "file_read" {
		t.Fatalf("AllowedToolNames = %#v, want [file_write file_read]", got)
	}
}

func TestScope_AllowToolsNil(t *testing.T) {
	s := AllowTools(nil)
	if !s.IsResolved() {
		t.Fatal("AllowTools(nil) should be resolved")
	}
	if !s.Permits("anything") {
		t.Fatal("AllowTools(nil) should permit everything (unrestricted)")
	}
	if s.AllowedToolNames() != nil {
		t.Fatal("AllowTools(nil) should return nil AllowedToolNames")
	}
}

func TestScope_AllowToolsEmpty(t *testing.T) {
	s := AllowTools([]string{})
	if !s.IsResolved() {
		t.Fatal("AllowTools([]) should be resolved")
	}
	if !s.Permits("anything") {
		t.Fatal("AllowTools([]) should permit everything (unrestricted)")
	}
	if got := s.AllowedToolNames(); got != nil {
		t.Fatal("AllowTools([]) should return nil AllowedToolNames")
	}
}

func TestScope_AllowedToolNamesReturnsCopy(t *testing.T) {
	orig := []string{"file_write"}
	s := AllowTools(orig)
	got := s.AllowedToolNames()
	orig[0] = "file_delete"
	if got[0] == "file_delete" {
		t.Fatal("AllowedToolNames must return a copy, not alias the input")
	}
}

func TestScope_NestedDelegationInherits(t *testing.T) {
	// Verify that a step created from a parent inherits the parent's scope.
	parent := ExecutionStep{
		ID:    "parent",
		Scope: AllowTools([]string{"file_write"}),
	}
	child := ExecutionStep{
		ID:    "child",
		Scope: parent.Scope,
	}
	if !child.Scope.IsResolved() {
		t.Fatal("child scope should be resolved")
	}
	if !child.Scope.Permits("file_write") {
		t.Fatal("child should inherit parent's allowed tools")
	}
	if child.Scope.Permits("file_search") {
		t.Fatal("child should inherit parent's restrictions")
	}
}

func TestPlan_ImmutableAfterBuild(t *testing.T) {
	doc := mustParseDoc(t, `thoughtrecipe immutable_test
"Immutability test."

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

	// Capture original state.
	origStepID := plan.Steps[0].ID
	origScope := plan.Steps[0].Scope

	// Build the graph — must not mutate the plan.
	graph, err := BuildThoughtRecipeGraph(plan, nil, nil)
	if err != nil {
		t.Fatalf("BuildThoughtRecipeGraph failed: %v", err)
	}
	_ = graph

	// Verify plan is unchanged.
	if plan.Steps[0].ID != origStepID {
		t.Fatal("BuildThoughtRecipeGraph mutated step ID")
	}
	if plan.Steps[0].Scope.IsResolved() != origScope.IsResolved() {
		t.Fatal("BuildThoughtRecipeGraph mutated step scope")
	}
}

func TestScope_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		scope ResolvedToolScope
	}{
		{"deny-all", DenyAllToolScope()},
		{"allow-some", AllowTools([]string{"file_write", "file_read"})},
		{"unrestricted", AllowTools(nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.scope.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			var parsed ResolvedToolScope
			if err := parsed.UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			if parsed.IsResolved() != tc.scope.IsResolved() {
				t.Fatalf("IsResolved mismatch: got %v, want %v", parsed.IsResolved(), tc.scope.IsResolved())
			}
			if !equalStringSlices(parsed.AllowedToolNames(), tc.scope.AllowedToolNames()) {
				t.Fatalf("AllowedToolNames mismatch: got %#v, want %#v", parsed.AllowedToolNames(), tc.scope.AllowedToolNames())
			}
		})
	}
}
