package pipeline

import (
	"errors"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/core"
)

// Stage describes one typed unit of pipeline work.
type Stage interface {
	Name() string
	Contract() ContractDescriptor
	BuildPrompt(ctx *contextdata.Envelope) (string, error)
	Decode(resp *core.LLMResponse) (any, error)
	Validate(output any) error
	Apply(ctx *contextdata.Envelope, output any) error
}

// ToolScopedStage optionally narrows the tool set available to a stage.
type ToolScopedStage interface {
	AllowedToolNames() []string
}

// ToolRequiredStage marks stages that require at least one allowed tool to run
// before the stage output is accepted.
type ToolRequiredStage interface {
	RequiresToolExecution(task *core.Task, state *contextdata.Envelope, tools []core.Tool) bool
}

// ValidateStage checks stage identity and its declared contract metadata.
func ValidateStage(stage Stage) error {
	if stage == nil {
		return errors.New("pipeline stage required")
	}
	if strings.TrimSpace(stage.Name()) == "" {
		return errors.New("pipeline stage name required")
	}
	contract := stage.Contract()
	if err := contract.Validate(); err != nil {
		return fmt.Errorf("pipeline stage %s contract invalid: %w", stage.Name(), err)
	}
	return nil
}

// DecodeStageOutput annotates stage decode failures with stage/contract context.
func DecodeStageOutput(stage Stage, resp *core.LLMResponse) (any, error) {
	if err := ValidateStage(stage); err != nil {
		return nil, err
	}
	output, err := stage.Decode(resp)
	if err == nil {
		return output, nil
	}
	contract := stage.Contract()
	return nil, &DecodeError{
		Stage:    stage.Name(),
		Contract: contract.Name,
		Cause:    err,
	}
}

// ValidateStageOutput normalizes stage validation failures into ValidationError.
func ValidateStageOutput(stage Stage, output any) error {
	if err := ValidateStage(stage); err != nil {
		return err
	}
	if err := stage.Validate(output); err != nil {
		contract := stage.Contract()
		var validationErr *ValidationError
		if errors.As(err, &validationErr) {
			if validationErr.Stage == "" {
				validationErr.Stage = stage.Name()
			}
			if validationErr.Contract == "" {
				validationErr.Contract = contract.Name
			}
			return validationErr
		}
		return &ValidationError{
			Stage:    stage.Name(),
			Contract: contract.Name,
			Message:  err.Error(),
			Cause:    err,
		}
	}
	return nil
}

// ApplyStageOutput annotates stage projection failures with stage/contract context.
func ApplyStageOutput(stage Stage, ctx *contextdata.Envelope, output any) error {
	if err := ValidateStage(stage); err != nil {
		return err
	}
	if err := stage.Apply(ctx, output); err != nil {
		contract := stage.Contract()
		return &ApplyError{
			Stage:    stage.Name(),
			Contract: contract.Name,
			Cause:    err,
		}
	}
	return nil
}
