package intake

import (
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
)

// EnvelopeBuilder constructs TaskEnvelope instances with a fluent API.
type EnvelopeBuilder struct {
	envelope *TaskEnvelope
	hints    *ParseResult
	errors   []error
}

// NewEnvelopeBuilder creates a new envelope builder.
func NewEnvelopeBuilder() *EnvelopeBuilder {
	return &EnvelopeBuilder{
		envelope: &TaskEnvelope{
			Metadata: make(map[string]any),
		},
		errors: make([]error, 0),
	}
}

// FromTask initializes the builder from a framework Task.
func (b *EnvelopeBuilder) FromTask(task *core.Task) *EnvelopeBuilder {
	if task == nil {
		b.errors = append(b.errors, fmt.Errorf("task is nil"))
		return b
	}

	b.envelope.TaskID = task.ID
	b.envelope.Instruction = task.Instruction
	b.envelope.RawMessage = task.Instruction

	return b
}

// WithTaskID sets the task ID.
func (b *EnvelopeBuilder) WithTaskID(id string) *EnvelopeBuilder {
	b.envelope.TaskID = id
	return b
}

// WithSessionID sets the session ID.
func (b *EnvelopeBuilder) WithSessionID(id string) *EnvelopeBuilder {
	b.envelope.SessionID = id
	return b
}

// WithInstruction sets the raw instruction.
func (b *EnvelopeBuilder) WithInstruction(instruction string) *EnvelopeBuilder {
	b.envelope.Instruction = instruction
	b.envelope.RawMessage = instruction
	return b
}

// WithContextHint sets the context hint.
func (b *EnvelopeBuilder) WithContextHint(hint string) *EnvelopeBuilder {
	b.envelope.ContextHint = hint
	return b
}

// WithSessionHint sets the session hint.
func (b *EnvelopeBuilder) WithSessionHint(hint string) *EnvelopeBuilder {
	b.envelope.SessionHint = hint
	return b
}

// WithFollowUpHint sets the follow-up hint.
func (b *EnvelopeBuilder) WithFollowUpHint(hint string) *EnvelopeBuilder {
	b.envelope.FollowUpHint = hint
	return b
}

// WithAgentModeHint sets the agent mode hint.
func (b *EnvelopeBuilder) WithAgentModeHint(hint string) *EnvelopeBuilder {
	b.envelope.AgentModeHint = hint
	return b
}

// WithWorkspaceScopes sets the workspace scopes.
func (b *EnvelopeBuilder) WithWorkspaceScopes(scopes []string) *EnvelopeBuilder {
	b.envelope.WorkspaceScopes = scopes
	return b
}

// AddWorkspaceScope adds a single workspace scope.
func (b *EnvelopeBuilder) AddWorkspaceScope(scope string) *EnvelopeBuilder {
	b.envelope.WorkspaceScopes = append(b.envelope.WorkspaceScopes, scope)
	return b
}

// WithExplicitFiles sets the explicit file paths.
func (b *EnvelopeBuilder) WithExplicitFiles(files []string) *EnvelopeBuilder {
	b.envelope.ExplicitFiles = files
	return b
}

// AddExplicitFile adds a single explicit file path.
func (b *EnvelopeBuilder) AddExplicitFile(file string) *EnvelopeBuilder {
	b.envelope.ExplicitFiles = append(b.envelope.ExplicitFiles, file)
	return b
}

// WithIngestPolicy sets the ingest policy.
func (b *EnvelopeBuilder) WithIngestPolicy(policy string) *EnvelopeBuilder {
	b.envelope.IngestPolicy = policy
	return b
}

// WithIncrementalSince sets the incremental since ref.
func (b *EnvelopeBuilder) WithIncrementalSince(ref string) *EnvelopeBuilder {
	b.envelope.IncrementalSince = ref
	return b
}

// WithCleanMessage sets the cleaned message (with hints stripped).
func (b *EnvelopeBuilder) WithCleanMessage(message string) *EnvelopeBuilder {
	b.envelope.CleanMessage = message
	return b
}

// WithMetadata sets metadata key-value pairs.
func (b *EnvelopeBuilder) WithMetadata(key string, value any) *EnvelopeBuilder {
	if b.envelope.Metadata == nil {
		b.envelope.Metadata = make(map[string]any)
	}
	b.envelope.Metadata[key] = value
	return b
}

// ParseAndNormalize parses hints from the instruction and normalizes.
func (b *EnvelopeBuilder) ParseAndNormalize() *EnvelopeBuilder {
	normalizer := NewTaskNormalizer()

	// Ensure we have required fields
	if b.envelope.TaskID == "" {
		b.errors = append(b.errors, fmt.Errorf("task ID is required for normalization"))
		return b
	}

	if b.envelope.RawMessage == "" {
		b.errors = append(b.errors, fmt.Errorf("raw message is required for normalization"))
		return b
	}

	// Normalize
	result := normalizer.Normalize(
		b.envelope.TaskID,
		b.envelope.SessionID,
		b.envelope.RawMessage,
	)

	// Copy normalized values
	b.envelope = result.TaskEnvelope
	b.hints = result.Hints

	return b
}

// Build returns the constructed TaskEnvelope.
func (b *EnvelopeBuilder) Build() (*TaskEnvelope, error) {
	if len(b.errors) > 0 {
		return nil, b.errors[0]
	}

	// Validate required fields
	if b.envelope.TaskID == "" {
		return nil, fmt.Errorf("task ID is required")
	}

	if b.envelope.Instruction == "" {
		return nil, fmt.Errorf("instruction is required")
	}

	// Set defaults
	if b.envelope.IngestPolicy == "" {
		b.envelope.IngestPolicy = "full"
	}

	// Set build timestamp
	if b.envelope.Metadata == nil {
		b.envelope.Metadata = make(map[string]any)
	}
	b.envelope.Metadata["built_at"] = time.Now().UTC().Format(time.RFC3339)

	return b.envelope, nil
}

// MustBuild returns the constructed TaskEnvelope or panics on error.
func (b *EnvelopeBuilder) MustBuild() *TaskEnvelope {
	env, err := b.Build()
	if err != nil {
		panic(err)
	}
	return env
}

// Errors returns any accumulated errors.
func (b *EnvelopeBuilder) Errors() []error {
	return b.errors
}

// BuildFromTask is a convenience function to build a TaskEnvelope from a core.Task.
func BuildFromTask(task *core.Task) (*TaskEnvelope, error) {
	return NewEnvelopeBuilder().
		FromTask(task).
		ParseAndNormalize().
		Build()
}
