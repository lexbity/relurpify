package safety

import "testing"

func TestRuntimeSafetySpecValidate(t *testing.T) {
	if err := (RuntimeSafetySpec{MaxCallsPerCapability: 1, MaxBytesPerSession: 10}).Validate(); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	if err := (RuntimeSafetySpec{MaxCallsPerProvider: -1}).Validate(); err == nil {
		t.Fatal("negative limit accepted; want error")
	}
}

func TestRuntimeSafetySpecRedactionEnabled(t *testing.T) {
	if !(RuntimeSafetySpec{}).RedactionEnabled() {
		t.Fatal("redaction should default to enabled when unset")
	}
	off := false
	if (RuntimeSafetySpec{RedactSensitiveMetadata: &off}).RedactionEnabled() {
		t.Fatal("redaction should be disabled when explicitly false")
	}
	on := true
	if !(RuntimeSafetySpec{RedactSensitiveMetadata: &on}).RedactionEnabled() {
		t.Fatal("redaction should be enabled when explicitly true")
	}
}
