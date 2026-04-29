package orchestrate

import (
	"time"
)

// MessageRole represents the role of a message sender.
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
)

// Message represents a single message in the interaction frame.
type Message struct {
	Role      MessageRole
	Content   string
	Timestamp time.Time
	Metadata  map[string]string
}

// InteractionFrame holds the conversation state, messages, and interaction metadata.
type InteractionFrame struct {
	SessionID    string
	TaskID       string
	Messages     []Message
	Metadata     map[string]string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Active       bool
}

// NewInteractionFrame creates a new interaction frame.
func NewInteractionFrame(sessionID, taskID string) *InteractionFrame {
	now := time.Now()
	return &InteractionFrame{
		SessionID: sessionID,
		TaskID:    taskID,
		Messages:  []Message{},
		Metadata:  map[string]string{},
		CreatedAt: now,
		UpdatedAt: now,
		Active:    true,
	}
}

// AddMessage adds a message to the frame.
func (f *InteractionFrame) AddMessage(role MessageRole, content string, metadata map[string]string) {
	now := time.Now()
	message := Message{
		Role:      role,
		Content:   content,
		Timestamp: now,
		Metadata:  metadata,
	}
	f.Messages = append(f.Messages, message)
	f.UpdatedAt = now
}

// GetHistory returns the message history.
func (f *InteractionFrame) GetHistory() []Message {
	return f.Messages
}

// GetLastMessage returns the last message in the frame.
func (f *InteractionFrame) GetLastMessage() *Message {
	if len(f.Messages) == 0 {
		return nil
	}
	return &f.Messages[len(f.Messages)-1]
}

// SetMetadata sets a metadata key-value pair.
func (f *InteractionFrame) SetMetadata(key, value string) {
	f.Metadata[key] = value
	f.UpdatedAt = time.Now()
}

// GetMetadata retrieves a metadata value by key.
func (f *InteractionFrame) GetMetadata(key string) (string, bool) {
	value, ok := f.Metadata[key]
	return value, ok
}

// GetAllMetadata returns all metadata.
func (f *InteractionFrame) GetAllMetadata() map[string]string {
	return f.Metadata
}

// Clear clears all messages from the frame.
func (f *InteractionFrame) Clear() {
	f.Messages = []Message{}
	f.UpdatedAt = time.Now()
}

// Deactivate marks the frame as inactive.
func (f *InteractionFrame) Deactivate() {
	f.Active = false
	f.UpdatedAt = time.Now()
}

// Activate marks the frame as active.
func (f *InteractionFrame) Activate() {
	f.Active = true
	f.UpdatedAt = time.Now()
}

// IsActive returns whether the frame is active.
func (f *InteractionFrame) IsActive() bool {
	return f.Active
}

// MessageCount returns the number of messages in the frame.
func (f *InteractionFrame) MessageCount() int {
	return len(f.Messages)
}
