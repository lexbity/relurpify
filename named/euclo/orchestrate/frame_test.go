package orchestrate

import (
	"testing"
)

func TestInteractionFrameAddMessage(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	frame.AddMessage(MessageRoleUser, "Hello, world", nil)

	if frame.MessageCount() != 1 {
		t.Errorf("Expected 1 message, got %d", frame.MessageCount())
	}

	message := frame.GetLastMessage()
	if message == nil {
		t.Fatal("Expected last message to be non-nil")
	}

	if message.Role != MessageRoleUser {
		t.Errorf("Expected role user, got %s", message.Role)
	}

	if message.Content != "Hello, world" {
		t.Errorf("Expected content Hello, world, got %s", message.Content)
	}
}

func TestInteractionFrameGetHistory(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	frame.AddMessage(MessageRoleUser, "Hello", nil)
	frame.AddMessage(MessageRoleAssistant, "Hi there", nil)
	frame.AddMessage(MessageRoleUser, "How are you?", nil)

	history := frame.GetHistory()
	if len(history) != 3 {
		t.Errorf("Expected 3 messages in history, got %d", len(history))
	}

	if history[0].Content != "Hello" {
		t.Errorf("Expected first message content Hello, got %s", history[0].Content)
	}

	if history[1].Content != "Hi there" {
		t.Errorf("Expected second message content Hi there, got %s", history[1].Content)
	}

	if history[2].Content != "How are you?" {
		t.Errorf("Expected third message content How are you?, got %s", history[2].Content)
	}
}

func TestInteractionFrameSetMetadata(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	frame.SetMetadata("key1", "value1")
	frame.SetMetadata("key2", "value2")

	value1, ok := frame.GetMetadata("key1")
	if !ok {
		t.Error("Expected key1 to exist")
	}

	if value1 != "value1" {
		t.Errorf("Expected value1, got %s", value1)
	}

	value2, ok := frame.GetMetadata("key2")
	if !ok {
		t.Error("Expected key2 to exist")
	}

	if value2 != "value2" {
		t.Errorf("Expected value2, got %s", value2)
	}
}

func TestInteractionFrameClear(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	frame.AddMessage(MessageRoleUser, "Hello", nil)
	frame.AddMessage(MessageRoleAssistant, "Hi", nil)

	if frame.MessageCount() != 2 {
		t.Errorf("Expected 2 messages before clear, got %d", frame.MessageCount())
	}

	frame.Clear()

	if frame.MessageCount() != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", frame.MessageCount())
	}
}

func TestInteractionFrameGetLastMessage(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	frame.AddMessage(MessageRoleUser, "First", nil)
	frame.AddMessage(MessageRoleAssistant, "Second", nil)

	lastMessage := frame.GetLastMessage()
	if lastMessage == nil {
		t.Fatal("Expected last message to be non-nil")
	}

	if lastMessage.Content != "Second" {
		t.Errorf("Expected last message content Second, got %s", lastMessage.Content)
	}
}

func TestInteractionFrameGetLastMessageEmpty(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	lastMessage := frame.GetLastMessage()
	if lastMessage != nil {
		t.Error("Expected last message to be nil when frame is empty")
	}
}

func TestInteractionFrameGetAllMetadata(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	frame.SetMetadata("key1", "value1")
	frame.SetMetadata("key2", "value2")

	allMetadata := frame.GetAllMetadata()
	if len(allMetadata) != 2 {
		t.Errorf("Expected 2 metadata entries, got %d", len(allMetadata))
	}

	if allMetadata["key1"] != "value1" {
		t.Errorf("Expected key1 value1, got %s", allMetadata["key1"])
	}

	if allMetadata["key2"] != "value2" {
		t.Errorf("Expected key2 value2, got %s", allMetadata["key2"])
	}
}

func TestInteractionFrameDeactivate(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	if !frame.IsActive() {
		t.Error("Expected frame to be active initially")
	}

	frame.Deactivate()

	if frame.IsActive() {
		t.Error("Expected frame to be inactive after deactivate")
	}
}

func TestInteractionFrameActivate(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	frame.Deactivate()
	frame.Activate()

	if !frame.IsActive() {
		t.Error("Expected frame to be active after activate")
	}
}

func TestInteractionFrameSessionID(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	if frame.SessionID != "session-123" {
		t.Errorf("Expected session ID session-123, got %s", frame.SessionID)
	}
}

func TestInteractionFrameTaskID(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	if frame.TaskID != "task-456" {
		t.Errorf("Expected task ID task-456, got %s", frame.TaskID)
	}
}

func TestInteractionFrameMessageWithMetadata(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	metadata := map[string]string{
		"source": "test",
		"priority": "high",
	}
	frame.AddMessage(MessageRoleUser, "Hello", metadata)

	message := frame.GetLastMessage()
	if message == nil {
		t.Fatal("Expected last message to be non-nil")
	}

	if message.Metadata["source"] != "test" {
		t.Errorf("Expected metadata source test, got %s", message.Metadata["source"])
	}

	if message.Metadata["priority"] != "high" {
		t.Errorf("Expected metadata priority high, got %s", message.Metadata["priority"])
	}
}

func TestInteractionFrameGetMetadataMissing(t *testing.T) {
	frame := NewInteractionFrame("session-123", "task-456")

	_, ok := frame.GetMetadata("nonexistent")
	if ok {
		t.Error("Expected ok to be false for missing metadata key")
	}
}
