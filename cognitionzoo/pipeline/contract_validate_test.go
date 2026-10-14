package pipeline

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractDescriptorValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ContractDescriptor)
		wantErr string
	}{
		{
			name:   "valid descriptor passes",
			mutate: func(d *ContractDescriptor) {},
		},
		{
			name:    "missing name rejected",
			mutate:  func(d *ContractDescriptor) { d.Name = "  " },
			wantErr: "contract name required",
		},
		{
			name:    "missing input key rejected",
			mutate:  func(d *ContractDescriptor) { d.Metadata.InputKey = "" },
			wantErr: "contract input key required",
		},
		{
			name:    "missing output key rejected",
			mutate:  func(d *ContractDescriptor) { d.Metadata.OutputKey = "" },
			wantErr: "contract output key required",
		},
		{
			name:    "missing schema version rejected",
			mutate:  func(d *ContractDescriptor) { d.Metadata.SchemaVersion = "" },
			wantErr: "contract schema version required",
		},
		{
			name:    "identical input and output keys rejected",
			mutate:  func(d *ContractDescriptor) { d.Metadata.OutputKey = d.Metadata.InputKey },
			wantErr: `contract input key "input" must differ from output key`,
		},
		{
			name:    "negative retry attempts rejected",
			mutate:  func(d *ContractDescriptor) { d.Metadata.RetryPolicy.MaxAttempts = -1 },
			wantErr: "contract retry max attempts cannot be negative",
		},
		{
			name: "zero retry attempts allowed",
			mutate: func(d *ContractDescriptor) {
				d.Metadata.RetryPolicy.MaxAttempts = 0
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := validContract()
			tt.mutate(&d)
			err := d.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPipelineErrorTypesAreNilSafe(t *testing.T) {
	var decodeErr *DecodeError
	var validationErr *ValidationError
	var applyErr *ApplyError

	assert.Empty(t, decodeErr.Error())
	assert.Empty(t, validationErr.Error())
	assert.Empty(t, applyErr.Error())
	assert.NoError(t, decodeErr.Unwrap())
	assert.NoError(t, validationErr.Unwrap())
	assert.NoError(t, applyErr.Unwrap())
}

func TestPipelineErrorMessagesAndUnwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")

	decodeErr := &DecodeError{Stage: "explore", Contract: "c1", Cause: cause}
	assert.Equal(t, "pipeline decode failed: stage=explore contract=c1: root cause", decodeErr.Error())
	assert.ErrorIs(t, errors.Unwrap(decodeErr), cause)
	assert.Equal(t,
		"pipeline decode failed: stage=explore contract=c1",
		(&DecodeError{Stage: "explore", Contract: "c1"}).Error())

	fieldErr := &ValidationError{Stage: "s", Contract: "c", Field: "name", Message: "missing"}
	assert.Equal(t, "pipeline validation failed: stage=s contract=c field=name: missing", fieldErr.Error())
	messageErr := &ValidationError{Stage: "s", Contract: "c", Message: "bad shape"}
	assert.Equal(t, "pipeline validation failed: stage=s contract=c: bad shape", messageErr.Error())
	causeErr := &ValidationError{Stage: "s", Contract: "c", Cause: cause}
	assert.Equal(t, "pipeline validation failed: stage=s contract=c: root cause", causeErr.Error())
	bareErr := &ValidationError{Stage: "s", Contract: "c"}
	assert.Equal(t, "pipeline validation failed: stage=s contract=c", bareErr.Error())
	assert.ErrorIs(t, errors.Unwrap(causeErr), cause)

	applyErr := &ApplyError{Stage: "plan", Contract: "c2", Cause: cause}
	assert.Equal(t, "pipeline apply failed: stage=plan contract=c2: root cause", applyErr.Error())
	assert.ErrorIs(t, errors.Unwrap(applyErr), cause)
	assert.Equal(t,
		"pipeline apply failed: stage=plan contract=c2",
		(&ApplyError{Stage: "plan", Contract: "c2"}).Error())
}
