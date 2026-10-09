package thoughtrecipe

import (
	"encoding/json"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/descriptor"
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

// TestScope_AllowToolsNilDenies: a resolved-but-empty allowlist DENIES every
// tool — empty never means open (§5.8, D5). Programmatic construction that
// genuinely means unrestricted MUST use AllowAll.
func TestScope_AllowToolsNilDenies(t *testing.T) {
	for name, s := range map[string]ResolvedToolScope{
		"nil":   AllowTools(nil),
		"empty": AllowTools([]string{}),
	} {
		if !s.IsResolved() {
			t.Fatalf("%s: scope should be resolved", name)
		}
		if s.Permits("anything") {
			t.Fatalf("%s: resolved-but-empty scope must deny (empty never means open)", name)
		}
		if got := s.AllowedToolNames(); got != nil {
			t.Fatalf("%s: AllowedToolNames = %#v, want nil", name, got)
		}
	}
}

// TestScope_AllowAll: the explicit unrestricted scope — the only writer of
// that state.
func TestScope_AllowAll(t *testing.T) {
	s := AllowAll()
	if !s.IsResolved() {
		t.Fatal("AllowAll should be resolved")
	}
	if !s.Permits("anything") || !s.Permits("file_write") {
		t.Fatal("AllowAll must permit every tool")
	}
	if s.IsDenyAll() {
		t.Fatal("AllowAll is not deny-all")
	}
	if s.AllowedToolNames() != nil {
		t.Fatal("AllowAll carries no enumeration")
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

// TestScope_JSONRoundTrip: sentinel wire format and list preservation.
func TestScope_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		scope ResolvedToolScope
	}{
		{"deny-all", DenyAllToolScope()},
		{"allow-all", AllowAll()},
		{"allow-some", AllowTools([]string{"file_write", "file_read"})},
		{"resolved-empty", AllowTools(nil)},
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
			if parsed.Permits("probe") != tc.scope.Permits("probe") {
				t.Fatalf("Permits mismatch: got %v, want %v", parsed.Permits("probe"), tc.scope.Permits("probe"))
			}
			if !equalStringSlices(parsed.AllowedToolNames(), tc.scope.AllowedToolNames()) {
				t.Fatalf("AllowedToolNames mismatch: got %#v, want %#v", parsed.AllowedToolNames(), tc.scope.AllowedToolNames())
			}
		})
	}
}

// TestScope_SentinelWireFormat: "__deny_all__" is untouched; "__allow_all__"
// is the only representation of the explicit unrestricted scope.
func TestScope_SentinelWireFormat(t *testing.T) {
	deny, err := json.Marshal(DenyAllToolScope())
	if err != nil {
		t.Fatal(err)
	}
	if string(deny) != `["__deny_all__"]` {
		t.Fatalf("deny-all wire = %s", deny)
	}
	allow, err := json.Marshal(AllowAll())
	if err != nil {
		t.Fatal(err)
	}
	if string(allow) != `["__allow_all__"]` {
		t.Fatalf("allow-all wire = %s", allow)
	}
	var back ResolvedToolScope
	if err := json.Unmarshal([]byte(`["__allow_all__"]`), &back); err != nil {
		t.Fatal(err)
	}
	if !back.IsAllowAll() || !back.Permits("anything") {
		t.Fatalf("allow-all unmarshal broken: %+v", back)
	}
}

// TestCapabilityStepScope_OwnCapabilityOnly: without a registry the scope is
// exactly the named capability (§5.8: the named capability is the grant).
func TestCapabilityStepScope_OwnCapabilityOnly(t *testing.T) {
	scope := capabilityStepScope("euclo:cap.code_review", nil)
	if !scope.IsResolved() {
		t.Fatal("scope unresolved")
	}
	if !scope.Permits("euclo:cap.code_review") {
		t.Fatal("own capability must be permitted")
	}
	if scope.Permits("euclo:cap.other") {
		t.Fatal("other capabilities must be denied")
	}
	if names := scope.AllowedToolNames(); len(names) != 1 || names[0] != "euclo:cap.code_review" {
		t.Fatalf("allowed = %v", names)
	}
}

// TestCapabilityStepScope_UnionsCoordinationTargets: manifest-declared
// coordination targets union into the scope; nothing else is granted.
func TestCapabilityStepScope_UnionsCoordinationTargets(t *testing.T) {
	lookup := descriptorLookupFunc(func(id string) (descriptor.CapabilityDescriptor, bool) {
		if id == "euclo:cap.orchestrator" {
			return descriptor.CapabilityDescriptor{
				ID: id,
				Annotations: map[string]any{
					coordinationTargetsAnnotation: []any{"euclo:cap.test_run", "euclo:cap.ast_query"},
				},
			}, true
		}
		return descriptor.CapabilityDescriptor{}, false
	})
	scope := capabilityStepScope("euclo:cap.orchestrator", lookup)
	for _, permitted := range []string{"euclo:cap.orchestrator", "euclo:cap.test_run", "euclo:cap.ast_query"} {
		if !scope.Permits(permitted) {
			t.Errorf("declared target %q denied", permitted)
		}
	}
	if scope.Permits("euclo:cap.file_write") {
		t.Error("undeclared capability permitted")
	}
}

// TestCapabilityStepScope_UnregisteredCapability: a capability without a
// descriptor scopes to itself alone.
func TestCapabilityStepScope_UnregisteredCapability(t *testing.T) {
	scope := capabilityStepScope("euclo:cap.lonely", descriptorLookupFunc(func(string) (descriptor.CapabilityDescriptor, bool) {
		return descriptor.CapabilityDescriptor{}, false
	}))
	if !scope.Permits("euclo:cap.lonely") || scope.Permits("other") {
		t.Fatalf("scope = %v", scope.AllowedToolNames())
	}
}

// TestLowerCapabilityStepSetsScope: lowering a standalone capability step
// produces a resolved scope — the invariant the loader enforces.
func TestLowerCapabilityStepSetsScope(t *testing.T) {
	src := `thoughtrecipe standalone_cap
"Standalone capability step."

trigger as capability:
  may read workspace

route:
  otherwise:
    do relurpic:code_review on input.workspace
`
	doc, err := ParseSource("test.erpe", src)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := LowerDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	var walk func(steps []ExecutionStep)
	walk = func(steps []ExecutionStep) {
		for _, step := range steps {
			if step.Kind != StepKindCapability {
				continue
			}
			found++
			if !step.Scope.IsResolved() {
				t.Fatalf("capability step %q lowered with unresolved scope", step.ID)
			}
			if !step.Scope.Permits("euclo:cap.code_review") {
				t.Fatalf("own capability not permitted: %v", step.Scope.AllowedToolNames())
			}
			if step.Scope.Permits("euclo:cap.file_write") {
				t.Fatal("scope exceeds the named capability")
			}
		}
	}
	walk(plan.Steps)
	for _, route := range plan.Routes {
		for _, branch := range route.Branches {
			walk(branch.Steps)
		}
	}
	if found == 0 {
		t.Fatal("no capability step lowered")
	}
}

type descriptorLookupFunc func(string) (descriptor.CapabilityDescriptor, bool)

func (f descriptorLookupFunc) Select(id string) (descriptor.CapabilityDescriptor, bool) {
	return f(id)
}
