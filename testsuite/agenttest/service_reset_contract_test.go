package agenttest

import "testing"

func TestBuildServiceResetContractUsesDescriptorValues(t *testing.T) {
	desc := &PreparedRunDescriptor{
		ServiceResetStrategy: " restart ",
		ServiceResetBetween:  true,
		BackendService:       "ollama",
	}
	contract, err := BuildServiceResetContract(desc)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Strategy != "restart" {
		t.Fatalf("Strategy = %q, want restart", contract.Strategy)
	}
	if !contract.BetweenCases {
		t.Fatal("expected BetweenCases to be true")
	}
	if contract.ServiceTarget != "ollama" {
		t.Fatalf("ServiceTarget = %q, want ollama", contract.ServiceTarget)
	}
}

func TestServiceResetContractRequiresReset(t *testing.T) {
	if (ServiceResetContract{Strategy: "none"}).RequiresReset() {
		t.Fatal("none strategy should not require reset")
	}
	if !(ServiceResetContract{Strategy: "restart"}).RequiresReset() {
		t.Fatal("restart strategy should require reset")
	}
}

func TestServiceResetContractShouldResetBetweenCases(t *testing.T) {
	if (ServiceResetContract{Strategy: "none", BetweenCases: true}).ShouldResetBetweenCases() {
		t.Fatal("none strategy should not reset between cases")
	}
	if !(ServiceResetContract{Strategy: "restart", BetweenCases: true}).ShouldResetBetweenCases() {
		t.Fatal("restart strategy should reset between cases when requested")
	}
}
