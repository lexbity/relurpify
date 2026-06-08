package registry

// ResultPayload is the typed outcome payload carried by a Result.
type ResultPayload interface {
	resultPayload()
	Fields() map[string]any
}

// ToolResultPayload carries structured node output as keyed fields.
type ToolResultPayload struct {
	FieldsMap map[string]any
}

func (ToolResultPayload) resultPayload() {}

// Fields returns the payload fields.
func (p ToolResultPayload) Fields() map[string]any {
	return p.FieldsMap
}

// PromptResultPayload carries prompt-oriented output as text plus optional fields.
type PromptResultPayload struct {
	Text      string
	FieldsMap map[string]any
}

func (PromptResultPayload) resultPayload() {}

// Fields returns the payload fields.
func (p PromptResultPayload) Fields() map[string]any {
	fields := p.FieldsMap
	if p.Text != "" {
		if fields == nil {
			fields = map[string]any{}
		}
		fields["text"] = p.Text
	}
	return fields
}

// ErrorResultPayload carries a structured error payload.
type ErrorResultPayload struct {
	Message string
}

func (ErrorResultPayload) resultPayload() {}

// Fields returns the payload fields.
func (p ErrorResultPayload) Fields() map[string]any {
	if p.Message == "" {
		return nil
	}
	return map[string]any{"error": p.Message}
}

// NewToolResultPayload wraps the provided fields in a typed payload.
func NewToolResultPayload(fields map[string]any) ToolResultPayload {
	return ToolResultPayload{FieldsMap: fields}
}

// NewPromptResultPayload wraps prompt output in a typed payload.
func NewPromptResultPayload(text string, fields map[string]any) PromptResultPayload {
	return PromptResultPayload{Text: text, FieldsMap: fields}
}

// NewErrorResultPayload wraps an error message in a typed payload.
func NewErrorResultPayload(message string) ErrorResultPayload {
	return ErrorResultPayload{Message: message}
}

// ResultFields returns the payload fields for the provided result payload.
func ResultFields(payload ResultPayload) map[string]any {
	if payload == nil {
		return nil
	}
	return payload.Fields()
}

// ResultField extracts a single field from a payload.
func ResultField(payload ResultPayload, key string) (any, bool) {
	fields := ResultFields(payload)
	if len(fields) == 0 {
		return nil, false
	}
	value, ok := fields[key]
	return value, ok
}
