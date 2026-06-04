package surface

import (
	"testing"
)

// legacyParadigmList is the set of non-empty paradigm strings accepted by
// thoughtrecipes.schema.validateStepParadigm. Kept here as a guard against
// drift; the canonical list lives in this package.
var legacyValidateStepParadigm = []string{
	"react",
	"planner",
	"htn",
	"reflection",
	"blackboard",
	"chainer",
	"pipeline",
	"rewoo",
	"goalcon",
	"euclo",
}

// legacyAgentParadigmList is the set of strings accepted by
// thoughtrecipes.lowering.isSupportedAgentParadigm.
var legacyAgentParadigmList = []string{
	"react",
	"planner",
	"htn",
	"reflection",
	"blackboard",
	"chainer",
	"pipeline",
	"rewoo",
	"goalcon",
}

func TestAllParadigms_MatchesLegacyValidateList(t *testing.T) {
	got := AllParadigms()
	if len(got) != len(legacyValidateStepParadigm) {
		t.Fatalf("AllParadigms() returned %d paradigms; want %d", len(got), len(legacyValidateStepParadigm))
	}
	m := make(map[string]bool, len(got))
	for _, p := range got {
		if m[string(p)] {
			t.Errorf("AllParadigms() contains duplicate %q", p)
		}
		m[string(p)] = true
	}
	for _, want := range legacyValidateStepParadigm {
		if !m[want] {
			t.Errorf("AllParadigms() missing legacy paradigm %q", want)
		}
	}
}

func TestAgentParadigms_MatchesLegacyAgentList(t *testing.T) {
	got := AgentParadigms()
	if len(got) != len(legacyAgentParadigmList) {
		t.Fatalf("AgentParadigms() returned %d paradigms; want %d", len(got), len(legacyAgentParadigmList))
	}
	m := make(map[string]bool, len(got))
	for _, p := range got {
		if m[string(p)] {
			t.Errorf("AgentParadigms() contains duplicate %q", p)
		}
		m[string(p)] = true
	}
	for _, want := range legacyAgentParadigmList {
		if !m[want] {
			t.Errorf("AgentParadigms() missing legacy paradigm %q", want)
		}
	}
}

func TestIsSupported_ParityWithLegacyAgentList(t *testing.T) {
	for _, p := range AllParadigms() {
		got := IsSupported(p)
		want := false
		for _, lp := range legacyAgentParadigmList {
			if string(p) == lp {
				want = true
				break
			}
		}
		if got != want {
			t.Errorf("IsSupported(%q) = %v; want %v (legacy: isSupportedAgentParadigm)", p, got, want)
		}
	}
}

func TestIsSupported_EmptyString(t *testing.T) {
	if IsSupported("") {
		t.Error("IsSupported('') = true; want false")
	}
}

func TestIsSupported_Unknown(t *testing.T) {
	if IsSupported("unknown_paradigm") {
		t.Error("IsSupported('unknown_paradigm') = true; want false")
	}
}

func TestDescribe_AllParadigmsHaveMeta(t *testing.T) {
	for _, p := range AllParadigms() {
		m := p.Describe()
		if m.Label == "" {
			t.Errorf("Describe(%q).Label is empty", p)
		}
		if m.ShortGlyph == "" {
			t.Errorf("Describe(%q).ShortGlyph is empty", p)
		}
		if m.Family == "" {
			t.Errorf("Describe(%q).Family is empty", p)
		}
	}
}

func TestDescribe_UnknownFallback(t *testing.T) {
	m := Paradigm("bogus").Describe()
	if m.ShortGlyph != "??" {
		t.Errorf("Describe('bogus').ShortGlyph = %q; want '??'", m.ShortGlyph)
	}
}

func TestAllParadigms_ExhaustiveOverAgentParadigms(t *testing.T) {
	all := AllParadigms()
	agents := AgentParadigms()
	if len(all) <= len(agents) {
		t.Error("AllParadigms must have more entries than AgentParadigms (all includes euclo)")
	}
	agentSet := make(map[Paradigm]bool, len(agents))
	for _, p := range agents {
		agentSet[p] = true
	}
	if agentSet[ParadigmEuclo] {
		t.Error("AgentParadigms contains Euclo; want it excluded")
	}
	if !agentSet[ParadigmReact] {
		t.Error("AgentParadigms missing React")
	}
}
