package paradigm

import (
	"strings"
	"testing"
)

func testContract(name string) Contract {
	return Contract{
		Paradigm: name,
		Summary:  "test paradigm",
		Shape:    ShapeLinear,
		Directives: []DirectiveSpec{
			{Name: "goal", Form: FormLine, Text: ArgOne, Required: true},
			{Name: "stage", Form: FormBlock, Text: ArgNone, Body: []BodyItem{BodyItemRun, BodyItemDo}},
		},
		Conformance: []ConformanceCase{
			{ID: name + "/goal_required", Directive: "goal", Assertion: "goal feeds the step instruction"},
			{ID: name + "/stage_runs_do", Directive: "stage", Assertion: "stage body items execute in order"},
		},
	}
}

func TestContractRegistryRoundTrip(t *testing.T) {
	reg := NewContractRegistry()
	if err := reg.Register(testContract("alpha")); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}
	if err := reg.Register(testContract("beta")); err != nil {
		t.Fatalf("Register beta: %v", err)
	}

	c, ok := reg.Lookup("alpha")
	if !ok || c == nil {
		t.Fatal("expected alpha to be registered")
	}
	if c.Paradigm != "alpha" {
		t.Fatalf("contract paradigm = %q, want alpha", c.Paradigm)
	}
	if _, ok := reg.Lookup("missing"); ok {
		t.Fatal("did not expect missing paradigm to resolve")
	}
}

func TestContractRegistryNamesSorted(t *testing.T) {
	reg := NewContractRegistry()
	if err := reg.Register(testContract("zeta")); err != nil {
		t.Fatalf("register zeta: %v", err)
	}
	if err := reg.Register(testContract("alpha")); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	names := reg.Names()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("Names() = %v, want [alpha zeta] sorted", names)
	}
	all := reg.All()
	if len(all) != 2 || all[0].Paradigm != "alpha" || all[1].Paradigm != "zeta" {
		t.Fatalf("All() = %#v, want sorted [alpha zeta]", all)
	}
}

func TestContractRegistryDuplicateRegistrationFails(t *testing.T) {
	reg := NewContractRegistry()
	if err := reg.Register(testContract("dup")); err != nil {
		t.Fatalf("first register: %v", err)
	}
	err := reg.Register(testContract("dup"))
	if err == nil || !strings.Contains(err.Error(), "duplicate paradigm contract registration") {
		t.Fatalf("expected duplicate registration error, got %v", err)
	}
}

func TestContractRegistryRejectsEmptyName(t *testing.T) {
	reg := NewContractRegistry()
	if err := reg.Register(testContract("  ")); err == nil {
		t.Fatal("expected empty-name registration to fail")
	}
	if err := reg.Register(Contract{Paradigm: ""}); err == nil {
		t.Fatal("expected unnamed contract registration to fail")
	}
}

func TestContractRegistryRejectsInvalidContract(t *testing.T) {
	reg := NewContractRegistry()
	bad := testContract("broken")
	bad.Directives = []DirectiveSpec{
		{Name: "plan", Form: "inline" /* invalid form */, Text: ArgOne, Required: true},
	}
	if err := reg.Register(bad); err == nil {
		t.Fatal("expected invalid-form contract to be rejected")
	}
}

func TestContractRejectsUnpinnedDirective(t *testing.T) {
	reg := NewContractRegistry()
	c := testContract("unpinned")
	c.Directives = append(c.Directives, DirectiveSpec{Name: "verify", Form: FormLine, Text: ArgOne})
	if err := reg.Register(c); err == nil || !strings.Contains(err.Error(), "no conformance case") {
		t.Fatalf("expected unpinned directive rejection, got %v", err)
	}
}

func TestContractRejectsCasePinOfUndeclaredDirective(t *testing.T) {
	reg := NewContractRegistry()
	c := testContract("ghost")
	c.Conformance = append(c.Conformance, ConformanceCase{
		ID:        c.Paradigm + "/ghost_pin",
		Directive: "ghost",
		Assertion: "pins nothing",
	})
	if err := reg.Register(c); err == nil || !strings.Contains(err.Error(), "undeclared directive") {
		t.Fatalf("expected undeclared-pin rejection, got %v", err)
	}
}

func TestContractAcceptsAndSurfacesRequiredDirective(t *testing.T) {
	reg := NewContractRegistry()
	c := testContract("required")
	c.Directives = []DirectiveSpec{
		{Name: "goal", Form: FormLine, Text: ArgOne},
		{Name: "mode", Form: FormLine, Text: ArgOne, Required: true},
	}
	c.Conformance = []ConformanceCase{
		{ID: c.Paradigm + "/goal", Directive: "goal", Assertion: "goal feeds step"},
		{ID: c.Paradigm + "/mode", Directive: "mode", Assertion: "mode feeds step"},
	}
	if err := reg.Register(c); err != nil {
		t.Fatalf("expected valid required-directive contract to register: %v", err)
	}
	registered, ok := reg.Lookup(c.Paradigm)
	if !ok {
		t.Fatal("expected contract to be registered")
	}
	required := registered.RequiredNames()
	if len(required) != 1 || required[0] != "mode" {
		t.Fatalf("RequiredNames() = %v, want [mode]", required)
	}
}

func TestContractDirectiveLookup(t *testing.T) {
	c := testContract("lookup")
	spec, ok := c.Directive("goal")
	if !ok || spec.Name != "goal" || !spec.Required {
		t.Fatalf("Directive(goal) = %#v, %v; want required goal spec", spec, ok)
	}
	if _, ok := c.Directive("nope"); ok {
		t.Fatal("did not expect nope to resolve")
	}
	required := c.RequiredNames()
	if len(required) != 1 || required[0] != "goal" {
		t.Fatalf("RequiredNames() = %v, want [goal]", required)
	}
}

func TestErrUnknownParadigmMessage(t *testing.T) {
	err := &ErrUnknownParadigm{
		Paradigm: "goalcon",
		Valid:    []string{"planner", "react"},
		At:       ContractLocation{File: "x.erpe", Line: 12, Column: 5},
	}
	msg := err.Error()
	for _, want := range []string{"x.erpe:12:5", "unsupported agent paradigm", "goalcon", "planner, react"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
}

func TestRegistryRegisterHelperUsesGlobal(t *testing.T) {
	// The package helper writes into the process-wide registry; the entry is
	// keyed by an explicit name, so a repeated -count run in the same process
	// would already have it registered.
	const unique = "contract_helper_test"
	if _, ok := Registry.Lookup(unique); ok {
		t.Skip("test name already registered by a previous run in this process")
	}
	if err := Register(testContract(unique)); err != nil {
		t.Fatalf("Register helper: %v", err)
	}
	if _, ok := Registry.Lookup(unique); !ok {
		t.Fatal("expected helper registration to reach the global registry")
	}
}

func TestContractOrderRuleAccessor(t *testing.T) {
	var nilContract *Contract
	if nilContract.OrderRule() != nil {
		t.Fatal("nil contract must report no order rule")
	}
	plain := testContract("plain")
	if plain.OrderRule() != nil {
		t.Fatal("contract without an order rule must report nil")
	}
	ordered := testContract("ordered")
	ordered.Order = &OrderRule{Sequence: []string{"goal", "stage"}}
	if got := ordered.OrderRule(); got == nil || len(got.Sequence) != 2 {
		t.Fatalf("OrderRule() = %#v, want the declared rule", got)
	}
}

func TestContractRejectsOrderRuleReferencingUndeclaredDirective(t *testing.T) {
	reg := NewContractRegistry()
	c := testContract("badorder")
	c.Order = &OrderRule{Sequence: []string{"goal", "ghost"}}
	if err := reg.Register(c); err == nil || !strings.Contains(err.Error(), "undeclared directive") {
		t.Fatalf("expected order-rule rejection, got %v", err)
	}
}

func TestContractAcceptsValidOrderRule(t *testing.T) {
	reg := NewContractRegistry()
	c := testContract("goodorder")
	c.Order = &OrderRule{Sequence: []string{"goal", "stage"}}
	if err := reg.Register(c); err != nil {
		t.Fatalf("expected a valid order rule to register: %v", err)
	}
}

func TestErrDirectiveOrderMessage(t *testing.T) {
	err := &ErrDirectiveOrder{
		Paradigm:  "rewoo",
		Directive: "step",
		At:        ContractLocation{File: "r.erpe", Line: 5, Column: 3},
		After:     "synthesize",
		AfterAt:   ContractLocation{File: "r.erpe", Line: 7, Column: 3},
	}
	msg := err.Error()
	for _, want := range []string{"r.erpe:5:3", `"step"`, "rewoo", "out of order", `"synthesize"`, "r.erpe:7:3"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q missing %q", msg, want)
		}
	}
	// A predecessor without a source position falls back to a generic label.
	bare := &ErrDirectiveOrder{Paradigm: "rewoo", Directive: "plan", After: "synthesize"}
	if !strings.Contains(bare.Error(), "declared earlier") {
		t.Errorf("bare message %q missing the generic predecessor label", bare.Error())
	}
}

func TestErrContractViolationCauseAndUnwrap(t *testing.T) {
	cause := errString("rewoo step \"x\" requires a do clause")
	err := &ErrContractViolation{Step: "s1", Paradigm: "rewoo", Cause: cause}
	if !strings.Contains(err.Error(), "requires a do clause") {
		t.Fatalf("message %q missing the cause detail", err.Error())
	}
	if err.Unwrap() != cause {
		t.Fatal("Unwrap must expose the underlying cause")
	}
	plain := &ErrContractViolation{Step: "s1", Paradigm: "nosuch"}
	if !strings.Contains(plain.Error(), "no registered contract") {
		t.Fatalf("plain message %q missing the no-contract wording", plain.Error())
	}
	if plain.Unwrap() != nil {
		t.Fatal("Unwrap must be nil without a cause")
	}
}

// errString is a minimal error for exercising the typed guard's cause path.
type errString string

func (e errString) Error() string { return string(e) }
