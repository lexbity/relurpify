package pipeline

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/model"
)

func TestValidateStage(t *testing.T) {
	tests := []struct {
		name    string
		stage   Stage
		wantErr string
	}{
		{name: "nil stage rejected", stage: nil, wantErr: "pipeline stage required"},
		{
			name:    "blank name rejected",
			stage:   &fakeStage{name: "   ", contract: validContract()},
			wantErr: "pipeline stage name required",
		},
		{
			name:    "invalid contract rejected",
			stage:   &fakeStage{name: "s", contract: ContractDescriptor{}},
			wantErr: "contract name required",
		},
		{name: "valid stage accepted", stage: newFakeStage("s")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStage(tt.stage)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDecodeStageOutputAnnotatesFailures(t *testing.T) {
	resp := &model.LLMResponse{Text: "text"}

	out, err := DecodeStageOutput(newFakeStage("explore"), resp)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"ok": true}, out)

	_, err = DecodeStageOutput(&fakeStage{name: "s", contract: ContractDescriptor{}}, resp)
	require.Error(t, err)

	broken := newFakeStage("explore")
	broken.decodeErr = errors.New("bad json")
	_, err = DecodeStageOutput(broken, resp)
	require.Error(t, err)
	var decodeErr *DecodeError
	require.ErrorAs(t, err, &decodeErr)
	assert.Equal(t, "explore", decodeErr.Stage)
	assert.Equal(t, "fake.contract", decodeErr.Contract)
	assert.ErrorContains(t, decodeErr, "bad json")
}

func TestValidateStageOutputNormalizesErrors(t *testing.T) {
	stage := newFakeStage("explore")
	require.NoError(t, ValidateStageOutput(stage, map[string]any{}))

	plain := newFakeStage("explore")
	plain.validateErr = errors.New("missing field")
	err := ValidateStageOutput(plain, nil)
	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "explore", validationErr.Stage)
	assert.Equal(t, "fake.contract", validationErr.Contract)
	assert.Equal(t, "missing field", validationErr.Message)

	preTyped := newFakeStage("explore")
	preTyped.validateErr = &ValidationError{Message: "already typed"}
	err = ValidateStageOutput(preTyped, nil)
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "explore", validationErr.Stage, "blank stage must be filled in")
	assert.Equal(t, "fake.contract", validationErr.Contract, "blank contract must be filled in")
	assert.Equal(t, "already typed", validationErr.Message)

	invalid := &fakeStage{name: "s", contract: ContractDescriptor{}}
	err = ValidateStageOutput(invalid, nil)
	require.Error(t, err)
	var noValidationError *ValidationError
	assert.NotErrorAs(t, err, &noValidationError, "invalid stage fails before validation runs")
}

func TestApplyStageOutputAnnotatesFailures(t *testing.T) {
	stage := newFakeStage("explore")
	require.NoError(t, ApplyStageOutput(stage, nil, map[string]any{}))
	assert.Len(t, stage.applyCalls, 1)

	failing := newFakeStage("explore")
	failing.applyErr = errors.New("state closed")
	err := ApplyStageOutput(failing, nil, nil)
	var applyErr *ApplyError
	require.ErrorAs(t, err, &applyErr)
	assert.Equal(t, "explore", applyErr.Stage)
	assert.Equal(t, "fake.contract", applyErr.Contract)
	assert.ErrorContains(t, applyErr, "state closed")

	invalid := &fakeStage{name: "s", contract: ContractDescriptor{}}
	require.Error(t, ApplyStageOutput(invalid, nil, nil))
}
