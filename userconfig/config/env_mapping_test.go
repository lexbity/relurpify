// Phase 8 — Config discipline hardening: parseBoolEnv fails closed.

package config

import (
	"testing"
)

const falseEnvInput = "false"

// TestParseBoolEnvFailsClosed asserts that parseBoolEnv rejects unrecognized
// input with an error, rather than silently treating it as false. A
// RELURPIFY_STRICT typo like "flase" or "enabled" must fail the boot loudly.
func TestParseBoolEnvFailsClosed(t *testing.T) {
	tests := []struct {
		input   string
		want    bool
		wantErr bool
	}{
		// Accepted true values
		{input: "1", want: true, wantErr: false},
		{input: "true", want: true, wantErr: false},
		{input: "yes", want: true, wantErr: false},
		{input: "on", want: true, wantErr: false},
		{input: "TRUE", want: true, wantErr: false},
		{input: "Yes", want: true, wantErr: false},

		// Accepted false values
		{input: "0", want: false, wantErr: false},
		{input: falseEnvInput, want: false, wantErr: false},
		{input: "no", want: false, wantErr: false},
		{input: "off", want: false, wantErr: false},

		// Empty is accepted as false (unset env var)
		{input: "", want: false, wantErr: false},

		// Rejected: typos and unrecognized values
		{input: "flase", want: false, wantErr: true},
		{input: "enabled", want: false, wantErr: true},
		{input: "y", want: false, wantErr: true},
		{input: "n", want: false, wantErr: true},
		{input: "1.0", want: false, wantErr: true},
		{input: "t", want: false, wantErr: true},
		{input: "f", want: false, wantErr: true},
		{input: "disable", want: false, wantErr: true},
		{input: "enable", want: false, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parseBoolEnv(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("parseBoolEnv(%q) should error", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("parseBoolEnv(%q) should not error, got: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseBoolEnv(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
