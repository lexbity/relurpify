package chainer

import (
	"bytes"
	"fmt"
	"text/template"

	chainererrors "codeburg.org/lexbit/relurpify/agents/chainer/errors"
	"codeburg.org/lexbit/relurpify/agents/chainer/validation"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/pipeline"
)

// LinkStage wraps a chainer Link and implements pipeline.Stage.
//
// Each link receives only its declared InputKeys (input isolation) and writes
// exactly one output at its OutputKey. The contract declares these in the schema.
//
// This type lives in the chainer package (not stages) to avoid import cycles:
// - stages imports chainer for the Link type, so LinkStage must be in chainer
// - chainer_agent.go can import LinkStage without creating a cycle
type LinkStage struct {
	Link             *Link
	Model            core.LanguageModel
	LLMOptions       *core.LLMOptions
	linkNameOverride string // optional: override link name for debugging
}

// NewLinkStage creates a Stage wrapper for a Link.
func NewLinkStage(link *Link, model core.LanguageModel) *LinkStage {
	return &LinkStage{
		Link:  link,
		Model: model,
	}
}

// NewLinkStageWithOptions creates a Stage wrapper with custom LLM options.
func NewLinkStageWithOptions(link *Link, model core.LanguageModel, opts *core.LLMOptions) *LinkStage {
	return &LinkStage{
		Link:       link,
		Model:      model,
		LLMOptions: opts,
	}
}

// Name returns the link name for logging.
func (s *LinkStage) Name() string {
	if s == nil || s.Link == nil {
		return ""
	}
	if s.linkNameOverride != "" {
		return s.linkNameOverride
	}
	return s.Link.Name
}

// Contract declares input/output schema for this link.
// Asserts that:
//   - InputKey: first element of Link.InputKeys (or "__chainer_instruction" as fallback)
//   - OutputKey: Link.OutputKey
//   - SchemaVersion: "1.0"
//   - RetryPolicy: Retry on validation failure (maps from Link.OnFailure)
func (s *LinkStage) Contract() pipeline.ContractDescriptor {
	if s == nil || s.Link == nil {
		return pipeline.ContractDescriptor{}
	}

	inputKey := "__chainer_instruction"
	if len(s.Link.InputKeys) > 0 {
		inputKey = s.Link.InputKeys[0]
	}

	maxAttempts := 2
	if s.Link.MaxRetries > 0 {
		maxAttempts = s.Link.MaxRetries + 1
	}

	retryPolicy := pipeline.RetryPolicy{
		MaxAttempts:            maxAttempts,
		RetryOnDecodeError:     true,
		RetryOnValidationError: true,
	}
	if s.Link.OnFailure == FailurePolicyFailFast {
		retryPolicy = pipeline.RetryPolicy{MaxAttempts: 1}
	}

	return pipeline.ContractDescriptor{
		Name: fmt.Sprintf("chainer.%s", s.Link.Name),
		Metadata: pipeline.ContractMetadata{
			InputKey:      inputKey,
			OutputKey:     s.Link.OutputKey,
			SchemaVersion: "1.0",
			RetryPolicy:   retryPolicy,
		},
	}
}

// BuildPrompt renders the link's system prompt with filtered inputs.
// Isolates the stage: template sees only keys in Link.InputKeys (plus .Instruction).
func (s *LinkStage) BuildPrompt(ctx *core.Context) (string, error) {
	if s == nil || s.Link == nil {
		return "", fmt.Errorf("chainer: nil LinkStage or Link")
	}
	if ctx == nil {
		ctx = core.NewContext()
	}

	// Filter state to only declared input keys
	filtered := filterStateForStage(ctx, s.Link.InputKeys)

	// Get task instruction from context (stored during execute)
	var instruction string
	if instValue, ok := ctx.Get("__chainer_instruction"); ok {
		if instStr, ok := instValue.(string); ok {
			instruction = instStr
		}
	}

	// Render the link's system prompt with filtered inputs
	prompt, err := renderPromptForStage(s.Link.SystemPrompt, instruction, filtered)
	if err != nil {
		return "", fmt.Errorf("chainer: link %s prompt render: %w", s.Link.Name, err)
	}
	return prompt, nil
}

// Decode invokes the Link's Parse function (if set), or returns raw text.
// If Parse is nil, returns the LLM response text as-is.
func (s *LinkStage) Decode(resp *core.LLMResponse) (any, error) {
	if s == nil || s.Link == nil {
		return nil, fmt.Errorf("chainer: nil LinkStage or Link")
	}
	if resp == nil {
		return nil, fmt.Errorf("chainer: nil LLMResponse")
	}

	// If no parser, return raw text
	if s.Link.Parse == nil {
		return resp.Text, nil
	}

	// Invoke the parser
	parsed, err := s.Link.Parse(resp.Text)
	if err != nil {
		return nil, &chainererrors.LinkDecodeError{
			LinkName:     s.Link.Name,
			ResponseText: resp.Text,
			Cause:        err,
		}
	}
	return parsed, nil
}

// Validate checks the parsed output against the link's schema.
// If Link.Schema is set, validates output as JSON matching the declared schema.
// Returns LinkValidationError on validation failure (triggers retry).
func (s *LinkStage) Validate(output any) error {
	if s == nil {
		return fmt.Errorf("chainer: nil LinkStage")
	}
	if s.Link == nil {
		return fmt.Errorf("chainer: nil Link")
	}

	// Phase 5: Schema validation
	// Skip validation if no schema declared
	if s.Link.Schema == "" {
		return nil
	}

	// Create JSON validator
	validator := validation.NewJSONValidator(s.Link.Schema)

	// Validate output
	err := validator.Validate(output)
	if err != nil {
		return &chainererrors.LinkValidationError{
			LinkName:       s.Link.Name,
			OutputKey:      s.Link.OutputKey,
			ParsedOutput:   output,
			ExpectedSchema: s.Link.Schema,
			ValidationErr:  err,
		}
	}

	return nil
}

// Apply writes the output to the context at OutputKey.
// Enforces single-output-key invariant: writes exactly to s.Link.OutputKey.
func (s *LinkStage) Apply(ctx *core.Context, output any) error {
	if s == nil || s.Link == nil {
		return fmt.Errorf("chainer: nil LinkStage or Link")
	}
	if ctx == nil {
		ctx = core.NewContext()
	}

	// Write to the declared output key
	ctx.Set(s.Link.OutputKey, output)

	// Record interaction for audit trail
	if resp, ok := output.(string); ok {
		recordInteractionForStage(ctx, "assistant", resp, map[string]any{
			"link": s.Link.Name,
		})
	}

	return nil
}

// Helper functions (duplicated from stages/base.go to avoid import cycle)

func filterStateForStage(ctx *core.Context, keys []string) map[string]any {
	filtered := make(map[string]any, len(keys))
	if ctx == nil {
		return filtered
	}
	for _, key := range keys {
		if value, ok := ctx.Get(key); ok {
			filtered[key] = value
		}
	}
	return filtered
}

func renderPromptForStage(templateSrc string, instruction string, inputState map[string]any) (string, error) {
	if templateSrc == "" {
		return "", fmt.Errorf("chainer: template required")
	}
	tpl, err := template.New("link").Parse(templateSrc)
	if err != nil {
		return "", fmt.Errorf("chainer: parse template: %w", err)
	}
	var buf bytes.Buffer
	data := struct {
		Instruction string
		Input       map[string]any
	}{
		Instruction: instruction,
		Input:       inputState,
	}
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("chainer: execute template: %w", err)
	}
	return buf.String(), nil
}

func recordInteractionForStage(ctx *core.Context, role, content string, metadata map[string]any) {
	if ctx == nil {
		return
	}
	ctx.AddInteraction(role, content, metadata)
}
