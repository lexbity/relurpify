package pipeline

import "testing"

func TestContractDescriptorValidate(t *testing.T) {
	valid := ContractDescriptor{
		Name: "analyze",
		Metadata: ContractMetadata{
			InputKey:      "pipeline.input",
			OutputKey:     "pipeline.output",
			SchemaVersion: "v1",
			RetryPolicy: RetryPolicy{
				MaxAttempts: 1,
			},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid contract, got %v", err)
	}
}

func TestContractMetadataValidateRejectsInvalidValues(t *testing.T) {
	cases := []ContractMetadata{
		{},
		{InputKey: "in", OutputKey: "in", SchemaVersion: "v1"},
		{InputKey: "in", OutputKey: "out", SchemaVersion: "", RetryPolicy: RetryPolicy{MaxAttempts: 1}},
		{InputKey: "in", OutputKey: "out", SchemaVersion: "v1", RetryPolicy: RetryPolicy{MaxAttempts: -1}},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected invalid contract metadata: %+v", tc)
		}
	}
}
